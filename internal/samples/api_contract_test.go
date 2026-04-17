package samples_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/samples"
)

type sampleAPIServiceStub struct {
	samples.Service

	apiListSamples   func(ctx context.Context, p samples.APIListParams) (samples.APIListResult, error)
	apiGetSample     func(ctx context.Context, id int) (samples.APISampleDetail, error)
	apiCreateSample  func(ctx context.Context, name, description, keywords, owner, samplePrep string, ownerUserID *int) (samples.APISampleItem, error)
	apiUpdateSample  func(ctx context.Context, id int, name, description, keywords, owner, samplePrep string, ownerUserID *int) error
	apiDeleteSample  func(ctx context.Context, id int) ([]string, error)
	checkOwner       func(ctx context.Context, sampleID, userID int) error
	isAdmin          func(ctx context.Context, userID int) (bool, error)
	getUsernameByID  func(ctx context.Context, userID int) (string, error)
	sampleExistsByID func(ctx context.Context, id int) (bool, error)
	getSteps         func(ctx context.Context, sampleID int) ([]samples.FabricationStep, error)
	addStep          func(ctx context.Context, sampleID int, step samples.FabricationStep) (samples.FabricationStep, error)
	getStepByID      func(ctx context.Context, stepID int) (samples.FabricationStep, error)
	updateStep       func(ctx context.Context, stepID int, step samples.FabricationStep) error
	deleteStep       func(ctx context.Context, stepID int) error
	getAttachments   func(ctx context.Context, sampleID int) ([]samples.Attachment, error)
	addAttachment    func(ctx context.Context, sampleID int, path string) (int, error)
	attachmentPath   func(ctx context.Context, attachmentID int) (string, error)
	attachmentInfo   func(ctx context.Context, attachmentID int) (int, string, error)
	deleteAttachment func(ctx context.Context, attachmentID int) (string, error)
}

func (s *sampleAPIServiceStub) APIListSamples(ctx context.Context, p samples.APIListParams) (samples.APIListResult, error) {
	return s.apiListSamples(ctx, p)
}

func (s *sampleAPIServiceStub) APIGetSample(ctx context.Context, id int) (samples.APISampleDetail, error) {
	return s.apiGetSample(ctx, id)
}

func (s *sampleAPIServiceStub) APICreateSample(ctx context.Context, name, description, keywords, owner, samplePrep string, ownerUserID *int) (samples.APISampleItem, error) {
	return s.apiCreateSample(ctx, name, description, keywords, owner, samplePrep, ownerUserID)
}

func (s *sampleAPIServiceStub) APIUpdateSample(ctx context.Context, id int, name, description, keywords, owner, samplePrep string, ownerUserID *int) error {
	return s.apiUpdateSample(ctx, id, name, description, keywords, owner, samplePrep, ownerUserID)
}

func (s *sampleAPIServiceStub) APIDeleteSample(ctx context.Context, id int) ([]string, error) {
	return s.apiDeleteSample(ctx, id)
}

func (s *sampleAPIServiceStub) CheckOwner(ctx context.Context, sampleID, userID int) error {
	return s.checkOwner(ctx, sampleID, userID)
}

func (s *sampleAPIServiceStub) IsAdmin(ctx context.Context, userID int) (bool, error) {
	return s.isAdmin(ctx, userID)
}

func (s *sampleAPIServiceStub) GetUsernameByID(ctx context.Context, userID int) (string, error) {
	return s.getUsernameByID(ctx, userID)
}

func (s *sampleAPIServiceStub) SampleExistsByID(ctx context.Context, id int) (bool, error) {
	return s.sampleExistsByID(ctx, id)
}

func (s *sampleAPIServiceStub) GetSteps(ctx context.Context, sampleID int) ([]samples.FabricationStep, error) {
	return s.getSteps(ctx, sampleID)
}

func (s *sampleAPIServiceStub) AddStep(ctx context.Context, sampleID int, step samples.FabricationStep) (samples.FabricationStep, error) {
	return s.addStep(ctx, sampleID, step)
}

func (s *sampleAPIServiceStub) GetStepByID(ctx context.Context, stepID int) (samples.FabricationStep, error) {
	return s.getStepByID(ctx, stepID)
}

func (s *sampleAPIServiceStub) UpdateStep(ctx context.Context, stepID int, step samples.FabricationStep) error {
	return s.updateStep(ctx, stepID, step)
}

func (s *sampleAPIServiceStub) DeleteStep(ctx context.Context, stepID int) error {
	return s.deleteStep(ctx, stepID)
}

func (s *sampleAPIServiceStub) GetAttachments(ctx context.Context, sampleID int) ([]samples.Attachment, error) {
	return s.getAttachments(ctx, sampleID)
}

func (s *sampleAPIServiceStub) AddAttachment(ctx context.Context, sampleID int, path string) (int, error) {
	return s.addAttachment(ctx, sampleID, path)
}

func (s *sampleAPIServiceStub) AttachmentFilePath(ctx context.Context, attachmentID int) (string, error) {
	return s.attachmentPath(ctx, attachmentID)
}

func (s *sampleAPIServiceStub) AttachmentInfo(ctx context.Context, attachmentID int) (int, string, error) {
	return s.attachmentInfo(ctx, attachmentID)
}

func (s *sampleAPIServiceStub) DeleteAttachment(ctx context.Context, attachmentID int) (string, error) {
	return s.deleteAttachment(ctx, attachmentID)
}

func newSampleAPIHandlers(t *testing.T, svc samples.Service) (*samples.Handlers, string) {
	t.Helper()
	baseDir := t.TempDir()
	return samples.NewHandlers(svc, nil, nil, samples.HandlerConfig{
		BaseDir:    baseDir,
		UploadsDir: filepath.Join(baseDir, "uploads"),
	}), baseDir
}

func authenticatedRequest(method, target string, body io.Reader, contentType string, session auth.Session) *http.Request {
	req := httptest.NewRequest(method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req.WithContext(auth.ContextWithSession(req.Context(), session))
}

func decodeResponse[T any](t *testing.T, body *bytes.Buffer) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestAPISamples_ListSamples_UsesDocumentedEnvelope(t *testing.T) {
	ownerUsername := "alice"
	var gotParams samples.APIListParams

	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		apiListSamples: func(_ context.Context, p samples.APIListParams) (samples.APIListResult, error) {
			gotParams = p
			return samples.APIListResult{
				Items: []samples.APISampleItem{{
					ID:              1,
					Name:            "Si wafer",
					Description:     "4-inch p-type silicon",
					Keywords:        "silicon wafer",
					Owner:           "alice",
					OwnerUsername:   &ownerUsername,
					AttachmentCount: 2,
					CreatedAt:       time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
				}},
				Total:  9,
				Limit:  25,
				Offset: 50,
			}, nil
		},
	})

	req := authenticatedRequest(http.MethodGet, "/api/v1/samples?q=wafer&owner=ali&from=2026-01-01&to=2026-01-31&limit=25&offset=50", nil, "", auth.Session{UserID: 1, Username: "tester"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if gotParams.Q != "wafer" || gotParams.Owner != "ali" || gotParams.From != "2026-01-01" || gotParams.To != "2026-01-31" {
		t.Fatalf("unexpected params: %#v", gotParams)
	}
	if gotParams.Limit != 25 || gotParams.Offset != 50 {
		t.Fatalf("unexpected pagination: %#v", gotParams)
	}

	var resp struct {
		Items  []samples.APISampleItem `json:"items"`
		Total  int                     `json:"total"`
		Limit  int                     `json:"limit"`
		Offset int                     `json:"offset"`
	}
	resp = decodeResponse[struct {
		Items  []samples.APISampleItem `json:"items"`
		Total  int                     `json:"total"`
		Limit  int                     `json:"limit"`
		Offset int                     `json:"offset"`
	}](t, rr.Body)

	if len(resp.Items) != 1 || resp.Items[0].Name != "Si wafer" {
		t.Fatalf("unexpected items: %#v", resp.Items)
	}
	if resp.Total != 9 || resp.Limit != 25 || resp.Offset != 50 {
		t.Fatalf("unexpected envelope: %#v", resp)
	}
}

func TestAPISamples_GetSample_ReturnsDetail(t *testing.T) {
	stepDate := "2026-01-14"
	personUsername := "alice"
	equipmentName := "Wet bench 1"
	ownerUserID := 3
	ownerUsername := "alice"

	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		apiGetSample: func(_ context.Context, id int) (samples.APISampleDetail, error) {
			if id != 42 {
				t.Fatalf("APIGetSample called with id=%d, want 42", id)
			}
			return samples.APISampleDetail{
				ID:              42,
				Name:            "Si wafer",
				Description:     "4-inch p-type silicon",
				Keywords:        "silicon wafer",
				Owner:           "alice",
				OwnerUserID:     &ownerUserID,
				OwnerUsername:   &ownerUsername,
				SamplePrep:      "Cleaned with piranha solution",
				AttachmentCount: 1,
				CreatedAt:       time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
				Steps: []samples.APIFabricationStep{{
					ID:             10,
					SampleID:       42,
					StepOrder:      1,
					StepDate:       &stepDate,
					PersonUserID:   &ownerUserID,
					PersonUsername: &personUsername,
					Description:    "RCA clean",
					EquipmentID:    intPtr(5),
					EquipmentName:  &equipmentName,
					CreatedAt:      time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC),
				}},
				Attachments: []samples.APIAttachment{{
					ID:           20,
					OriginalName: "wafer.jpg",
					URL:          "/attachment/20",
					ContentType:  "image/jpeg",
					IsImage:      true,
					UploadedAt:   time.Date(2026, 1, 15, 10, 5, 0, 0, time.UTC),
				}},
			}, nil
		},
	})

	req := authenticatedRequest(http.MethodGet, "/api/v1/samples/42", nil, "", auth.Session{UserID: 1, Username: "tester"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	resp := decodeResponse[samples.APISampleDetail](t, rr.Body)
	if resp.SamplePrep != "Cleaned with piranha solution" {
		t.Fatalf("sample_prep = %q, want updated value", resp.SamplePrep)
	}
	if len(resp.Steps) != 1 || len(resp.Attachments) != 1 {
		t.Fatalf("unexpected nested detail: %#v", resp)
	}
}

func TestAPISamples_CreateSample_AcceptsSamplePrep(t *testing.T) {
	ownerID := 7
	var captured struct {
		Name        string
		Description string
		Keywords    string
		Owner       string
		SamplePrep  string
		OwnerUserID *int
	}

	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		isAdmin: func(_ context.Context, userID int) (bool, error) {
			if userID != 1 {
				t.Fatalf("IsAdmin called with userID=%d, want 1", userID)
			}
			return true, nil
		},
		apiCreateSample: func(_ context.Context, name, description, keywords, owner, samplePrep string, ownerUserID *int) (samples.APISampleItem, error) {
			captured = struct {
				Name        string
				Description string
				Keywords    string
				Owner       string
				SamplePrep  string
				OwnerUserID *int
			}{name, description, keywords, owner, samplePrep, ownerUserID}
			return samples.APISampleItem{
				ID:              42,
				Name:            name,
				Description:     description,
				Keywords:        keywords,
				Owner:           owner,
				OwnerUserID:     ownerUserID,
				AttachmentCount: 0,
				CreatedAt:       time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	body := strings.NewReader(`{"name":"Si wafer","description":"4-inch p-type silicon","keywords":"silicon wafer substrate","owner":"alice","owner_user_id":7,"sample_prep":"Optional preparation notes in Markdown"}`)
	req := authenticatedRequest(http.MethodPost, "/api/v1/samples", body, "application/json", auth.Session{UserID: 1, Username: "admin"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	if captured.SamplePrep != "Optional preparation notes in Markdown" {
		t.Fatalf("sample_prep = %q, want request value", captured.SamplePrep)
	}
	if captured.OwnerUserID == nil || *captured.OwnerUserID != ownerID {
		t.Fatalf("owner_user_id = %#v, want %d", captured.OwnerUserID, ownerID)
	}

	resp := decodeResponse[samples.APISampleItem](t, rr.Body)
	if resp.ID != 42 || resp.AttachmentCount != 0 {
		t.Fatalf("unexpected create response: %#v", resp)
	}
}

func TestAPISamples_UpdateSample_ReturnsUpdatedDetail(t *testing.T) {
	ownerID := 7
	ownerUsername := "alice"
	var captured struct {
		ID          int
		SamplePrep  string
		OwnerUserID *int
	}

	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		checkOwner: func(_ context.Context, sampleID, userID int) error {
			if sampleID != 42 || userID != 1 {
				t.Fatalf("CheckOwner called with sampleID=%d userID=%d", sampleID, userID)
			}
			return nil
		},
		isAdmin: func(_ context.Context, _ int) (bool, error) { return true, nil },
		apiUpdateSample: func(_ context.Context, id int, _, _, _, _, samplePrep string, ownerUserID *int) error {
			captured.ID = id
			captured.SamplePrep = samplePrep
			captured.OwnerUserID = ownerUserID
			return nil
		},
		apiGetSample: func(_ context.Context, id int) (samples.APISampleDetail, error) {
			return samples.APISampleDetail{
				ID:            id,
				Name:          "Si wafer",
				Owner:         "alice",
				OwnerUserID:   &ownerID,
				OwnerUsername: &ownerUsername,
				SamplePrep:    "Updated prep",
			}, nil
		},
	})

	body := strings.NewReader(`{"name":"Si wafer","description":"updated","keywords":"silicon","owner":"alice","owner_user_id":7,"sample_prep":"Updated prep"}`)
	req := authenticatedRequest(http.MethodPut, "/api/v1/samples/42", body, "application/json", auth.Session{UserID: 1, Username: "admin"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if captured.ID != 42 || captured.SamplePrep != "Updated prep" {
		t.Fatalf("unexpected update args: %#v", captured)
	}
	if captured.OwnerUserID == nil || *captured.OwnerUserID != ownerID {
		t.Fatalf("unexpected owner_user_id: %#v", captured.OwnerUserID)
	}

	resp := decodeResponse[samples.APISampleDetail](t, rr.Body)
	if resp.SamplePrep != "Updated prep" {
		t.Fatalf("sample_prep = %q, want updated value", resp.SamplePrep)
	}
}

func TestAPISamples_DeleteSample_RemovesAttachmentFiles(t *testing.T) {
	h, baseDir := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		checkOwner: func(_ context.Context, _, _ int) error { return nil },
		apiDeleteSample: func(_ context.Context, id int) ([]string, error) {
			if id != 42 {
				t.Fatalf("APIDeleteSample called with id=%d, want 42", id)
			}
			return []string{"uploads/to-delete.txt"}, nil
		},
	})

	targetPath := filepath.Join(baseDir, "uploads", "to-delete.txt")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("payload"), 0644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	req := authenticatedRequest(http.MethodDelete, "/api/v1/samples/42", nil, "", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat err = %v", targetPath, err)
	}
}

func TestAPISteps_ListSteps_ReturnsEnvelope(t *testing.T) {
	stepDate := time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)
	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		sampleExistsByID: func(_ context.Context, id int) (bool, error) {
			return id == 42, nil
		},
		getSteps: func(_ context.Context, sampleID int) ([]samples.FabricationStep, error) {
			if sampleID != 42 {
				t.Fatalf("GetSteps called with sampleID=%d, want 42", sampleID)
			}
			return []samples.FabricationStep{
				{ID: 1, SampleID: 42, StepOrder: 1, StepDate: &stepDate, Description: "RCA clean"},
				{ID: 2, SampleID: 42, StepOrder: 2, Description: "Dry"},
			}, nil
		},
	})

	req := authenticatedRequest(http.MethodGet, "/api/v1/samples/42/steps", nil, "", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp struct {
		Items  []samples.APIFabricationStep `json:"items"`
		Total  int                          `json:"total"`
		Limit  int                          `json:"limit"`
		Offset int                          `json:"offset"`
	}
	resp = decodeResponse[struct {
		Items  []samples.APIFabricationStep `json:"items"`
		Total  int                          `json:"total"`
		Limit  int                          `json:"limit"`
		Offset int                          `json:"offset"`
	}](t, rr.Body)

	if resp.Total != 2 || resp.Limit != 0 || resp.Offset != 0 {
		t.Fatalf("unexpected envelope: %#v", resp)
	}
	if len(resp.Items) != 2 || resp.Items[0].StepDate == nil || *resp.Items[0].StepDate != "2026-01-14" {
		t.Fatalf("unexpected items: %#v", resp.Items)
	}
}

func TestAPISteps_AddStep_ReturnsCreatedStep(t *testing.T) {
	var captured samples.FabricationStep
	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		checkOwner: func(_ context.Context, sampleID, userID int) error {
			if sampleID != 42 || userID != 1 {
				t.Fatalf("CheckOwner called with sampleID=%d userID=%d", sampleID, userID)
			}
			return nil
		},
		addStep: func(_ context.Context, sampleID int, step samples.FabricationStep) (samples.FabricationStep, error) {
			captured = step
			step.ID = 9
			step.SampleID = sampleID
			step.CreatedAt = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
			return step, nil
		},
	})

	body := strings.NewReader(`{"step_order":1,"step_date":"2026-01-14","person_user_id":3,"description":"RCA clean","equipment_id":5}`)
	req := authenticatedRequest(http.MethodPost, "/api/v1/samples/42/steps", body, "application/json", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	if captured.StepDate == nil || captured.StepDate.Format("2006-01-02") != "2026-01-14" {
		t.Fatalf("expected parsed step date, got %#v", captured.StepDate)
	}

	resp := decodeResponse[samples.APIFabricationStep](t, rr.Body)
	if resp.ID != 9 || resp.SampleID != 42 || resp.StepDate == nil || *resp.StepDate != "2026-01-14" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestAPISteps_UpdateStep_ReturnsUpdatedStep(t *testing.T) {
	var captured samples.FabricationStep
	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		getStepByID: func(_ context.Context, stepID int) (samples.FabricationStep, error) {
			return samples.FabricationStep{ID: stepID, SampleID: 42}, nil
		},
		checkOwner: func(_ context.Context, _, _ int) error { return nil },
		updateStep: func(_ context.Context, stepID int, step samples.FabricationStep) error {
			if stepID != 9 {
				t.Fatalf("UpdateStep called with stepID=%d, want 9", stepID)
			}
			captured = step
			return nil
		},
	})

	body := strings.NewReader(`{"step_order":2,"step_date":"2026-01-16","description":"Anneal"}`)
	req := authenticatedRequest(http.MethodPut, "/api/v1/steps/9", body, "application/json", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APISteps(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse[samples.APIFabricationStep](t, rr.Body)
	if resp.ID != 9 || resp.SampleID != 42 {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if captured.StepDate == nil || captured.StepDate.Format("2006-01-02") != "2026-01-16" {
		t.Fatalf("unexpected captured step: %#v", captured)
	}
}

func TestAPISteps_DeleteStep_ReturnsNoContent(t *testing.T) {
	var deletedID int
	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		getStepByID: func(_ context.Context, stepID int) (samples.FabricationStep, error) {
			return samples.FabricationStep{ID: stepID, SampleID: 42}, nil
		},
		checkOwner: func(_ context.Context, _, _ int) error { return nil },
		deleteStep: func(_ context.Context, stepID int) error {
			deletedID = stepID
			return nil
		},
	})

	req := authenticatedRequest(http.MethodDelete, "/api/v1/steps/9", nil, "", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APISteps(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if deletedID != 9 {
		t.Fatalf("deleted step id = %d, want 9", deletedID)
	}
}

func TestAPIAttachments_ListAttachments_ReturnsEnvelope(t *testing.T) {
	h, _ := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		sampleExistsByID: func(_ context.Context, id int) (bool, error) { return id == 42, nil },
		getAttachments: func(_ context.Context, sampleID int) ([]samples.Attachment, error) {
			if sampleID != 42 {
				t.Fatalf("GetAttachments called with sampleID=%d, want 42", sampleID)
			}
			return []samples.Attachment{
				{
					ID:           20,
					SampleID:     42,
					Address:      "uploads/wafer.jpg",
					OriginalName: "wafer.jpg",
					ContentType:  "image/jpeg",
					IsImage:      true,
					UploadedAt:   time.Date(2026, 1, 15, 10, 5, 0, 0, time.UTC),
				},
				{
					ID:           21,
					SampleID:     42,
					Address:      "uploads/datasheet.pdf",
					OriginalName: "datasheet.pdf",
					ContentType:  "application/pdf",
					IsImage:      false,
					UploadedAt:   time.Date(2026, 1, 15, 10, 6, 0, 0, time.UTC),
				},
			}, nil
		},
	})

	req := authenticatedRequest(http.MethodGet, "/api/v1/samples/42/attachments", nil, "", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp struct {
		Items  []samples.APIAttachment `json:"items"`
		Total  int                     `json:"total"`
		Limit  int                     `json:"limit"`
		Offset int                     `json:"offset"`
	}
	resp = decodeResponse[struct {
		Items  []samples.APIAttachment `json:"items"`
		Total  int                     `json:"total"`
		Limit  int                     `json:"limit"`
		Offset int                     `json:"offset"`
	}](t, rr.Body)

	if resp.Total != 2 || resp.Limit != 0 || resp.Offset != 0 {
		t.Fatalf("unexpected envelope: %#v", resp)
	}
	if len(resp.Items) != 2 || resp.Items[0].OriginalName != "wafer.jpg" {
		t.Fatalf("unexpected items: %#v", resp.Items)
	}
}

func TestAPIAttachments_UploadAttachments_ReturnsCreatedRecords(t *testing.T) {
	var addedPaths []string
	var baseDir string
	stub := &sampleAPIServiceStub{
		checkOwner: func(_ context.Context, _, _ int) error { return nil },
		addAttachment: func(_ context.Context, sampleID int, path string) (int, error) {
			if sampleID != 42 {
				t.Fatalf("AddAttachment called with sampleID=%d, want 42", sampleID)
			}
			if _, err := os.Stat(filepath.Join(baseDir, path)); err != nil {
				t.Fatalf("expected uploaded file at %s: %v", path, err)
			}
			addedPaths = append(addedPaths, path)
			// Return IDs 20, 21 for the first and second file respectively.
			return 19 + len(addedPaths), nil
		},
		getAttachments: func(_ context.Context, sampleID int) ([]samples.Attachment, error) {
			if len(addedPaths) != 2 {
				t.Fatalf("expected 2 added paths, got %d", len(addedPaths))
			}
			return []samples.Attachment{
				{
					ID:           21,
					SampleID:     sampleID,
					Address:      addedPaths[1],
					OriginalName: "datasheet.pdf",
					ContentType:  "application/pdf",
					IsImage:      false,
					UploadedAt:   time.Date(2026, 4, 17, 10, 5, 0, 0, time.UTC),
				},
				{
					ID:           20,
					SampleID:     sampleID,
					Address:      addedPaths[0],
					OriginalName: "wafer.jpg",
					ContentType:  "image/jpeg",
					IsImage:      true,
					UploadedAt:   time.Date(2026, 4, 17, 10, 4, 0, 0, time.UTC),
				},
			}, nil
		},
	}
	h, baseDir := newSampleAPIHandlers(t, stub)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part1, err := writer.CreateFormFile("files", "wafer.jpg")
	if err != nil {
		t.Fatalf("CreateFormFile(wafer.jpg): %v", err)
	}
	if _, err := part1.Write([]byte("image-bytes")); err != nil {
		t.Fatalf("write wafer.jpg: %v", err)
	}
	part2, err := writer.CreateFormFile("files", "datasheet.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile(datasheet.pdf): %v", err)
	}
	if _, err := part2.Write([]byte("%PDF-1.4")); err != nil {
		t.Fatalf("write datasheet.pdf: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	req := authenticatedRequest(http.MethodPost, "/api/v1/samples/42/attachments", &body, writer.FormDataContentType(), auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APISamples(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	if len(addedPaths) != 2 {
		t.Fatalf("expected 2 stored files, got %d", len(addedPaths))
	}

	var resp struct {
		Created []samples.APIAttachment `json:"created"`
		Errors  []map[string]string     `json:"errors"`
	}
	resp = decodeResponse[struct {
		Created []samples.APIAttachment `json:"created"`
		Errors  []map[string]string     `json:"errors"`
	}](t, rr.Body)

	if len(resp.Created) != 2 || len(resp.Errors) != 0 {
		t.Fatalf("unexpected upload response: %#v", resp)
	}
	if resp.Created[0].ID != 21 || resp.Created[1].ID != 20 {
		t.Fatalf("unexpected created ordering: %#v", resp.Created)
	}
}

func TestAPIAttachments_DownloadAttachment_ReturnsBytesAndInlineDisposition(t *testing.T) {
	h, baseDir := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		attachmentPath: func(_ context.Context, attachmentID int) (string, error) {
			if attachmentID != 20 {
				t.Fatalf("AttachmentFilePath called with attachmentID=%d, want 20", attachmentID)
			}
			return "uploads/wafer.jpg", nil
		},
	})

	targetPath := filepath.Join(baseDir, "uploads", "wafer.jpg")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("image-bytes"), 0644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	req := authenticatedRequest(http.MethodGet, "/api/v1/attachments/20", nil, "", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APIAttachments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Body.String() != "image-bytes" {
		t.Fatalf("unexpected body %q", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") || !strings.Contains(got, "wafer.jpg") {
		t.Fatalf("unexpected Content-Disposition %q", got)
	}
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("unexpected Content-Type %q", got)
	}
}

func TestAPIAttachments_DeleteAttachment_RemovesFile(t *testing.T) {
	h, baseDir := newSampleAPIHandlers(t, &sampleAPIServiceStub{
		attachmentInfo: func(_ context.Context, attachmentID int) (int, string, error) {
			if attachmentID != 20 {
				t.Fatalf("AttachmentInfo called with attachmentID=%d, want 20", attachmentID)
			}
			return 42, "uploads/datasheet.pdf", nil
		},
		checkOwner: func(_ context.Context, sampleID, userID int) error {
			if sampleID != 42 || userID != 1 {
				t.Fatalf("CheckOwner called with sampleID=%d userID=%d", sampleID, userID)
			}
			return nil
		},
		deleteAttachment: func(_ context.Context, attachmentID int) (string, error) {
			return "uploads/datasheet.pdf", nil
		},
	})

	targetPath := filepath.Join(baseDir, "uploads", "datasheet.pdf")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("%PDF-1.4"), 0644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	req := authenticatedRequest(http.MethodDelete, "/api/v1/attachments/20", nil, "", auth.Session{UserID: 1, Username: "owner"})
	rr := httptest.NewRecorder()
	h.APIAttachments(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat err = %v", targetPath, err)
	}
}

func intPtr(v int) *int { return &v }
