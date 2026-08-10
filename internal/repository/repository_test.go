package repository

import (
	"path/filepath"
	"testing"
)

func TestWithin(t *testing.T) {
	r := t.TempDir()
	if !within(r, filepath.Join(r, "nested", "file")) {
		t.Fatal("nested path rejected")
	}
	if within(r, filepath.Join(r, "..", "outside")) {
		t.Fatal("traversal accepted")
	}
}
