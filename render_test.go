package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Template render tests — verify templates execute without error with
// representative mock data. No DB or HTTP required.
// ---------------------------------------------------------------------------

func TestRenderMainTemplate(t *testing.T) {
	setTestTemplatesDir(t)

	now := time.Now()
	imgID := 42
	ownerID := 1
	username := "alice"

	data := MainPageData{
		BasePageData: BasePageData{
			Username: "testuser",
			UserID:   1,
			IsAdmin:  false,
		},
		Query:    "test",
		DateFrom: "2024-01-01",
		DateTo:   "2024-12-31",
		Samples: []Sample{
			{
				ID:            1,
				Name:          "Sample Alpha",
				Description:   "A test sample",
				Keywords:      "alpha, test",
				Owner:         "alice",
				OwnerUserID:   &ownerID,
				OwnerUsername: &username,
				CreatedAt:     now,
				FirstImageID:  &imgID,
			},
			{
				ID:          2,
				Name:        "Sample Beta",
				Description: "",
				Keywords:    "",
				Owner:       "",
				CreatedAt:   now,
				// No image
			},
		},
	}

	w := httptest.NewRecorder()
	if err := theApp.Renderer.Render(w, "main", data); err != nil {
		t.Fatalf("Render(main): %v", err)
	}

	html := w.Body.String()
	if !strings.Contains(html, "Sample Alpha") {
		t.Error("rendered HTML missing 'Sample Alpha'")
	}
	if !strings.Contains(html, "Sample Beta") {
		t.Error("rendered HTML missing 'Sample Beta'")
	}
	// Thumbnail should render for the first sample
	if !strings.Contains(html, "/attachment/42") {
		t.Error("rendered HTML missing thumbnail src '/attachment/42'")
	}
}

func TestRenderMainTemplate_Empty(t *testing.T) {
	setTestTemplatesDir(t)

	data := MainPageData{
		BasePageData: BasePageData{Username: "testuser", UserID: 1},
		Samples:      nil,
	}

	w := httptest.NewRecorder()
	if err := theApp.Renderer.Render(w, "main", data); err != nil {
		t.Fatalf("Render(main) with empty samples: %v", err)
	}

	if !strings.Contains(w.Body.String(), "No samples found") {
		t.Error("empty-state message not found in rendered HTML")
	}
}

func TestRenderSampleDetailTemplate(t *testing.T) {
	setTestTemplatesDir(t)

	now := time.Now()
	ownerID := 3
	username := "bob"
	stepDate := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)
	equipID := 7

	data := SampleDetailPageData{
		BasePageData: BasePageData{
			Username: "testuser",
			UserID:   1,
			IsAdmin:  false,
		},
		Sample: Sample{
			ID:              5,
			Name:            "Detail Sample",
			Description:     "Detailed description",
			Keywords:        "detail, render",
			Owner:           "bob",
			OwnerUserID:     &ownerID,
			OwnerUsername:   &username,
			CreatedAt:       now,
			ShareVisibility: "private",
			Attachments: []Attachment{
				{ID: 10, OriginalName: "image.png", ContentType: "image/png", IsImage: true},
				{ID: 11, OriginalName: "data.csv", ContentType: "text/csv", IsImage: false},
			},
		},
		Steps: []FabricationStep{
			{
				ID:          1,
				SampleID:    5,
				StepOrder:   1,
				StepDate:    &stepDate,
				Description: "First step description",
			},
		},
		Users: []User{
			{ID: 3, Username: "bob"},
		},
		Equipment: []Equipment{
			{ID: 7, Name: "Spin Coater"},
		},
		EditingStep: 0,
	}
	_ = equipID

	w := httptest.NewRecorder()
	if err := theApp.Renderer.Render(w, "sample_detail", data); err != nil {
		t.Fatalf("Render(sample_detail): %v", err)
	}

	html := w.Body.String()
	if !strings.Contains(html, "Detail Sample") {
		t.Error("rendered HTML missing sample name 'Detail Sample'")
	}
	if !strings.Contains(html, "image.png") {
		t.Error("rendered HTML missing attachment 'image.png'")
	}
	if !strings.Contains(html, "data.csv") {
		t.Error("rendered HTML missing attachment 'data.csv'")
	}
	if !strings.Contains(html, "First step description") {
		t.Error("rendered HTML missing fabrication step description")
	}
	// Collapsible panel summaries should be present
	if !strings.Contains(html, "Fabrication Steps") {
		t.Error("rendered HTML missing 'Fabrication Steps' panel summary")
	}
	if !strings.Contains(html, "Attachments") {
		t.Error("rendered HTML missing 'Attachments' panel summary")
	}
}
