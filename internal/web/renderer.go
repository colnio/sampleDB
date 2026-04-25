// Package web provides HTTP helpers and template rendering utilities.
package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
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
		{"dashboard", mainFM, []string{"dashboard.html"}},
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
		"toJSON": func(v interface{}) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},
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
		"add":             func(a, b int) int { return a + b },
		"urlEscape":       url.PathEscape,
		"statusChipClass": statusChipClass,
		"statusLabel":     statusLabel,
		"activityVerb":    activityVerb,
		"activityPhrase":  activityPhrase,
		"firstInitial":    firstInitial,
	}
	for k, v := range extraFuncs {
		fm[k] = v
	}
	return fm
}

// statusChipClass maps a raw status string to the CSS modifier used by the
// status-chip component. Unknown values fall back to "archived" so a stray
// status never breaks the layout.
func statusChipClass(status string) string {
	switch status {
	case "active":
		return "active"
	case "in_progress":
		return "progress"
	case "archived":
		return "archived"
	default:
		return "archived"
	}
}

// statusLabel returns the human-readable label displayed inside a status chip.
func statusLabel(status string) string {
	switch status {
	case "active":
		return "Active"
	case "in_progress":
		return "In Progress"
	case "archived":
		return "Archived"
	default:
		// Capitalize the first letter so templates never show an empty chip.
		if status == "" {
			return "Unknown"
		}
		return strings.ToUpper(status[:1]) + status[1:]
	}
}

// firstInitial returns the uppercase first rune of s, or "" if s is empty.
// Rune-aware so multi-byte leading characters (e.g. non-ASCII names) are not
// truncated mid-codepoint, unlike `{{slice .Name 0 1}}` which operates on
// bytes.
func firstInitial(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return ""
}

// activityVerb expands the stored verb into a short phrase for the feed.
// Keeping this in the template layer means the canonical vocabulary in
// activity.Verb stays terse while the feed renders as prose.
func activityVerb(verb string) string {
	switch verb {
	case "created":
		return "created"
	case "updated":
		return "edited"
	case "deleted":
		return "deleted"
	case "booked":
		return "booked"
	case "uploaded":
		return "uploaded"
	case "published":
		return "published"
	case "added_step":
		return "added step to"
	case "edited_step":
		return "edited step in"
	case "granted":
		return "granted access to"
	case "revoked":
		return "revoked access to"
	default:
		return verb
	}
}

// activityPhrase renders one full action phrase after the actor name. It keeps
// the feed specific enough to distinguish sample edits from child actions.
func activityPhrase(verb, targetType, targetLabel string) string {
	label := strings.TrimSpace(targetLabel)
	withLabel := func(prefix string) string {
		if label == "" {
			return prefix
		}
		return prefix + " " + label
	}
	withColonLabel := func(prefix string) string {
		if label == "" {
			return prefix
		}
		return prefix + ": " + label
	}

	switch targetType {
	case "sample":
		switch verb {
		case "created":
			return withLabel("created sample")
		case "updated":
			if label == "" {
				return "edited sample details"
			}
			if isCompleteActivityPhrase(label) {
				return label
			}
			return "edited sample " + label
		case "deleted":
			return withLabel("deleted sample")
		}
	case "attachment":
		switch verb {
		case "uploaded":
			return withLabel("added attachment")
		case "deleted":
			return withLabel("removed attachment")
		}
	case "fabrication_step":
		switch verb {
		case "added_step":
			return withColonLabel("added fabrication step")
		case "edited_step":
			return withColonLabel("edited fabrication step")
		case "deleted":
			return withColonLabel("deleted fabrication step")
		}
	case "article":
		switch verb {
		case "created", "published":
			return withLabel("published article")
		case "updated":
			return withLabel("edited article")
		case "deleted":
			return withLabel("deleted article")
		}
	case "article_attachment":
		switch verb {
		case "uploaded":
			return withLabel("added article attachment")
		case "deleted":
			return withLabel("removed article attachment")
		}
	}

	fallback := strings.TrimSpace(activityVerb(verb) + " " + label)
	if fallback != "" {
		return fallback
	}
	return activityVerb(verb)
}

func isCompleteActivityPhrase(label string) bool {
	switch {
	case strings.HasPrefix(label, "edited "):
		return true
	case strings.HasPrefix(label, "renamed "):
		return true
	case strings.HasPrefix(label, "changed "):
		return true
	case strings.HasPrefix(label, "moved "):
		return true
	case strings.HasPrefix(label, "updated "):
		return true
	default:
		return false
	}
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
			return t.Format("15:04")
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
		"firstInitial": firstInitial,
	}
}
