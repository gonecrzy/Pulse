package installtests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoFile(parts ...string) string {
	segments := append([]string{filepath.Join("..", "..")}, parts...)
	return filepath.Join(segments...)
}

func assertFileContainsAll(t *testing.T, path string, required ...string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, needle := range required {
		if !strings.Contains(string(content), needle) {
			t.Fatalf("%s missing required substring: %s", path, needle)
		}
	}
}
