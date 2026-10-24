package repository

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MuikaAI/mas-launcher/internal/i18n"
)

// FetchRepository clones the project into dest and fetches its release tags.
//
// A git checkout is required: release tags let pdm-backend derive the version
// that Self-Change compares across updates, and update needs a checkout. A zip
// copy has neither, so it is refused instead of pinning the instance to the
// build fallback version.
func FetchRepository(repo, ref, dest string) error {
	if _, err := os.Stat(filepath.Join(dest, "pyproject.toml")); err == nil {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New(i18n.T("Git is required to fetch the project; install Git and retry"))
	}
	c := exec.Command("git", "clone", "--depth", "1", "--branch", ref, repo, dest)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return errors.New(i18n.T("cannot fetch repository; use a GitHub URL or install Git"))
	}
	// A shallow clone only carries tags that point at the fetched tip, so fetch
	// every tag afterwards.
	tags := exec.Command("git", "-C", dest, "fetch", "--depth", "1", "--tags", "origin")
	tags.Stdout, tags.Stderr = os.Stdout, os.Stderr
	if err := tags.Run(); err != nil {
		return fmt.Errorf(i18n.T("failed to fetch release tags: %w"), err)
	}
	return nil
}

func GitOutput(dir string, args ...string) string {
	c := exec.Command("git", args...)
	c.Dir = dir
	b, e := c.Output()
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
