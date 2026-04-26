package web

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveStoragePath resolves a stored attachment path while allowing files
// under either the application base directory or the configured uploads
// directory. It returns an empty string when an absolute path would escape both
// roots.
func ResolveStoragePath(path, baseDir, uploadsDir string) string {
	if path == "" {
		return ""
	}

	roots := allowedStorageRoots(baseDir, uploadsDir)
	if filepath.IsAbs(path) {
		clean := filepath.Clean(path)
		if len(roots) == 0 || isWithinAnyRoot(clean, roots) {
			return clean
		}
		return ""
	}

	clean := filepath.Clean(path)
	if baseDir == "" {
		return clean
	}

	joined := filepath.Join(baseDir, clean)
	if isWithinRoot(joined, filepath.Clean(baseDir)) {
		return joined
	}
	return ""
}

func allowedStorageRoots(baseDir, uploadsDir string) []string {
	var roots []string
	for _, root := range []string{baseDir, uploadsDir} {
		if root == "" {
			continue
		}
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		roots = append(roots, filepath.Clean(root))
	}
	return roots
}

func isWithinAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if isWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func isWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
