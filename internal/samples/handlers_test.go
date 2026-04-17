package samples_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sampleDB/internal/auth"
	"sampleDB/internal/samples"
	"sampleDB/internal/web"
)

// editSampleStub is a minimal Service stub for EditSample handler tests.
// Unimplemented methods are inherited from the embedded interface (nil) and
// will panic if called, which surfaces missing stubs as test failures.
type editSampleStub struct {
	samples.Service
	checkOwner func(ctx context.Context, sampleID, userID int) error
	isAdmin    func(ctx context.Context, userID int) (bool, error)
	getByID    func(ctx context.Context, id int) (samples.Sample, error)
	update     func(ctx context.Context, id int, p samples.UpdateParams) error
}

func (s *editSampleStub) CheckOwner(ctx context.Context, sampleID, userID int) error {
	return s.checkOwner(ctx, sampleID, userID)
}
func (s *editSampleStub) IsAdmin(ctx context.Context, userID int) (bool, error) {
	return s.isAdmin(ctx, userID)
}
func (s *editSampleStub) GetByID(ctx context.Context, id int) (samples.Sample, error) {
	return s.getByID(ctx, id)
}
func (s *editSampleStub) Update(ctx context.Context, id int, p samples.UpdateParams) error {
	return s.update(ctx, id, p)
}

// TestEditSample_AdminPreservesOwnerWhenNotSubmitted verifies that an admin
// editing a sample without providing an owner_user_id in the form does not
// wipe the existing owner — the handler must fetch the current owner and pass
// it through to Update rather than sending nil.
func TestEditSample_AdminPreservesOwnerWhenNotSubmitted(t *testing.T) {
	existingOwnerID := 42
	existingOwnerName := "alice"

	var capturedParams samples.UpdateParams

	svc := &editSampleStub{
		checkOwner: func(_ context.Context, _, _ int) error { return nil },
		isAdmin:    func(_ context.Context, _ int) (bool, error) { return true, nil },
		getByID: func(_ context.Context, id int) (samples.Sample, error) {
			return samples.Sample{
				ID:          id,
				Name:        "Old Name",
				OwnerUserID: &existingOwnerID,
				Owner:       existingOwnerName,
			}, nil
		},
		update: func(_ context.Context, _ int, p samples.UpdateParams) error {
			capturedParams = p
			return nil
		},
	}

	h := samples.NewHandlers(svc, &web.Renderer{}, auth.NewManager(nil), samples.HandlerConfig{})

	form := strings.NewReader("name=New+Name&description=&keywords=")
	req := httptest.NewRequest(http.MethodPost, "/samples/edit/1", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Inject an admin session into the context.
	adminSession := auth.Session{UserID: 99, Username: "admin"}
	req = req.WithContext(auth.ContextWithSession(req.Context(), adminSession))

	rr := httptest.NewRecorder()
	h.EditSample(rr, req)

	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rr.Code, rr.Body.String())
	}

	if capturedParams.OwnerUserID == nil {
		t.Fatal("Update was called with nil OwnerUserID: admin edit without owner form field wiped the existing owner")
	}
	if *capturedParams.OwnerUserID != existingOwnerID {
		t.Errorf("OwnerUserID = %d, want %d", *capturedParams.OwnerUserID, existingOwnerID)
	}
	if capturedParams.Owner != existingOwnerName {
		t.Errorf("Owner = %q, want %q", capturedParams.Owner, existingOwnerName)
	}
}
