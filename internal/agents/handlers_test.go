package agents

import (
	"html/template"
	"net/http/httptest"
	"testing"
)

func TestRewriteRelativeImages(t *testing.T) {
	cases := []struct {
		name   string
		md     string
		base   string
		expect string
	}{
		{
			name:   "relative_image",
			md:     "![alt](image.png)",
			base:   "/static/examples",
			expect: "![alt](/static/examples/image.png)",
		},
		{
			name:   "absolute_url_unchanged",
			md:     "![alt](https://example.com/image.png)",
			base:   "/static/examples",
			expect: "![alt](https://example.com/image.png)",
		},
		{
			name:   "slash_prefix_unchanged",
			md:     "![alt](/static/img.png)",
			base:   "/static/examples",
			expect: "![alt](/static/img.png)",
		},
		{
			name:   "dot_slash_stripped",
			md:     "![alt](./image.png)",
			base:   "/static/examples",
			expect: "![alt](/static/examples/image.png)",
		},
		{
			name:   "no_images",
			md:     "just text with no images",
			base:   "/static/examples",
			expect: "just text with no images",
		},
		{
			name:   "trailing_slash_base",
			md:     "![alt](image.png)",
			base:   "/static/examples/",
			expect: "![alt](/static/examples/image.png)",
		},
		{
			name:   "data_url_unchanged",
			md:     "![alt](data:image/png;base64,abc)",
			base:   "/static/examples",
			expect: "![alt](data:image/png;base64,abc)",
		},
		{
			name:   "multiple_images",
			md:     "![a](a.png) text ![b](b.jpg)",
			base:   "/static/examples",
			expect: "![a](/static/examples/a.png) text ![b](/static/examples/b.jpg)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteRelativeImages(tc.md, tc.base)
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestAgentsPage_NilHandler(t *testing.T) {
	var h *Handlers
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents", nil)
	h.AgentsPage(w, req)
	if w.Code != 503 {
		t.Errorf("nil handler: got %d, want 503", w.Code)
	}
}

func TestAgentsPage_TemplateSmoke(t *testing.T) {
	tmpl := template.Must(template.New("ai_agents.html").Parse(`<html>agents</html>`))
	h := &Handlers{agentsTmpl: tmpl}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents", nil)
	h.AgentsPage(w, req)
	if w.Code != 200 {
		t.Errorf("smoke: got %d, want 200", w.Code)
	}
	if w.Body.String() == "" {
		t.Error("smoke: empty response body")
	}
}

func TestExamplesPage_NilHandler(t *testing.T) {
	var h *Handlers
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents/examples", nil)
	h.ExamplesPage(w, req)
	if w.Code != 503 {
		t.Errorf("nil handler: got %d, want 503", w.Code)
	}
}

func TestExamplesPage_RedirectNonCanonical(t *testing.T) {
	tmpl := template.Must(template.New("ai_agent_examples.html").Parse(`<html>examples</html>`))
	h := &Handlers{examplesTmpl: tmpl}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents/examples/extra", nil)
	h.ExamplesPage(w, req)
	if w.Code != 307 {
		t.Errorf("redirect: got %d, want 307", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/agents/examples" {
		t.Errorf("redirect location: got %q, want %q", loc, "/agents/examples")
	}
}
