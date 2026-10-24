package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFetchRepositoryRequiresGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dest := filepath.Join(t.TempDir(), "repo")
	if err := FetchRepository("https://example.com/repo.git", "main", dest); err == nil {
		t.Fatal("expected an error without git")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("created an instance directory without git")
	}
}
