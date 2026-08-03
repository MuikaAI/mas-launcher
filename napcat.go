package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"flag"
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

// napcatCmd implements `mas-launcher napcat [name] [--show-napcat]`.
func (m *Manager) napcatCmd(args []string) error {
	name, rest := instanceName(args)
	fs := flag.NewFlagSet("napcat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	showWindow := fs.Bool("show-napcat", false, "show NapCat terminal window")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	i, repo, err := m.instance(name)
	if err != nil {
		return err
	}
	napcatConfigInfo(repo, i)
	fmt.Println()

	switch runtime.GOOS {
	case "windows":
		dir, qq, err := napcatWindows(i.Path, i.NapCatDir, i.NapCatQQ, *showWindow)
		if err != nil {
			return err
		}
		if dir != "" {
			inst := m.Config.Instances[name]
			inst.NapCatDir = dir
			inst.NapCatQQ = qq
			m.Config.Instances[name] = inst
			_ = m.save()
		}
		return nil
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

// napcatConfigInfo prints the Onebot V11 listening address (reverse WebSocket
// — the protocol implementation connects as a client) and Core connection
// details. Called from startCmd after Core + Bot are up, and from napcatCmd.
func napcatConfigInfo(repo string, i Instance) {
	env := parseEnv(filepath.Join(repo, ".env"))
	fmt.Println("─ Onebot V11 (reverse WebSocket) ─")
	fmt.Printf("  Listen:   ws://%s:8080/onebot/v11/\n", i.Host)
	fmt.Println("  The protocol implementation (e.g. NapCat) connects here as a WebSocket client.")
	fmt.Println()
	fmt.Println("─ Core ─")
	fmt.Printf("  WebSocket:   ws://%s:%d/ws\n", i.Host, i.Port)
	if s := env["IPC_SECRET"]; s != "" {
		fmt.Printf("  IPC Secret:  %s\n", s)
	}
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
}

// napcatWindows guides the user through NapCat.Shell setup, remembers the
// directory and QQ number across runs, auto-configures the Onebot v11
// reverse-WebSocket client, and launches NapCat minimized.
func napcatWindows(instancePath, defaultDir, defaultQQ string, showWindow bool) (dir, qq string, _ error) {
	if defaultDir == "" {
		defaultDir = filepath.Join(instancePath, "napcat")
	}
	p := newPrompt()
	dir, err := p.ask("NapCat directory", defaultDir)
	if err != nil {
		return "", "", err
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
			return "", "", err
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

	qq, err = p.ask("QQ number (optional, or leave empty to scan QR in WebUI)", defaultQQ)
	if err != nil {
		return "", "", err
	}
	qq = strings.TrimSpace(qq)

	// Pick the right launcher script
	bat := filepath.Join(dir, "launcher.bat")
	if _, err := os.Stat(bat); err != nil {
		bat = filepath.Join(dir, "launcher-win10.bat")
		if _, err := os.Stat(bat); err != nil {
			fmt.Println("launcher.bat not found in", dir)
			fmt.Println("Make sure NapCat.Shell is extracted and try again.")
			return dir, qq, nil
		}
	}

	// Auto-configure Onebot v11 reverse WebSocket client so NapCat
	// connects back to the bot without manual configuration.
	if qq != "" {
		if err := configureOnebot(dir, qq); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not auto-configure Onebot: %v\n", err)
		}
	}

	// Build the start command. Use /MIN to hide the terminal window unless
	// the user explicitly asked to see it (--show-napcat).
	startArgs := []string{"/c", "start", ""}
	if !showWindow {
		startArgs = append(startArgs, "/MIN")
	}
	startArgs = append(startArgs, bat)
	if qq != "" {
		startArgs = append(startArgs, qq)
	}
	c := exec.Command("cmd", startArgs...)
	c.Dir = dir
	if err := c.Run(); err != nil {
		return dir, qq, fmt.Errorf("failed to start NapCat: %w", err)
	}

	// Wait briefly for NapCat to initialise, then print the WebUI address
	// with the token from webui.json.
	time.Sleep(2 * time.Second)
	if tokenURL := readWebUIToken(dir); tokenURL != "" {
		fmt.Println("NapCat WebUI:", tokenURL)
	} else {
		fmt.Println("NapCat WebUI: http://127.0.0.1:6099/webui (token will appear after first launch)")
	}
	fmt.Println("Open the WebUI to scan QR and log in.")
	return dir, qq, nil
}

// configureOnebot writes (or updates) onebot11_<qq>.json in napcat/config
// with a reverse-WebSocket client that points to the bot's Onebot V11
// listen address.
func configureOnebot(dir, qq string) error {
	configDir := filepath.Join(dir, "config")
	path := filepath.Join(configDir, "onebot11_"+qq+".json")

	// Read existing config if present; otherwise start fresh.
	cfg := make(map[string]any)
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &cfg)
	}

	// Build the websocketClients entry pointing at the bot.
	wsClient := map[string]any{
		"enable":            true,
		"name":              "Muika-After-Story",
		"url":               "ws://127.0.0.1:8080/onebot/v11/",
		"reportSelfMessage": false,
		"messagePostFormat": "array",
		"token":             "",
		"debug":             false,
		"heartInterval":     30000,
		"reconnectInterval": 30000,
		"verifyCertificate": true,
	}

	network, _ := cfg["network"].(map[string]any)
	if network == nil {
		network = make(map[string]any)
		cfg["network"] = network
	}
	network["websocketClients"] = []any{wsClient}

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// readWebUIToken reads webui.json in the NapCat config directory
// and returns a ready-to-use WebUI URL like
// "http://127.0.0.1:6099/webui?token=abc123".
func readWebUIToken(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "config", "webui.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil || cfg.Token == "" {
		return ""
	}
	port := cfg.Port
	if port == 0 {
		port = 6099
	}
	return fmt.Sprintf("http://127.0.0.1:%d/webui?token=%s", port, cfg.Token)
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
