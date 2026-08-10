package repository

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MuikaAI/mas-launcher/internal/i18n"
)

func FetchRepository(repo, ref, dest string) error {
	if _, err := os.Stat(filepath.Join(dest, "pyproject.toml")); err == nil {
		return nil
	}
	// Prefer a git clone so instances stay updatable (mas-launcher update
	// requires a checkout). Fall back to a GitHub zip only without git.
	if _, err := exec.LookPath("git"); err == nil {
		c := exec.Command("git", "clone", "--depth", "1", "--branch", ref, repo, dest)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err == nil {
			return nil
		}
	}
	if !strings.HasPrefix(repo, "https://github.com/") {
		return errors.New(i18n.T("cannot fetch repository; use a GitHub URL or install Git"))
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(repo, "https://github.com/"), ".git"), "/")
	if len(parts) < 2 {
		return errors.New(i18n.T("cannot fetch repository; use a GitHub URL or install Git"))
	}
	url := fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", parts[0], parts[1], ref)
	return downloadZip(url, dest)
}
func downloadZip(url, dest string) error {
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf(i18n.T("repository download failed: %s"), r.Status)
	}
	tmp, err := os.CreateTemp("", "mas-launcher-*.zip")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = io.Copy(tmp, r.Body); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	z, err := zip.OpenReader(name)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) == 0 {
		return errors.New(i18n.T("empty repository archive"))
	}
	prefix := strings.Split(filepath.ToSlash(z.File[0].Name), "/")[0] + "/"
	for _, f := range z.File {
		n := filepath.ToSlash(f.Name)
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		rel := strings.TrimPrefix(n, prefix)
		if rel == "" {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if !within(dest, target) {
			return errors.New(i18n.T("archive contains an invalid path"))
		}
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		in.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
func within(root, target string) bool {
	r, e1 := filepath.Abs(root)
	t, e2 := filepath.Abs(target)
	if e1 != nil || e2 != nil {
		return false
	}
	rel, e := filepath.Rel(r, t)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
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
