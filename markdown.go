package main

import (
	"html/template"

	"sampleDB/internal/web"
)

// renderMarkdown is a package-level adapter so the web.Renderer template
// function and any legacy call-sites in package main can call into the
// shared implementation in internal/web.
func renderMarkdown(md string) template.HTML {
	return web.RenderMarkdown(md)
}
