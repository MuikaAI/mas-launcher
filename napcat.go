package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// napcatCmd implements `mas-launcher napcat [name]`.
func (m *Manager) napcatCmd(args []string) error {
	name, _ := instanceName(args)
	i, repo, err := m.instance(name)
	if err != nil {
		return err
	}
	// Print Onebot V11 connection info regardless of platform
	napcatConfigInfo(repo, i)
	fmt.Println()

	switch runtime.GOOS {
	case "windows":
		return napcatWindows(i.Path)
	case "linux":
		return napcatLinux()
	case "darwin":
		return napcatMacOS()
	default:
		fmt.Println("NapCat Shell mode is not supported on this platform.")
		fmt.Println("Docker deployment: see deploy/README.md")
		return nil
	}
}

// napcatConfigInfo prints the Onebot V11 connection details and IPC secret
// so any compatible protocol implementation can connect. Also called from
// startCmd after Core + Bot are up.
func napcatConfigInfo(repo string, i Instance) {
	env := parseEnv(filepath.Join(repo, ".env"))
	fmt.Println("─ Onebot V11 connection info ─")
	fmt.Printf("  WebSocket:   ws://%s:3001\n", i.Host)
	fmt.Printf("  Core:        ws://%s:%d/ws\n", i.Host, i.Port)
	if s := env["IPC_SECRET"]; s != "" {
		fmt.Printf("  IPC Secret:  %s\n", s)
	}
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
}

// napcatWindows guides the user through NapCat.Shell download, extraction
// and launch on Windows.
func napcatWindows(instancePath string) error {
	p := newPrompt()
	dir, err := p.ask("NapCat directory", filepath.Join(instancePath, "napcat"))
	if err != nil {
		return err
	}
	dir = strings.TrimSpace(dir)

	needDownload := true
	if fi, e := os.Stat(dir); e == nil && fi.IsDir() {
		needDownload = false
		if entries, _ := os.ReadDir(dir); len(entries) == 0 {
			needDownload = true
		}
	}
	if needDownload {
		dl, err := p.askBool("Download NapCat.Shell.zip to this directory", true)
		if err != nil {
			return err
		}
		if dl {
			if err := downloadNapCat(dir); err != nil {
				fmt.Printf("Download failed: %v\n", err)
				fmt.Println("Download manually from https://github.com/NapNeko/NapCatQQ/releases")
			}
		} else {
			fmt.Println("Download NapCat.Shell.zip from https://github.com/NapNeko/NapCatQQ/releases")
			fmt.Println("and extract it to:", dir)
		}
	}

	qq, err := p.ask("QQ number (optional, or leave empty to scan QR in WebUI)", "")
	if err != nil {
		return err
	}
	qq = strings.TrimSpace(qq)

	bat := filepath.Join(dir, "launcher.bat")
	if _, err := os.Stat(bat); err != nil {
		bat = filepath.Join(dir, "launcher-win10.bat")
		if _, err := os.Stat(bat); err != nil {
			fmt.Println("launcher.bat not found in", dir)
			fmt.Println("Make sure NapCat.Shell is extracted and try again.")
			return nil
		}
	}

	fmt.Printf("Starting NapCat (WebUI: http://127.0.0.1:6099/webui)...\n")
	var c *exec.Cmd
	if qq != "" {
		c = exec.Command("cmd", "/c", "start", "", bat, qq)
	} else {
		c = exec.Command("cmd", "/c", "start", "", bat)
	}
	if err := c.Run(); err != nil {
		return fmt.Errorf("failed to start NapCat: %w", err)
	}
	fmt.Println("NapCat launched in a new terminal window.")
	fmt.Println("Open http://127.0.0.1:6099/webui to scan QR and log in.")
	return nil
}

// napcatLinux prints the one-click installer command for Linux.
func napcatLinux() error {
	fmt.Println("Run the NapCat installer (requires sudo):")
	fmt.Println()
	fmt.Println("  curl -o napcat.sh https://nclatest.znin.net/NapNeko/NapCat-Installer/main/script/install.sh && sudo bash napcat.sh")
	fmt.Println()
	fmt.Println("Advanced options: --tui  --docker [y/n]  --cli [y/n]  --proxy [0-6]  --force")
	fmt.Println("Docker deployment: see deploy/README.md")
	return nil
}

// napcatMacOS tells the user to self-install.
func napcatMacOS() error {
	fmt.Println("NapCat Shell mode is not available on macOS.")
	fmt.Println("Refer to https://napneko.pages.dev or use Docker:")
	fmt.Println("  deploy/README.md")
	return nil
}

// downloadNapCat fetches the latest NapCat.Shell.zip from GitHub, extracts
// it into dir, and removes the zip afterwards.
func downloadNapCat(dir string) error {
	// Find the download URL from the latest GitHub release
	url, err := napCatShellURL()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "napcat-*.zip")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	fmt.Println("Downloading", url, "...")
	resp, err := http.Get(url)
	if err != nil {
		tmp.Close()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	fmt.Println("Extracting...")
	return extractZip(tmpName, dir)
}

// napCatShellURL queries the GitHub API for the latest NapCatQQ release and
// returns the download URL of NapCat.Shell.zip.
func napCatShellURL() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// Unauthenticated requests have a low rate limit (60/hr). A token lets
	// us handle bursts, but is not required for a one-off download.
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var rel struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("failed to parse GitHub release: %w", err)
	}
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, "NapCat.Shell.zip") {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", errors.New("NapCat.Shell.zip not found in latest release")
}

// extractZip unzips src into dst, flattening a single top-level directory
// (common in GitHub zip archives) so files land directly under dst.
func extractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// Detect a single common prefix to strip (e.g. "NapCat.Shell/").
	var prefix string
	for i, f := range r.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if i == 0 {
			prefix = parts[0] + "/"
		} else if parts[0]+"/" != prefix {
			prefix = "" // no common prefix — extract as-is
			break
		}
	}

	for _, f := range r.File {
		name := f.Name
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
		}
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
