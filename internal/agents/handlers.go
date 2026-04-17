// Package agents provides HTTP handlers for the public /agents pages.
package agents

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"sampleDB/internal/web"
)

// ExamplesPageData holds the pre-rendered HTML blocks for each example section.
type ExamplesPageData struct {
	DataProcessingHTML      template.HTML
	ScriptingAutomationHTML template.HTML
	SoftwareDevelopmentHTML template.HTML
}

// Handlers holds pre-parsed templates and the path to the examples directory.
// A nil *Handlers is safe: all HTTP methods return 503 when the receiver is nil.
type Handlers struct {
	agentsTmpl   *template.Template
	examplesTmpl *template.Template
	examplesDir  string
}

// New parses the AI-agents page templates from templatesDir and returns a
// ready-to-use Handlers. examplesDir must point to the directory containing
// the example markdown files.
func New(templatesDir, examplesDir string) (*Handlers, error) {
	agentsTmpl, err := template.ParseFiles(filepath.Join(templatesDir, "ai_agents.html"))
	if err != nil {
		return nil, err
	}
	examplesTmpl, err := template.ParseFiles(filepath.Join(templatesDir, "ai_agent_examples.html"))
	if err != nil {
		return nil, err
	}
	return &Handlers{
		agentsTmpl:   agentsTmpl,
		examplesTmpl: examplesTmpl,
		examplesDir:  examplesDir,
	}, nil
}

// AgentsPage renders the ai_agents.html template.
// Returns 503 when called on a nil receiver.
func (h *Handlers) AgentsPage(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.agentsTmpl == nil {
		http.Error(w, "agents not configured", http.StatusServiceUnavailable)
		return
	}
	if err := h.agentsTmpl.Execute(w, nil); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// ExamplesPage renders the ai_agent_examples.html template with pre-rendered
// markdown content. Non-canonical paths are redirected to /agents/examples.
// Returns 503 when called on a nil receiver.
func (h *Handlers) ExamplesPage(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.examplesTmpl == nil {
		http.Error(w, "agents not configured", http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path != "/agents/examples" && r.URL.Path != "/agents/examples/" {
		http.Redirect(w, r, "/agents/examples", http.StatusTemporaryRedirect)
		return
	}

	dataProcessingMD, err := os.ReadFile(filepath.Join(h.examplesDir, "data_processing.md"))
	if err != nil {
		http.Error(w, "Error loading example content", http.StatusInternalServerError)
		return
	}
	scriptingMD, err := os.ReadFile(filepath.Join(h.examplesDir, "scripting_automation.md"))
	if err != nil {
		http.Error(w, "Error loading example content", http.StatusInternalServerError)
		return
	}
	softwareMD, err := os.ReadFile(filepath.Join(h.examplesDir, "software_development.md"))
	if err != nil {
		http.Error(w, "Error loading example content", http.StatusInternalServerError)
		return
	}

	pageData := ExamplesPageData{
		DataProcessingHTML:      web.RenderMarkdown(rewriteRelativeImages(string(dataProcessingMD), "/static/examples")),
		ScriptingAutomationHTML: web.RenderMarkdown(rewriteRelativeImages(string(scriptingMD), "/static/examples")),
		SoftwareDevelopmentHTML: web.RenderMarkdown(rewriteRelativeImages(string(softwareMD), "/static/examples")),
	}
	if err := h.examplesTmpl.Execute(w, pageData); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}
