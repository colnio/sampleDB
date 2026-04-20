package wiki_test

import (
	"html/template"
	"strings"
	"testing"

	"sampleDB/internal/web"
	"sampleDB/internal/wiki"
)

func TestBuildTOC_EmptyHTML(t *testing.T) {
	if got := wiki.BuildTOC(""); got != nil {
		t.Errorf("BuildTOC(\"\") = %v, want nil", got)
	}
}

func TestBuildTOC_NoHeadings(t *testing.T) {
	html := template.HTML(`<p>Just a paragraph with <strong>no</strong> headings.</p>`)
	if got := wiki.BuildTOC(html); got != nil {
		t.Errorf("BuildTOC(no headings) = %v, want nil", got)
	}
}

func TestBuildTOC_H2AndH3Order(t *testing.T) {
	html := template.HTML(`
<h2 id="overview">Overview</h2>
<p>body</p>
<h3 id="details">Details</h3>
<h2 id="appendix">Appendix</h2>`)

	got := wiki.BuildTOC(html)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %#v", len(got), got)
	}
	wantTitles := []string{"Overview", "Details", "Appendix"}
	wantAnchors := []string{"overview", "details", "appendix"}
	wantLevels := []int{2, 3, 2}
	for i, e := range got {
		if e.Title != wantTitles[i] {
			t.Errorf("entry %d title = %q, want %q", i, e.Title, wantTitles[i])
		}
		if e.Anchor != wantAnchors[i] {
			t.Errorf("entry %d anchor = %q, want %q", i, e.Anchor, wantAnchors[i])
		}
		if e.Level != wantLevels[i] {
			t.Errorf("entry %d level = %d, want %d", i, e.Level, wantLevels[i])
		}
	}
}

func TestBuildTOC_SkipsHeadingsWithoutID(t *testing.T) {
	html := template.HTML(`<h2>No ID</h2><h2 id="ok">Has ID</h2>`)
	got := wiki.BuildTOC(html)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Anchor != "ok" {
		t.Errorf("expected anchor 'ok', got %q", got[0].Anchor)
	}
}

func TestBuildTOC_StripsNestedTagsFromTitle(t *testing.T) {
	html := template.HTML(`<h2 id="x"><em>Fancy</em> <strong>title</strong></h2>`)
	got := wiki.BuildTOC(html)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Title != "Fancy title" {
		t.Errorf("title = %q, want %q", got[0].Title, "Fancy title")
	}
}

func TestBuildTOC_DuplicateTitles_DistinctAnchors(t *testing.T) {
	// gomarkdown's AutoHeadingIDs suffixes duplicate IDs. Our extractor trusts
	// the IDs it finds and surfaces them as-is so anchor links remain unique.
	html := template.HTML(`<h2 id="setup">Setup</h2><h2 id="setup-1">Setup</h2>`)
	got := wiki.BuildTOC(html)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Anchor == got[1].Anchor {
		t.Errorf("expected distinct anchors, got both %q", got[0].Anchor)
	}
	if got[0].Title != got[1].Title || got[0].Title != "Setup" {
		t.Errorf("expected both titles %q, got %q / %q", "Setup", got[0].Title, got[1].Title)
	}
}

// TestBuildTOC_MatchesRenderedMarkdown ensures our extractor's anchors line up
// with the IDs that web.RenderMarkdown actually emits — if they drift, TOC
// links would 404-to-anchor. This is the contract the Wiki view relies on.
func TestBuildTOC_MatchesRenderedMarkdown(t *testing.T) {
	md := "## Overview\n\nLead.\n\n### Details\n\nMore.\n\n## Wrap Up\n"
	rendered := web.RenderMarkdown(md)
	toc := wiki.BuildTOC(rendered)

	if len(toc) == 0 {
		t.Fatalf("expected TOC entries from rendered markdown, got 0\nrendered:\n%s", rendered)
	}
	for _, e := range toc {
		if !strings.Contains(string(rendered), `id="`+e.Anchor+`"`) {
			t.Errorf("TOC anchor %q has no matching id in rendered HTML:\n%s", e.Anchor, rendered)
		}
	}
	wantTitles := []string{"Overview", "Details", "Wrap Up"}
	if len(toc) != len(wantTitles) {
		t.Fatalf("expected %d entries, got %d: %#v", len(wantTitles), len(toc), toc)
	}
	for i, e := range toc {
		if e.Title != wantTitles[i] {
			t.Errorf("entry %d title = %q, want %q", i, e.Title, wantTitles[i])
		}
	}
}
