package main

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_Basic(t *testing.T) {
	out := string(renderMarkdown("**bold** text"))
	if !strings.Contains(out, "<strong>bold</strong>") {
		t.Errorf("expected <strong>bold</strong> in output, got: %s", out)
	}
}

func TestRenderMarkdown_Empty(t *testing.T) {
	if renderMarkdown("") != "" {
		t.Error("expected empty output for empty input")
	}
	if renderMarkdown("   ") != "" {
		t.Error("expected empty output for whitespace-only input")
	}
}

// SkipHTML: raw HTML tags must be stripped, not rendered.
func TestRenderMarkdown_SkipHTML(t *testing.T) {
	out := string(renderMarkdown("hello <script>alert(1)</script> world"))
	if strings.Contains(out, "<script>") {
		t.Errorf("raw <script> tag should be stripped, got: %s", out)
	}
	out2 := string(renderMarkdown(`<img src="x" onerror="alert(1)">`))
	if strings.Contains(out2, "<img") {
		t.Errorf("raw <img> tag should be stripped, got: %s", out2)
	}
}

// Safelink: javascript: and other unsafe URL schemes must not appear as href.
func TestRenderMarkdown_UnsafeLinks(t *testing.T) {
	cases := []string{
		"[click](javascript:alert(document.domain))",
		"[click](javascript:void(0))",
		"[click](vbscript:msgbox(1))",
	}
	for _, md := range cases {
		out := string(renderMarkdown(md))
		if strings.Contains(out, "javascript:") || strings.Contains(out, "vbscript:") {
			t.Errorf("unsafe URL scheme should be blocked in output, got: %s (input: %s)", out, md)
		}
	}
}

// Safe links must still work normally.
func TestRenderMarkdown_SafeLinks(t *testing.T) {
	cases := []struct {
		md   string
		want string
	}{
		{"[example](https://example.com)", "https://example.com"},
		{"[local](/samples/1)", "/samples/1"},
		{"[mail](mailto:user@example.com)", "mailto:user@example.com"},
	}
	for _, tc := range cases {
		out := string(renderMarkdown(tc.md))
		if !strings.Contains(out, tc.want) {
			t.Errorf("safe link %q should be preserved, got: %s", tc.want, out)
		}
	}
}
