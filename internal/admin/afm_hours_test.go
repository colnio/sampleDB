package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/admin"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

func postAccess(t *testing.T, svc *mockService, body string) {
	t.Helper()
	h := newHandlers(svc)
	req := httptest.NewRequest(http.MethodPost, "/admin/update-access", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "admin"}))
	rr := httptest.NewRecorder()
	h.UpdateAccess(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestUpdateAccess_PassesAFMFlag(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
		want *bool
	}{
		{"unticked", `{"user_id":5,"approved":true,"afm_outside_hours_allowed":false}`, boolPtr(false)},
		{"ticked", `{"user_id":5,"approved":true,"afm_outside_hours_allowed":true}`, boolPtr(true)},
		{"absent leaves it unchanged", `{"user_id":5,"approved":true}`, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc := &mockService{}
			postAccess(t, svc, c.body)
			got := svc.lastAccess.AFMOutsideHoursAllowed
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Errorf("AFMOutsideHoursAllowed = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAdminPage_RendersAFMCheckboxState(t *testing.T) {
	svc := &mockService{users: []admin.UserAccess{
		{UserID: 1, Username: "trusted", AFMOutsideHoursAllowed: true},
		{UserID: 2, Username: "limited", AFMOutsideHoursAllowed: false},
	}}
	rr := renderAdminPage(t, svc)
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d, body = %s", rr.Code, rr.Body.String())
	}
	// The server treats an absent field as "unchanged", so without this key in
	// updateAccess the checkbox would silently do nothing.
	if !strings.Contains(rr.Body.String(), "afm_outside_hours_allowed: afmHoursToggle") {
		t.Error("updateAccess must send afm_outside_hours_allowed")
	}
	cards := strings.Split(rr.Body.String(), `<article class="user-card"`)
	if len(cards) != 3 {
		t.Fatalf("expected 2 user cards, got %d", len(cards)-1)
	}
	for i, wantChecked := range []bool{true, false} {
		card := cards[i+1]
		_, after, found := strings.Cut(card, "afm-hours-toggle")
		if !found {
			t.Fatalf("card %d has no AFM checkbox", i)
		}
		input, _, _ := strings.Cut(after, ">")
		if got := strings.Contains(input, "checked"); got != wantChecked {
			t.Errorf("card %d checked = %v, want %v", i, got, wantChecked)
		}
	}
}

func TestAdminPage_RendersWorkingHours(t *testing.T) {
	start, end := 540, 780
	svc := &mockService{
		workingHours: admin.WorkingHours{StartMinute: 510, EndMinute: 1020, Weekdays: []int32{1, 6}},
		exceptions: []admin.WorkingHoursException{
			{Date: "2026-12-24", StartMinute: &start, EndMinute: &end, Note: "Half day"},
			{Date: "2026-12-25", Note: "Christmas"},
		},
	}
	rr := renderAdminPage(t, svc)
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`value="08:30"`, `value="17:00"`,
		"2026-12-24", "09:00–13:00", "Half day",
		"2026-12-25", "No working hours", "Christmas",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin page missing %q", want)
		}
	}
	checked := regexp.MustCompile(`name="weekday" value="(\d)"\s+checked`).FindAllStringSubmatch(body, -1)
	if len(checked) != 2 || checked[0][1] != "1" || checked[1][1] != "6" {
		t.Errorf("checked weekdays = %v, want Mon and Sat", checked)
	}
}

func TestAdminPage_ShowsWhichEquipmentTheHoursApplyTo(t *testing.T) {
	svc := &mockService{equipment: []admin.Equipment{{ID: 1, Name: "SEM"}, {ID: 2, Name: "AFM-1"}, {ID: 3, Name: "Bruker AFM"}}}
	body := renderAdminPage(t, svc).Body.String()
	if !strings.Contains(body, "Applies to: AFM-1, Bruker AFM") {
		t.Error("admin page should list the matching equipment")
	}
	if strings.Contains(body, "not being enforced") {
		t.Error("no warning expected when equipment matches")
	}
}

func TestAdminPage_WarnsWhenNoEquipmentMatches(t *testing.T) {
	svc := &mockService{equipment: []admin.Equipment{{ID: 1, Name: "Bruker Icon"}}}
	if body := renderAdminPage(t, svc).Body.String(); !strings.Contains(body, "not being enforced") {
		t.Error("admin page should warn that the hours apply to nothing")
	}
}

func postForm(t *testing.T, handler http.HandlerFunc, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	handler(rr, newAuthenticatedRequest(http.MethodPost, path, form))
	return rr
}

func TestHandlerSetWorkingHours_Success(t *testing.T) {
	svc := &mockService{}
	form := url.Values{"start": {"09:15"}, "end": {"17:30"}, "weekday": {"1", "3", "3", "6"}}
	rr := postForm(t, newHandlers(svc).SetWorkingHours, "/admin/working-hours", form)

	if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || !strings.Contains(loc, "success=") {
		t.Fatalf("got %d %q, want success redirect", rr.Code, loc)
	}
	want := admin.WorkingHours{StartMinute: 555, EndMinute: 1050, Weekdays: []int32{1, 3, 6}}
	if svc.lastHours == nil || !reflect.DeepEqual(*svc.lastHours, want) {
		t.Errorf("SetWorkingHours called with %+v, want %+v", svc.lastHours, want)
	}
}

func TestHandlerSetWorkingHours_Invalid(t *testing.T) {
	for name, form := range map[string]url.Values{
		"missing start":  {"end": {"17:00"}, "weekday": {"1"}},
		"bad end":        {"start": {"09:00"}, "end": {"25:00"}, "weekday": {"1"}},
		"end not after":  {"start": {"09:00"}, "end": {"09:00"}, "weekday": {"1"}},
		"no days":        {"start": {"09:00"}, "end": {"17:00"}},
		"day too large":  {"start": {"09:00"}, "end": {"17:00"}, "weekday": {"7"}},
		"day not number": {"start": {"09:00"}, "end": {"17:00"}, "weekday": {"Mon"}},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &mockService{}
			rr := postForm(t, newHandlers(svc).SetWorkingHours, "/admin/working-hours", form)
			if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || !strings.Contains(loc, "error=") {
				t.Errorf("got %d %q, want error redirect", rr.Code, loc)
			}
			if svc.lastHours != nil {
				t.Error("invalid input must not reach the service")
			}
		})
	}
}

func TestHandlerSetWorkingHoursException_ClosedDay(t *testing.T) {
	svc := &mockService{}
	form := url.Values{"date": {"2099-12-25"}, "start": {""}, "end": {""}, "note": {"  Christmas "}}
	rr := postForm(t, newHandlers(svc).SetWorkingHoursException, "/admin/working-hours/exceptions", form)

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success=") {
		t.Fatalf("Location = %q, want success", loc)
	}
	e := svc.lastException
	if e == nil || e.Date != "2099-12-25" || e.StartMinute != nil || e.EndMinute != nil || e.Note != "Christmas" {
		t.Errorf("exception = %+v", e)
	}
}

func TestHandlerSetWorkingHoursException_CustomHours(t *testing.T) {
	svc := &mockService{}
	form := url.Values{"date": {"2099-12-24"}, "start": {"09:00"}, "end": {"13:00"}}
	rr := postForm(t, newHandlers(svc).SetWorkingHoursException, "/admin/working-hours/exceptions", form)

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success=") {
		t.Fatalf("Location = %q, want success", loc)
	}
	e := svc.lastException
	if e == nil || e.StartMinute == nil || *e.StartMinute != 540 || e.EndMinute == nil || *e.EndMinute != 780 {
		t.Errorf("exception = %+v", e)
	}
}

func TestHandlerSetWorkingHoursException_TodayIsAllowed(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	h.Loc = time.FixedZone("SGT", 8*3600)
	today := time.Now().In(h.Loc).Format("2006-01-02")
	rr := postForm(t, h.SetWorkingHoursException, "/admin/working-hours/exceptions", url.Values{"date": {today}})
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success=") {
		t.Errorf("Location = %q, want success", loc)
	}
}

func TestHandlerSetWorkingHoursException_Invalid(t *testing.T) {
	for name, form := range map[string]url.Values{
		"no date":       {"start": {"09:00"}, "end": {"13:00"}},
		"bad date":      {"date": {"2099-13-01"}},
		"only start":    {"date": {"2099-12-24"}, "start": {"09:00"}},
		"only end":      {"date": {"2099-12-24"}, "end": {"13:00"}},
		"end not after": {"date": {"2099-12-24"}, "start": {"13:00"}, "end": {"09:00"}},
		"long note":     {"date": {"2099-12-24"}, "note": {strings.Repeat("x", 101)}},
		"past date":     {"date": {"2020-01-01"}},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &mockService{}
			rr := postForm(t, newHandlers(svc).SetWorkingHoursException, "/admin/working-hours/exceptions", form)
			if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || !strings.Contains(loc, "error=") {
				t.Errorf("got %d %q, want error redirect", rr.Code, loc)
			}
			if svc.lastException != nil {
				t.Error("invalid input must not reach the service")
			}
		})
	}
}

func TestHandlerDeleteWorkingHoursException(t *testing.T) {
	svc := &mockService{}
	rr := postForm(t, newHandlers(svc).DeleteWorkingHoursException, "/admin/working-hours/exceptions/delete", url.Values{"date": {"2099-12-25"}})
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success=") || svc.lastDeletedDate != "2099-12-25" {
		t.Errorf("Location = %q, deleted = %q", loc, svc.lastDeletedDate)
	}

	missing := &mockService{deleteExceptionErr: web.ErrNotFound}
	rr = postForm(t, newHandlers(missing).DeleteWorkingHoursException, "/admin/working-hours/exceptions/delete", url.Values{"date": {"2099-12-25"}})
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("Location = %q, want error", loc)
	}
}

func TestWorkingHoursHandlers_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	for path, handler := range map[string]http.HandlerFunc{
		"/admin/working-hours":                   h.SetWorkingHours,
		"/admin/working-hours/exceptions":        h.SetWorkingHoursException,
		"/admin/working-hours/exceptions/delete": h.DeleteWorkingHoursException,
	} {
		rr := httptest.NewRecorder()
		handler(rr, newAuthenticatedRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", path, rr.Code)
		}
	}
}

func TestGetWorkingHours_ReadsRow(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(`SELECT start_minute, end_minute, weekdays FROM working_hours`).
		WillReturnRows(pgxmock.NewRows([]string{"start_minute", "end_minute", "weekdays"}).AddRow(510, 1020, []int32{1, 2}))

	got, err := svc.GetWorkingHours(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Start() != "08:30" || got.End() != "17:00" || len(got.Weekdays) != 2 {
		t.Errorf("hours = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListWorkingHoursExceptions_ScansClosedAndCustomDays(t *testing.T) {
	svc, mock := newSvc(t)
	start, end := 540, 780
	mock.ExpectQuery(`FROM working_hours_exceptions`).WithArgs("2099-01-01").
		WillReturnRows(pgxmock.NewRows([]string{"date", "start_minute", "end_minute", "note"}).
			AddRow("2099-12-24", &start, &end, "Half day").
			AddRow("2099-12-25", (*int)(nil), (*int)(nil), "Christmas"))

	got, err := svc.ListWorkingHoursExceptions(context.Background(), "2099-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Hours() != "09:00–13:00" || got[1].Hours() != "No working hours" {
		t.Errorf("exceptions = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteWorkingHoursException_NotFound(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectExec(`DELETE FROM working_hours_exceptions`).WithArgs("2026-12-25").
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	if err := svc.DeleteWorkingHoursException(context.Background(), "2026-12-25"); err != web.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(b bool) *bool { return &b }
