package wiki

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

// TOCEntry is a single item in the wiki article's "On this page" sidebar.
// Anchor is the heading's id attribute (without leading `#`), Title is the
// plain-text heading, and Level is 2 or 3 for h2/h3 respectively.
type TOCEntry struct {
	Anchor string
	Title  string
	Level  int
}

var (
	tocHeadingRE = regexp.MustCompile(`(?is)<h([23])\b([^>]*)>(.*?)</h[23]>`)
	tocIDAttrRE  = regexp.MustCompile(`(?i)\bid\s*=\s*"([^"]*)"`)
	tocTagRE     = regexp.MustCompile(`<[^>]+>`)
	tocSpaceRE   = regexp.MustCompile(`\s+`)
)

// BuildTOC scans the rendered article HTML for h2/h3 headings and returns a
// flat list of TOC entries in document order. Headings missing an `id`
// attribute are skipped so anchor links always work.
func BuildTOC(rendered template.HTML) []TOCEntry {
	if rendered == "" {
		return nil
	}
	matches := tocHeadingRE.FindAllStringSubmatch(string(rendered), -1)
	if len(matches) == 0 {
		return nil
	}
	entries := make([]TOCEntry, 0, len(matches))
	for _, m := range matches {
		idMatch := tocIDAttrRE.FindStringSubmatch(m[2])
		if len(idMatch) < 2 {
			continue
		}
		anchor := strings.TrimSpace(idMatch[1])
		if anchor == "" {
			continue
		}
		title := tocTagRE.ReplaceAllString(m[3], "")
		title = html.UnescapeString(title)
		title = tocSpaceRE.ReplaceAllString(title, " ")
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}
		level := 2
		if m[1] == "3" {
			level = 3
		}
		entries = append(entries, TOCEntry{Anchor: anchor, Title: title, Level: level})
	}
	return entries
}
