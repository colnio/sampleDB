package web_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"sampleDB/internal/web"
)

// ---------------------------------------------------------------------------
// errors.go — RenderError status code mapping
// ---------------------------------------------------------------------------

func TestRenderError_SentinelMapping(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
	}{
		{web.ErrNotFound, http.StatusNotFound},
		{web.ErrForbidden, http.StatusForbidden},
		{web.ErrConflict, http.StatusConflict},
		{web.ErrValidation, http.StatusBadRequest},
		// generic error → 500
		{errors.New("unexpected"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		web.RenderError(w, tc.err)
		if w.Code != tc.wantStatus {
			t.Errorf("RenderError(%v): got %d, want %d", tc.err, w.Code, tc.wantStatus)
		}
	}
}

func TestRenderError_WrappedSentinels(t *testing.T) {
	wrapped := func(sentinel error) error {
		return &wrappedErr{msg: "context: something went wrong", cause: sentinel}
	}

	cases := []struct {
		err        error
		wantStatus int
	}{
		{wrapped(web.ErrNotFound), http.StatusNotFound},
		{wrapped(web.ErrForbidden), http.StatusForbidden},
		{wrapped(web.ErrConflict), http.StatusConflict},
		{wrapped(web.ErrValidation), http.StatusBadRequest},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		web.RenderError(w, tc.err)
		if w.Code != tc.wantStatus {
			t.Errorf("RenderError(wrapped %v): got %d, want %d", errors.Unwrap(tc.err), w.Code, tc.wantStatus)
		}
	}
}

// wrappedErr is a helper that satisfies errors.Unwrap.
type wrappedErr struct {
	msg   string
	cause error
}

func (e *wrappedErr) Error() string { return e.msg }
func (e *wrappedErr) Unwrap() error { return e.cause }

// ---------------------------------------------------------------------------
// json.go — JSON write helper
// ---------------------------------------------------------------------------

func TestJSON_WritesStatusAndBody(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]string{"hello": "world"}
	web.JSON(w, http.StatusCreated, payload)

	if w.Code != http.StatusCreated {
		t.Fatalf("JSON: got status %d, want 201", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("JSON: Content-Type %q does not start with application/json", ct)
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("JSON: could not decode body: %v", err)
	}
	if got["hello"] != "world" {
		t.Fatalf("JSON: body mismatch, got %v", got)
	}
}

func TestDecodeJSON_Valid(t *testing.T) {
	body := `{"name":"alice","age":30}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	var dst struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	if err := web.DecodeJSON(r, &dst); err != nil {
		t.Fatalf("DecodeJSON: unexpected error: %v", err)
	}
	if dst.Name != "alice" || dst.Age != 30 {
		t.Fatalf("DecodeJSON: got %+v", dst)
	}
}

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{bad json"))
	if err := web.DecodeJSON(r, &struct{}{}); err == nil {
		t.Fatal("DecodeJSON: expected error for invalid JSON, got nil")
	}
}

func TestDecodeJSON_EmptyBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(nil))
	if err := web.DecodeJSON(r, &struct{}{}); err == nil {
		t.Fatal("DecodeJSON: expected error for empty body, got nil")
	}
}

func TestError_WritesStatusAndBody(t *testing.T) {
	w := httptest.NewRecorder()
	web.Error(w, http.StatusNotFound, "not found")

	if w.Code != http.StatusNotFound {
		t.Fatalf("Error: got status %d, want 404", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Error: Content-Type %q does not start with application/json", ct)
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("Error: could not decode body: %v", err)
	}
	if got["error"] != "not found" {
		t.Fatalf("Error: body mismatch, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// paths.go — path segment helpers
// ---------------------------------------------------------------------------

func TestPathSegments(t *testing.T) {
	cases := []struct {
		path   string
		prefix string
		want   []string
	}{
		{"/samples/123/upload", "/samples/", []string{"123", "upload"}},
		{"/samples/123", "/samples/", []string{"123"}},
		{"/wiki/edit/My%20Article", "/wiki/edit/", []string{"My%20Article"}},
		{"/wiki/", "/wiki/", []string{}},
		{"/", "/", []string{}},
	}
	for _, tc := range cases {
		got := web.PathSegments(tc.path, tc.prefix)
		if len(got) != len(tc.want) {
			t.Errorf("PathSegments(%q, %q): got %v, want %v", tc.path, tc.prefix, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("PathSegments[%d]: got %q, want %q", i, got[i], tc.want[i])
			}
		}
	}
}

func TestExtractIDFromPath(t *testing.T) {
	id, err := web.ExtractIDFromPath("/samples/42", "/samples/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "42" {
		t.Fatalf("got %q, want %q", id, "42")
	}
}

func TestExtractIDFromPath_Empty(t *testing.T) {
	_, err := web.ExtractIDFromPath("/samples/", "/samples/")
	if err == nil {
		t.Fatal("expected error for empty path segment, got nil")
	}
}

func TestExtractIDAndAction(t *testing.T) {
	cases := []struct {
		path       string
		prefix     string
		wantID     string
		wantAction string
	}{
		{"/steps/123/edit", "/steps/", "123", "edit"},
		{"/steps/456", "/steps/", "456", ""},
	}
	for _, tc := range cases {
		id, action, err := web.ExtractIDAndAction(tc.path, tc.prefix)
		if err != nil {
			t.Fatalf("ExtractIDAndAction(%q): unexpected error: %v", tc.path, err)
		}
		if id != tc.wantID {
			t.Errorf("ExtractIDAndAction(%q): id=%q, want %q", tc.path, id, tc.wantID)
		}
		if action != tc.wantAction {
			t.Errorf("ExtractIDAndAction(%q): action=%q, want %q", tc.path, action, tc.wantAction)
		}
	}
}

func TestExtractArticleTitle_URLDecoded(t *testing.T) {
	title, err := web.ExtractArticleTitle("/wiki/delete/My%20Article", "/wiki/delete/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "My Article" {
		t.Fatalf("got %q, want %q", title, "My Article")
	}
}

func TestExtractArticleTitle_Empty(t *testing.T) {
	_, err := web.ExtractArticleTitle("/wiki/delete/", "/wiki/delete/")
	if err == nil {
		t.Fatal("expected error for empty title, got nil")
	}
}

// ---------------------------------------------------------------------------
// renderer.go — Renderer template execution
// ---------------------------------------------------------------------------

func TestRenderer_Render_ExecutesBaseTemplate(t *testing.T) {
	tmpl := template.Must(template.New("base").Parse("hello {{.Name}}"))

	r := &web.Renderer{}
	r.Register("greeting", tmpl)

	w := httptest.NewRecorder()
	err := r.Render(w, "greeting", map[string]string{"Name": "world"})
	if err != nil {
		t.Fatalf("Render: unexpected error: %v", err)
	}
	if got := w.Body.String(); got != "hello world" {
		t.Fatalf("Render: got body %q, want %q", got, "hello world")
	}
}

func TestRenderer_Render_UnknownGroup(t *testing.T) {
	r := &web.Renderer{}
	w := httptest.NewRecorder()
	err := r.Render(w, "nonexistent", nil)
	if err == nil {
		t.Fatal("Render: expected error for unknown group, got nil")
	}
}

func TestRenderer_RenderSection_ExecutesNamedTemplate(t *testing.T) {
	tmpl := template.Must(template.New("base").Parse("full: {{.}}"))
	template.Must(tmpl.New("partial").Parse("partial: {{.}}"))

	r := &web.Renderer{}
	r.Register("page", tmpl)

	w := httptest.NewRecorder()
	err := r.RenderSection(w, "page", "partial", "data")
	if err != nil {
		t.Fatalf("RenderSection: unexpected error: %v", err)
	}
	if got := w.Body.String(); got != "partial: data" {
		t.Fatalf("RenderSection: got body %q, want %q", got, "partial: data")
	}
}

func TestRenderer_Concurrent_Safe(t *testing.T) {
	// Renderer initialized once; many goroutines render concurrently.
	tmpl := template.Must(template.New("base").Parse("{{.}}"))

	r := &web.Renderer{}
	r.Register("test", tmpl)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			if err := r.Render(w, "test", "hello"); err != nil {
				t.Errorf("Render: unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// markdown.go — RenderMarkdown
// ---------------------------------------------------------------------------

func TestRenderMarkdown_BasicContent(t *testing.T) {
	out := web.RenderMarkdown("# Hello\n\nWorld")
	s := string(out)
	if !strings.Contains(s, "Hello") {
		t.Fatalf("expected heading text in output, got %q", s)
	}
	if !strings.Contains(s, "World") {
		t.Fatalf("expected paragraph text in output, got %q", s)
	}
}

func TestRenderMarkdown_Empty(t *testing.T) {
	out := web.RenderMarkdown("   ")
	if out != "" {
		t.Fatalf("expected empty output for blank input, got %q", out)
	}
}
