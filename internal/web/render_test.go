package web_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/samples"
	"sampleDB/internal/web"
	"sampleDB/internal/wiki"
)

// testTemplatesDir returns the repo-root templates directory.
// Tests run with the package directory as CWD; templates are two levels up.
func testTemplatesDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := filepath.Join(wd, "../../templates")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("templates directory not found at %s — skipping render test", dir)
	}
	return dir
}

func TestRenderMainTemplate(t *testing.T) {
	dir := testTemplatesDir(t)
	renderer, err := web.NewRenderer(dir, nil, time.UTC)
	if err != nil {
		t.Fatalf("web.NewRenderer: %v", err)
	}

	now := time.Now()
	imgID := 42
	ownerID := 1
	username := "alice"

	data := samples.MainPageData{
		BasePageData: samples.BasePageData{
			Username: "testuser",
			UserID:   1,
			IsAdmin:  false,
		},
		Query:    "test",
		DateFrom: "2024-01-01",
		DateTo:   "2024-12-31",
		Samples: []samples.Sample{
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
				ID:        2,
				Name:      "Sample Beta",
				CreatedAt: now,
			},
		},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "main", data); err != nil {
		t.Fatalf("Render(main): %v", err)
	}

	html := w.Body.String()
	if !strings.Contains(html, "Sample Alpha") {
		t.Error("rendered HTML missing 'Sample Alpha'")
	}
	if !strings.Contains(html, "Sample Beta") {
		t.Error("rendered HTML missing 'Sample Beta'")
	}
	if !strings.Contains(html, "/attachment/42") {
		t.Error("rendered HTML missing thumbnail src '/attachment/42'")
	}
}

func TestRenderMainTemplate_Empty(t *testing.T) {
	dir := testTemplatesDir(t)
	renderer, err := web.NewRenderer(dir, nil, time.UTC)
	if err != nil {
		t.Fatalf("web.NewRenderer: %v", err)
	}

	data := samples.MainPageData{
		BasePageData: samples.BasePageData{Username: "testuser", UserID: 1},
		Samples:      nil,
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "main", data); err != nil {
		t.Fatalf("Render(main) with empty samples: %v", err)
	}

	if !strings.Contains(w.Body.String(), "No samples found") {
		t.Error("empty-state message not found in rendered HTML")
	}
}

func TestRenderSampleDetailTemplate(t *testing.T) {
	dir := testTemplatesDir(t)
	renderer, err := web.NewRenderer(dir, nil, time.UTC)
	if err != nil {
		t.Fatalf("web.NewRenderer: %v", err)
	}

	now := time.Now()
	ownerID := 3
	username := "bob"
	stepDate := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)

	data := samples.SampleDetailPageData{
		BasePageData: samples.BasePageData{
			Username: "testuser",
			UserID:   1,
			IsAdmin:  false,
		},
		Sample: samples.Sample{
			ID:              5,
			Name:            "Detail Sample",
			Description:     "Detailed description",
			Keywords:        "detail, render",
			Owner:           "bob",
			OwnerUserID:     &ownerID,
			OwnerUsername:   &username,
			CreatedAt:       now,
			ShareVisibility: "private",
			Attachments: []samples.Attachment{
				{ID: 10, OriginalName: "image.png", ContentType: "image/png", IsImage: true},
				{ID: 11, OriginalName: "data.csv", ContentType: "text/csv", IsImage: false},
			},
		},
		Steps: []samples.FabricationStep{
			{
				ID:          1,
				SampleID:    5,
				StepOrder:   1,
				StepDate:    &stepDate,
				Description: "First step description",
			},
		},
		Users:     []samples.User{{ID: 3, Username: "bob"}},
		Equipment: []samples.Equipment{{ID: 7, Name: "Spin Coater"}},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "sample_detail", data); err != nil {
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
	if !strings.Contains(html, "Fabrication Steps") {
		t.Error("rendered HTML missing 'Fabrication Steps' panel summary")
	}
	if !strings.Contains(html, "Attachments") {
		t.Error("rendered HTML missing 'Attachments' panel summary")
	}
}

func TestRenderWikiEditTemplate_PreviewFetchSendsCSRFHeader(t *testing.T) {
	dir := testTemplatesDir(t)
	renderer, err := web.NewRenderer(dir, nil, time.UTC)
	if err != nil {
		t.Fatalf("web.NewRenderer: %v", err)
	}

	data := wiki.WikiEditPageData{
		BasePageData: wiki.BasePageData{
			Username:  "testuser",
			UserID:    1,
			CSRFToken: "csrf-token-123",
		},
		Article: &wiki.Article{
			ID:      1,
			Title:   "Preview Test",
			Content: wiki.ArticleContent{Raw: "# Heading"},
		},
	}

	w := httptest.NewRecorder()
	if err := renderer.Render(w, "wiki_edit", data); err != nil {
		t.Fatalf("Render(wiki_edit): %v", err)
	}

	html := w.Body.String()
	for _, fragment := range []string{
		`meta name="csrf-token" content="csrf-token-123"`,
		`headers['X-CSRF-Token'] = csrfMeta.content`,
		`fetch('/wiki/preview'`,
	} {
		if !strings.Contains(html, fragment) {
			t.Fatalf("rendered wiki edit page missing %q", fragment)
		}
	}
}
