package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sampleDB/internal/auth"
	"time"
)

// ---------------------------------------------------------------------------
// wikiPreviewHandler — unit tests (no DB required)
// ---------------------------------------------------------------------------

// newPreviewRequest builds a request with a test session in context.
func newPreviewRequest(method, body, contentType string) *http.Request {
	var reqBody *bytes.Reader
	if body != "" {
		reqBody = bytes.NewReader([]byte(body))
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/wiki/preview", reqBody)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	s := auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}
	return req.WithContext(auth.ContextWithSession(req.Context(), s))
}

func TestWikiPreview_MethodNotAllowed(t *testing.T) {
	req := newPreviewRequest(http.MethodGet, "", "")
	rr := httptest.NewRecorder()
	wikiPreviewHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /wiki/preview: status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestWikiPreview_JSONBody_ReturnsHTML(t *testing.T) {
	body := `{"content":"# Hello\n\nThis is **bold**."}`
	req := newPreviewRequest(http.MethodPost, body, "application/json")
	rr := httptest.NewRecorder()
	wikiPreviewHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	html := rr.Body.String()
	if !strings.Contains(html, "<h1") {
		t.Errorf("rendered HTML missing <h1>, got: %q", html)
	}
	if !strings.Contains(html, "<strong>") {
		t.Errorf("rendered HTML missing <strong> for bold, got: %q", html)
	}
}

func TestWikiPreview_FormBody_ReturnsHTML(t *testing.T) {
	body := "content=## Heading%0A%0AParagraph+text"
	req := newPreviewRequest(http.MethodPost, body, "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	wikiPreviewHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestWikiPreview_EmptyContent_ReturnsOK(t *testing.T) {
	body := `{"content":""}`
	req := newPreviewRequest(http.MethodPost, body, "application/json")
	rr := httptest.NewRecorder()
	wikiPreviewHandler(rr, req)

	// renderMarkdown returns empty string for empty input; handler should still be 200
	if rr.Code != http.StatusOK {
		t.Errorf("empty content: status = %d, want 200", rr.Code)
	}
}

func TestWikiPreview_InvalidJSON_Returns400(t *testing.T) {
	body := `{not valid json`
	req := newPreviewRequest(http.MethodPost, body, "application/json")
	rr := httptest.NewRecorder()
	wikiPreviewHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON: status = %d, want 400", rr.Code)
	}
}

func TestWikiPreview_CodeBlock_ReturnsHTMLWithPre(t *testing.T) {
	body := `{"content":"` + "```go\\nfmt.Println(\\\"hello\\\")\\n```" + `"}`
	req := newPreviewRequest(http.MethodPost, body, "application/json")
	rr := httptest.NewRecorder()
	wikiPreviewHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("code block: status = %d, want 200", rr.Code)
	}
	html := rr.Body.String()
	if !strings.Contains(html, "<pre>") && !strings.Contains(html, "<code") {
		t.Errorf("code block: expected <pre> or <code> in output, got: %q", html)
	}
}
