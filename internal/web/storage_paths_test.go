package web

import (
	"path/filepath"
	"testing"
)

func TestResolveStoragePath_AllowsConfiguredUploadsDir(t *testing.T) {
	baseDir := filepath.Join(string(filepath.Separator), "home", "ubuntu", "sampleDB")
	uploadsDir := filepath.Join(string(filepath.Separator), "home", "ubuntu", "uploads")
	stored := filepath.Join(uploadsDir, "legacy.png")

	got := ResolveStoragePath(stored, baseDir, uploadsDir)
	if got != stored {
		t.Fatalf("ResolveStoragePath() = %q, want %q", got, stored)
	}
}

func TestResolveStoragePath_RejectsAbsolutePathOutsideAllowedRoots(t *testing.T) {
	baseDir := filepath.Join(string(filepath.Separator), "home", "ubuntu", "sampleDB")
	uploadsDir := filepath.Join(string(filepath.Separator), "home", "ubuntu", "uploads")
	stored := filepath.Join(string(filepath.Separator), "etc", "passwd")

	if got := ResolveStoragePath(stored, baseDir, uploadsDir); got != "" {
		t.Fatalf("ResolveStoragePath() = %q, want empty string", got)
	}
}

func TestResolveStoragePath_ResolvesRelativePathUnderBaseDir(t *testing.T) {
	baseDir := filepath.Join(string(filepath.Separator), "srv", "sampleDB")
	got := ResolveStoragePath("uploads/file.txt", baseDir, "")
	want := filepath.Join(baseDir, "uploads", "file.txt")
	if got != want {
		t.Fatalf("ResolveStoragePath() = %q, want %q", got, want)
	}
}
