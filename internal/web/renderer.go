// Package web provides HTTP helpers and template rendering utilities.
package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Renderer caches parsed template groups and executes them by name.
// It is safe for concurrent use by multiple goroutines.
// The zero value is a valid, empty Renderer — call Register to add groups.
type Renderer struct {
	mu     sync.RWMutex
	groups map[string]*template.Template
}

// Register adds (or replaces) a named template group in the renderer.
// It is safe to call concurrently with Render/RenderSection.
func (r *Renderer) Register(name string, tmpl *template.Template) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.groups == nil {
		r.groups = make(map[string]*template.Template)
	}
	r.groups[name] = tmpl
}

// Render executes the "base" named template of the group identified by name
// and writes the result to w.
func (r *Renderer) Render(w http.ResponseWriter, name string, data interface{}) error {
	return r.RenderSection(w, name, "base", data)
}

// RenderSection executes the named section within the template group and
// writes the result to w. Use this for HTMX partial responses.
func (r *Renderer) RenderSection(w http.ResponseWriter, name, section string, data interface{}) error {
	r.mu.RLock()
	tmpl, ok := r.groups[name]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("web: unknown template group %q", name)
	}
	return tmpl.ExecuteTemplate(w, section, data)
}

// NewRenderer parses all standard template groups from templatesDir and
// returns a ready-to-use Renderer.
//
// extraFuncs may supply additional template functions (e.g. "markdown") that
// the web package cannot provide itself; pass nil if not needed.
// loc is used by the booking template's combineDatetime function.
//
// If templatesDir is empty, "templates" is used as the default.
func NewRenderer(templatesDir string, extraFuncs template.FuncMap, loc *time.Location) (*Renderer, error) {
	if templatesDir == "" {
		templatesDir = "templates"
	}
	if loc == nil {
		loc = time.UTC
	}

	mainFM := buildMainFuncMap(extraFuncs)
	bookingFM := buildBookingFuncMap(loc)

	type group struct {
		name    string
		funcMap template.FuncMap
		files   []string // relative to templatesDir, without base/header
	}

	groups := []group{
		{"main", mainFM, []string{"main.html"}},
		{"change_password", mainFM, []string{"change_password.html"}},
		{"profile_tokens", mainFM, []string{"profile_tokens.html"}},
		{"new_sample", mainFM, []string{"new_sample.html"}},
		{"sample_detail", mainFM, []string{
			"sample_detail.html",
			"fabrication_steps.html",
			"step_form.html",
		}},
		{"sample_shared", mainFM, []string{
			"sample_shared.html",
			"fabrication_steps.html",
			"step_form.html",
		}},
		{"admin", mainFM, []string{
			"admin.html",
			"admin/groups.html",
			"admin/users.html",
		}},
		{"wiki_list", mainFM, []string{"wiki_list.html"}},
		{"wiki_view", mainFM, []string{"wiki_view.html"}},
		{"wiki_edit", mainFM, []string{"wiki_edit.html"}},
		{"booking", bookingFM, []string{"booking.html"}},
	}

	r := &Renderer{}
	for _, g := range groups {
		// Always include base.html and header.html first.
		all := append([]string{"base.html", "header.html"}, g.files...)
		resolved := make([]string, len(all))
		for i, f := range all {
			resolved[i] = filepath.Join(templatesDir, f)
		}
		tmpl, err := template.New("").Funcs(g.funcMap).ParseFiles(resolved...)
		if err != nil {
			return nil, fmt.Errorf("web: parsing template group %q: %w", g.name, err)
		}
		r.Register(g.name, tmpl)
	}
	return r, nil
}

// buildMainFuncMap returns the template.FuncMap used by all non-booking groups.
// A minimal "markdown" stub is provided by default so templates parse without
// the gomarkdown dependency; callers may override it via extraFuncs (e.g.
// {"markdown": renderMarkdownFn}) to get real rendering.
func buildMainFuncMap(extraFuncs template.FuncMap) template.FuncMap {
	fm := template.FuncMap{
		// markdown default: pass-through so templates parse in test environments
		// without the gomarkdown library.  Production callers override via extraFuncs.
		"markdown": func(s string) template.HTML { return template.HTML(s) },
		"split":    strings.Split,
		"trim":     strings.TrimSpace,
		"ownerSelected": func(ownerUserID *int, userID int) bool {
			return ownerUserID != nil && *ownerUserID == userID
		},
		"formatDate": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"add": func(a, b int) int { return a + b },
	}
	for k, v := range extraFuncs {
		fm[k] = v
	}
	return fm
}

// buildBookingFuncMap returns the template.FuncMap used by the booking group.
// loc is captured in the combineDatetime closure.
func buildBookingFuncMap(loc *time.Location) template.FuncMap {
	return template.FuncMap{
		"toJSON": func(value interface{}) template.JS {
			data, err := json.Marshal(value)
			if err != nil {
				return template.JS("null")
			}
			if string(data) == "null" {
				return template.JS("[]")
			}
			return template.JS(data)
		},
		"formatDate": func(t time.Time) string {
			return t.Format("January 2, 2006")
		},
		"formatDateShort": func(t time.Time) string {
			return t.Format("Jan 2")
		},
		"formatDateNumeric": func(t time.Time) string {
			return t.Format("01/02/2006")
		},
		"formatDateISO": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		"formatDayName": func(t time.Time) string {
			return t.Format("Monday")
		},
		"formatTime": func(t time.Time) string {
			return t.Format("3:04 PM")
		},
		"formatTime24": func(t time.Time) string {
			return t.Format("15:04")
		},
		"formatDateTime": func(t time.Time) string {
			return t.Format("Jan 2, 15:04")
		},
		"add": func(a, b int) int { return a + b },
		"subtract": func(a, b int) int {
			return a - b
		},
		"iterate": func(start, end int) []int {
			result := make([]int, 0, end-start)
			for i := start; i < end; i++ {
				result = append(result, i)
			}
			return result
		},
		"combineDatetime": func(date, timeSlot time.Time) time.Time {
			return time.Date(
				date.Year(), date.Month(), date.Day(),
				timeSlot.Hour(), timeSlot.Minute(),
				0, 0, loc,
			)
		},
		"formatDateTimeISO": func(t time.Time) string {
			return t.Format("2006-01-02T15:04")
		},
		"addDays": func(t time.Time, days int) time.Time {
			return t.AddDate(0, 0, days)
		},
	}
}
