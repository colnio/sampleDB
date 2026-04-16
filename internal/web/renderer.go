// Package web provides HTTP helpers and template rendering utilities.
// The Renderer type will be fully implemented in a later task; this stub
// satisfies the dependency for the app.App struct introduced in Task 8.
package web

// Renderer caches parsed templates and executes them by name.
// Full implementation (Task 12) will parse all templates once at startup and
// provide concurrent-safe rendering via Render(w, name, data).
type Renderer struct{}
