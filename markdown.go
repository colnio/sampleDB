package main

import (
	"html/template"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

func renderMarkdown(md string) template.HTML {
	if strings.TrimSpace(md) == "" {
		return ""
	}

	extensions := parser.CommonExtensions |
		parser.AutoHeadingIDs |
		parser.NoEmptyLineBeforeBlock |
		parser.Tables |
		parser.FencedCode |
		parser.Autolink |
		parser.Strikethrough |
		parser.SpaceHeadings |
		parser.HeadingIDs |
		parser.BackslashLineBreak |
		parser.DefinitionLists |
		parser.Footnotes

	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(md))

	opts := html.RendererOptions{
		Flags: html.CommonFlags |
			html.HrefTargetBlank |
			html.LazyLoadImages |
			html.TOC |
			html.UseXHTML |
			html.FootnoteReturnLinks,
	}
	renderer := html.NewRenderer(opts)

	return template.HTML(markdown.Render(doc, renderer))
}
