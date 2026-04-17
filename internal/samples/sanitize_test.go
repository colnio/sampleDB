package samples

import (
	"regexp"
	"strings"
	"testing"
)

func TestSanitizeFilename_Normal(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello world.txt", "hello-world.txt"},
		{"report (final).PDF", "report-final.pdf"},
		{"my-file_name.jpg", "my-file_name.jpg"},
		{"file", "file"},
		{"file.tar.gz", "file.tar.gz"},
	}
	for _, tc := range cases {
		got := sanitizeFilename(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// Path traversal attempts must not produce a path with directory separators.
func TestSanitizeFilename_PathTraversal(t *testing.T) {
	cases := []string{
		"../../etc/passwd",
		"../secret.txt",
		"/absolute/path/file.txt",
		"subdir/file.txt",
	}
	for _, input := range cases {
		got := sanitizeFilename(input)
		if strings.Contains(got, "/") || strings.Contains(got, "\\") {
			t.Errorf("sanitizeFilename(%q) = %q; must not contain path separators", input, got)
		}
	}
}

func TestSanitizeFilename_EmptyStem(t *testing.T) {
	cases := []string{"", "   ", "...", "!!!.jpg"}
	for _, input := range cases {
		got := sanitizeFilename(input)
		if got == "" {
			t.Errorf("sanitizeFilename(%q) returned empty string; expected fallback name", input)
		}
	}
}

func TestSanitizeFilename_NullBytes(t *testing.T) {
	got := sanitizeFilename("file\x00name.txt")
	if strings.Contains(got, "\x00") {
		t.Errorf("sanitizeFilename produced null byte in output: %q", got)
	}
}

func TestGenerateUniqueFilename_Format(t *testing.T) {
	name, err := generateUniqueFilename("test.txt")
	if err != nil {
		t.Fatalf("generateUniqueFilename: %v", err)
	}
	// Expected format: <16 hex chars>_test.txt
	if !regexp.MustCompile(`^[0-9a-f]{16}_`).MatchString(name) {
		t.Errorf("generateUniqueFilename(%q) = %q; expected <16-hex>_<sanitized>", "test.txt", name)
	}
}

func TestGenerateUniqueFilename_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		name, err := generateUniqueFilename("file.txt")
		if err != nil {
			t.Fatalf("generateUniqueFilename: %v", err)
		}
		if seen[name] {
			t.Fatalf("generateUniqueFilename produced duplicate: %q", name)
		}
		seen[name] = true
	}
}
