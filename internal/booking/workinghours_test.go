package booking_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/booking"
)

// 2026-11-02 is a Monday; 2026-11-07 is a Saturday.
func TestWorkingWindow_Contains(t *testing.T) {
	loc := time.FixedZone("SGT", 8*3600)
	window := booking.WorkingWindow{Open: true, StartMinute: 480, EndMinute: 1080}
	cases := []struct {
		name       string
		start, end string
		want       bool
	}{
		{"inside", "2026-11-02T10:00", "2026-11-02T12:00", true},
		{"exactly the full window", "2026-11-02T08:00", "2026-11-02T18:00", true},
		{"starts before opening", "2026-11-02T07:45", "2026-11-02T09:00", false},
		{"ends after closing", "2026-11-02T17:00", "2026-11-02T18:15", false},
		{"fully before opening", "2026-11-02T05:00", "2026-11-02T06:00", false},
		{"fully after closing", "2026-11-02T19:00", "2026-11-02T20:00", false},
		{"starts at closing", "2026-11-02T18:00", "2026-11-02T18:15", false},
		{"ends at opening", "2026-11-02T07:00", "2026-11-02T08:00", false},
		{"overnight", "2026-11-02T17:00", "2026-11-03T09:00", false},
		{"two days, hours each inside", "2026-11-02T09:00", "2026-11-03T17:00", false},
		{"ends at midnight", "2026-11-02T17:00", "2026-11-03T00:00", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, _ := time.ParseInLocation("2006-01-02T15:04", c.start, loc)
			end, _ := time.ParseInLocation("2006-01-02T15:04", c.end, loc)
			if got := window.Contains(start, end, loc); got != c.want {
				t.Errorf("Contains(%s, %s) = %v, want %v", c.start, c.end, got, c.want)
			}
		})
	}
}

func TestWorkingWindow_ClosedDayContainsNothing(t *testing.T) {
	start := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	window := booking.WorkingWindow{Open: false, StartMinute: 480, EndMinute: 1080}
	if window.Contains(start, start.Add(time.Hour), time.UTC) {
		t.Error("a closed day must not contain any booking")
	}
}

// The window is judged in the app timezone, whatever zone the times arrive in.
func TestWorkingWindow_UsesGivenLocation(t *testing.T) {
	loc := time.FixedZone("SGT", 8*3600)
	window := booking.WorkingWindow{Open: true, StartMinute: 480, EndMinute: 1080}
	// 02:00–03:00 UTC is 10:00–11:00 in SGT.
	start := time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)
	if !window.Contains(start, start.Add(time.Hour), loc) {
		t.Error("10:00–11:00 local should be inside working hours")
	}
	if window.Contains(start, start.Add(time.Hour), time.UTC) {
		t.Error("02:00–03:00 UTC should be outside working hours")
	}
}

func TestWorkingWindow_Describe(t *testing.T) {
	regular := booking.WorkingWindow{
		Date: "2026-12-25", RegularStartMinute: 510, RegularEndMinute: 1020,
		RegularWeekdays: []int32{0, 1, 5},
	}
	cases := []struct {
		name   string
		mutate func(*booking.WorkingWindow)
		want   string
	}{
		{"regular, Monday first", func(*booking.WorkingWindow) {}, "Working hours are Mon, Fri, Sun, 08:30–17:00."},
		{"closed exception", func(w *booking.WorkingWindow) { w.IsException = true }, "There are no working hours on 2026-12-25."},
		{"closed exception with note", func(w *booking.WorkingWindow) {
			w.IsException, w.Note = true, "Christmas"
		}, "There are no working hours on 2026-12-25 (Christmas)."},
		{"exception with hours", func(w *booking.WorkingWindow) {
			w.IsException, w.Open, w.StartMinute, w.EndMinute = true, true, 540, 780
		}, "Working hours on 2026-12-25 are 09:00–13:00."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := regular
			c.mutate(&w)
			if got := w.Describe(); got != c.want {
				t.Errorf("Describe() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestIsAFM(t *testing.T) {
	for name, want := range map[string]bool{
		"AFM": true, "afm": true, "AFM-Two": true, "Bruker AFM": true,
		"SEM": false, "TEM": false, "": false,
	} {
		if got := booking.IsAFM(name); got != want {
			t.Errorf("IsAFM(%q) = %v, want %v", name, got, want)
		}
	}
}

func postBooking(t *testing.T, svc *mockService, start, end string) string {
	t.Helper()
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)
	form := url.Values{"equipment_id": {"1"}, "start_time": {start}, "end_time": {end}, "purpose": {"scan"}}
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, newAuthenticatedRequest(http.MethodPost, "/booking", form))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	return rr.Header().Get("Location")
}

const (
	insideHours  = "2026-11-02T10:00"
	insideHours2 = "2026-11-02T12:00"
	crossesEnd   = "2026-11-02T19:00"
	weekendStart = "2026-11-07T10:00"
	weekendEnd   = "2026-11-07T11:00"
)

func TestSubmitBooking_LimitedUserBlockedOutsideHours(t *testing.T) {
	for _, c := range []struct{ name, start, end string }{
		{"crosses the boundary", insideHours, crossesEnd},
		{"fully outside", "2026-11-02T19:00", "2026-11-02T20:00"},
		{"weekend", weekendStart, weekendEnd},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc := &mockService{hasPermission: true, equipmentName: "AFM"}
			loc := postBooking(t, svc, c.start, c.end)
			if !strings.Contains(loc, "error=") || !strings.Contains(loc, "08%3A00") {
				t.Errorf("Location = %q, want the working-hours error", loc)
			}
			if svc.created != 0 {
				t.Error("booking must not be created")
			}
		})
	}
}

func TestSubmitBooking_LimitedUserAllowedInsideHours(t *testing.T) {
	svc := &mockService{hasPermission: true, equipmentName: "AFM"}
	if loc := postBooking(t, svc, insideHours, insideHours2); !strings.Contains(loc, "success=") {
		t.Errorf("Location = %q, want success", loc)
	}
	if svc.created != 1 {
		t.Errorf("created = %d, want 1", svc.created)
	}
}

func TestSubmitBooking_AllowedUserCanBookOutsideHours(t *testing.T) {
	svc := &mockService{hasPermission: true, equipmentName: "AFM", afmAllowed: true}
	if loc := postBooking(t, svc, weekendStart, weekendEnd); !strings.Contains(loc, "success=") {
		t.Errorf("Location = %q, want success", loc)
	}
}

// The checkbox is the only exemption: an admin with it unticked is limited too.
func TestSubmitBooking_LimitedAdminBlockedOutsideHours(t *testing.T) {
	svc := &mockService{isAdmin: true, equipmentName: "AFM"}
	if loc := postBooking(t, svc, weekendStart, weekendEnd); !strings.Contains(loc, "error=") {
		t.Errorf("Location = %q, want the working-hours error", loc)
	}
	if svc.created != 0 {
		t.Error("booking must not be created")
	}
}

func TestSubmitBooking_OtherEquipmentNotLimited(t *testing.T) {
	svc := &mockService{hasPermission: true, equipmentName: "SEM"}
	if loc := postBooking(t, svc, weekendStart, weekendEnd); !strings.Contains(loc, "success=") {
		t.Errorf("Location = %q, want success", loc)
	}
	if svc.afmLookups != 0 {
		t.Errorf("flag looked up %d times for non-AFM equipment", svc.afmLookups)
	}
}

func TestSubmitBooking_LookupFailuresDeny(t *testing.T) {
	for name, svc := range map[string]*mockService{
		"flag":   {hasPermission: true, equipmentName: "AFM", afmErr: errors.New("db down")},
		"window": {hasPermission: true, equipmentName: "AFM", windowErr: errors.New("db down")},
	} {
		t.Run(name, func(t *testing.T) {
			if loc := postBooking(t, svc, insideHours, insideHours2); !strings.Contains(loc, "error=") {
				t.Errorf("Location = %q, want error", loc)
			}
			if svc.created != 0 {
				t.Error("booking must not be created when the check can't run")
			}
		})
	}
}

// A holiday on a normal working day shows that day's rule, not the regular week.
func TestSubmitBooking_ExceptionMessageNamesTheDay(t *testing.T) {
	svc := &mockService{hasPermission: true, equipmentName: "AFM", window: &booking.WorkingWindow{
		Date: "2026-11-02", IsException: true, Note: "Deepavali",
	}}
	loc := postBooking(t, svc, insideHours, insideHours2)
	if !strings.Contains(loc, "error=") || !strings.Contains(loc, "2026-11-02") || !strings.Contains(loc, "Deepavali") {
		t.Errorf("Location = %q, want an error naming the date and note", loc)
	}
	if strings.Contains(loc, "fit+inside") {
		t.Errorf("Location = %q, a day with no hours has nothing to fit inside", loc)
	}
	if svc.created != 0 {
		t.Error("booking must not be created")
	}
}

var afmFlagSQL = regexp.QuoteMeta(`SELECT afm_outside_hours_allowed FROM users WHERE user_id = $1`)

func TestCanBookAFMOutsideHours_ReturnsFlag(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(afmFlagSQL).WithArgs(7).
		WillReturnRows(pgxmock.NewRows([]string{"afm_outside_hours_allowed"}).AddRow(true))

	got, err := svc.CanBookAFMOutsideHours(context.Background(), 7)
	if err != nil || !got {
		t.Fatalf("got %v, %v; want true, nil", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCanBookAFMOutsideHours_UnknownUserIsNotAllowed(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(afmFlagSQL).WithArgs(7).
		WillReturnRows(pgxmock.NewRows([]string{"afm_outside_hours_allowed"}))

	got, err := svc.CanBookAFMOutsideHours(context.Background(), 7)
	if err != nil || got {
		t.Fatalf("got %v, %v; want false, nil", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var (
	scheduleSQL  = regexp.QuoteMeta(`SELECT start_minute, end_minute, weekdays FROM working_hours`)
	exceptionSQL = regexp.QuoteMeta(`SELECT start_minute, end_minute, note FROM working_hours_exceptions`)
	monday       = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	saturday     = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
)

func expectSchedule(mock pgxmock.PgxPoolIface, start, end int, days []int32) {
	mock.ExpectQuery(scheduleSQL).
		WillReturnRows(pgxmock.NewRows([]string{"start_minute", "end_minute", "weekdays"}).AddRow(start, end, days))
}

func TestGetWorkingWindow_RegularSchedule(t *testing.T) {
	svc, mock := newSvc(t)
	for _, c := range []struct {
		day      time.Time
		date     string
		wantOpen bool
	}{
		{monday, "2026-11-02", true},
		{saturday, "2026-11-07", false},
	} {
		expectSchedule(mock, 540, 1020, []int32{1, 2, 3, 4, 5})
		mock.ExpectQuery(exceptionSQL).WithArgs(c.date).WillReturnError(pgx.ErrNoRows)

		w, err := svc.GetWorkingWindow(context.Background(), c.day)
		if err != nil {
			t.Fatalf("GetWorkingWindow(%s): %v", c.date, err)
		}
		if w.Open != c.wantOpen || w.StartMinute != 540 || w.EndMinute != 1020 || w.IsException {
			t.Errorf("%s: window = %+v", c.date, w)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetWorkingWindow_ClosedExceptionOverridesWorkingDay(t *testing.T) {
	svc, mock := newSvc(t)
	expectSchedule(mock, 480, 1080, []int32{1, 2, 3, 4, 5})
	mock.ExpectQuery(exceptionSQL).WithArgs("2026-11-02").
		WillReturnRows(pgxmock.NewRows([]string{"start_minute", "end_minute", "note"}).
			AddRow((*int)(nil), (*int)(nil), "Holiday"))

	w, err := svc.GetWorkingWindow(context.Background(), monday)
	if err != nil {
		t.Fatal(err)
	}
	if w.Open || !w.IsException || w.Note != "Holiday" {
		t.Errorf("window = %+v", w)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetWorkingWindow_ExceptionOpensNonWorkingDay(t *testing.T) {
	svc, mock := newSvc(t)
	start, end := 540, 780
	expectSchedule(mock, 480, 1080, []int32{1, 2, 3, 4, 5})
	mock.ExpectQuery(exceptionSQL).WithArgs("2026-11-07").
		WillReturnRows(pgxmock.NewRows([]string{"start_minute", "end_minute", "note"}).AddRow(&start, &end, ""))

	w, err := svc.GetWorkingWindow(context.Background(), saturday)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Open || w.StartMinute != 540 || w.EndMinute != 780 {
		t.Errorf("window = %+v", w)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetWorkingWindow_ScheduleErrorIsReturned(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(scheduleSQL).WillReturnError(errors.New("db down"))

	if _, err := svc.GetWorkingWindow(context.Background(), monday); err == nil {
		t.Fatal("expected an error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
