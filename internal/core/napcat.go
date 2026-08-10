package core

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

	"github.com/MuikaAI/mas-launcher/internal/i18n"
)

// napcatCmd implements `mas-launcher napcat [name] [--show-napcat] [--admin] [--stop]`.
func (m *Manager) napcatCmd(args []string) error {
	name, rest := instanceName(args)
	fs := flag.NewFlagSet("napcat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	showWindow := fs.Bool("show-napcat", false, "show NapCat terminal window")
	admin := fs.Bool("admin", false, "run with administrator privileges (UAC, window visible)")
	stop := fs.Bool("stop", false, "stop the running NapCat process")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	i, repo, err := m.instance(name)
	if err != nil {
		return err
	}

	if *stop {
		return stopNapCat(i.NapCatDir)
	}

	napcatConfigInfo(repo, i)
	fmt.Println()

	switch runtime.GOOS {
	case "windows":
		// Kill any previously-running NapCat before launching a new one.
		if i.NapCatDir != "" {
			_ = stopNapCat(i.NapCatDir)
		}
		dir, qq, err := napcatWindows(i.Path, i.NapCatDir, i.NapCatQQ, *showWindow, *admin)
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
		fmt.Println(i18n.T("NapCat Shell mode is not supported on this platform."))
		fmt.Println(i18n.T("Docker deployment: see deploy/README.md"))
		return nil
	}
}

// napcatConfigInfo prints the Onebot V11 listening address (reverse WebSocket
// — the protocol implementation connects as a client) and Core connection
// details. Called from startCmd after Core + Bot are up, and from napcatCmd.
func napcatConfigInfo(repo string, i Instance) {
	env := parseEnv(filepath.Join(repo, ".env"))
	fmt.Println(i18n.T("─ Onebot V11 (reverse WebSocket) ─"))
	fmt.Printf(i18n.T("  Listen:   ws://%s:8080/onebot/v11/\n"), i.Host)
	fmt.Println(i18n.T("  The protocol implementation (e.g. NapCat) connects here as a WebSocket client."))
	fmt.Println()
	fmt.Println(i18n.T("─ Core ─"))
	fmt.Printf(i18n.T("  WebSocket:   ws://%s:%d/ws\n"), i.Host, i.Port)
	if s := env["IPC_SECRET"]; s != "" {
		fmt.Printf(i18n.T("  IPC Secret:  %s\n"), s)
	}
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
}

// napcatWindows guides the user through NapCat.Shell setup, remembers the
// directory and QQ number across runs, auto-configures the Onebot v11
// reverse-WebSocket client, and launches NapCat.
//
// By default NapCat runs without admin privileges (launcher-user.bat) so
// there is no UAC popup and the window can be hidden with /MIN. Pass admin
// to use the privileged launcher.bat (UAC elevation, window always visible).
func napcatWindows(instancePath, defaultDir, defaultQQ string, showWindow, admin bool) (dir, qq string, _ error) {
	if defaultDir == "" {
		defaultDir = filepath.Join(instancePath, "napcat")
	}
	p := newPrompt()
	dir, err := p.ask(i18n.T("NapCat directory"), defaultDir)
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
		dl, err := p.askBool(i18n.T("Download NapCat.Shell.zip to this directory"), true)
		if err != nil {
			return "", "", err
		}
		if dl {
			if err := downloadNapCat(dir); err != nil {
				fmt.Printf(i18n.T("Download failed: %v\n"), err)
				fmt.Println(i18n.T("Download manually from https://github.com/NapNeko/NapCatQQ/releases"))
			}
		} else {
			fmt.Println(i18n.T("Download NapCat.Shell.zip from https://github.com/NapNeko/NapCatQQ/releases"))
			fmt.Printf(i18n.T("and extract it to: %s\n"), dir)
		}
	}

	qq, err = p.ask(i18n.T("QQ number (optional, or leave empty to scan QR in WebUI)"), defaultQQ)
	if err != nil {
		return "", "", err
	}
	qq = strings.TrimSpace(qq)

	// Pick the right launcher script. Default to the non-admin variant
	// so we can run in the background without a UAC popup.
	var bat string
	if admin {
		bat = filepath.Join(dir, "launcher.bat")
		if _, err := os.Stat(bat); err != nil {
			bat = filepath.Join(dir, "launcher-win10.bat")
		}
	} else {
		bat = filepath.Join(dir, "launcher-user.bat")
		if _, err := os.Stat(bat); err != nil {
			bat = filepath.Join(dir, "launcher-win10-user.bat")
		}
	}
	if _, err := os.Stat(bat); err != nil {
		fmt.Printf(i18n.T("launcher script not found in: %s\n"), dir)
		fmt.Println(i18n.T("Make sure NapCat.Shell is extracted and try again."))
		return dir, qq, nil
	}

	// Auto-configure Onebot v11 reverse WebSocket client so NapCat
	// connects back to the bot without manual configuration.
	if qq != "" {
		if err := configureOnebot(dir, qq); err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("warning: could not auto-configure Onebot: %v\n"), err)
		}
	}

	// Build the launch command. Hidden mode (default) avoids the
	// terminal window entirely by firing the bat via cmd /c without
	// 'start', combined with HideWindow (CREATE_NO_WINDOW).
	var c *exec.Cmd
	if !showWindow && !admin {
		args := []string{"/c", bat}
		if qq != "" {
			args = append(args, qq)
		}
		c = exec.Command("cmd", args...)
		c.Dir = dir
		hideWindow(c)
		if err := c.Start(); err != nil {
			return dir, qq, fmt.Errorf(i18n.T("failed to start NapCat: %w"), err)
		}
		fmt.Printf(i18n.T("NapCat started in background (PID %d).\n"), c.Process.Pid)
	} else {
		startArgs := []string{"/c", "start", "", bat}
		if qq != "" {
			startArgs = append(startArgs, qq)
		}
		c = exec.Command("cmd", startArgs...)
		c.Dir = dir
		if err := c.Run(); err != nil {
			return dir, qq, fmt.Errorf(i18n.T("failed to start NapCat: %w"), err)
		}
	}

	// Wait briefly for NapCat to initialise, then print the WebUI address
	// with the token from webui.json.
	time.Sleep(2 * time.Second)
	if tokenURL := readWebUIToken(dir); tokenURL != "" {
		fmt.Printf(i18n.T("NapCat WebUI: %s\n"), tokenURL)
	} else {
		fmt.Println(i18n.T("NapCat WebUI: http://127.0.0.1:6099/webui (token will appear after first launch)"))
	}
	fmt.Println(i18n.T("Open the WebUI to scan QR and log in."))
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
	fmt.Println(i18n.T(`Run the NapCat installer (requires sudo):

  curl -o napcat.sh https://nclatest.znin.net/NapNeko/NapCat-Installer/main/script/install.sh && sudo bash napcat.sh

Advanced options: --tui  --docker [y/n]  --cli [y/n]  --proxy [0-6]  --force
Docker deployment: see deploy/README.md`))
	return nil
}

// napcatMacOS tells the user to self-install.
func napcatMacOS() error {
	fmt.Println(i18n.T(`NapCat Shell mode is not available on macOS.
Refer to https://napneko.pages.dev or use Docker:
  deploy/README.md`))
	return nil
}

// stopNapCat finds and kills the NapCat process whose executable is under
// the given napcat directory. Returns nil if no such process exists.
func stopNapCat(dir string) error {
	if dir == "" {
		return errors.New(i18n.T("no napcat directory configured; run mas-launcher napcat first"))
	}
	pid, err := findNapCatPID(dir)
	if err != nil {
		return err
	}
	if err := killPID(pid); err != nil {
		return err
	}
	fmt.Printf(i18n.T("NapCat stopped (PID %d).\n"), pid)
	return nil
}

// findNapCatPID locates a NapCatWinBootMain.exe process whose executable
// path is inside the given napcat directory.
func findNapCatPID(dir string) (int, error) {
	// Step 1 — find candidate PIDs with tasklist (reliable, no name
	// truncation issue like Get-Process has with 17-char names).
	out, err := exec.Command("tasklist", "/fi", "IMAGENAME eq NapCatWinBootMain.exe", "/fo", "csv", "/nh").Output()
	if err != nil {
		return 0, fmt.Errorf(i18n.T("cannot enumerate NapCat processes: %w"), err)
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "NapCat") {
			continue
		}
		// CSV: "NapCatWinBootMain.exe","67572","Console","2","4,616 K"
		cols := strings.Split(line, `","`)
		if len(cols) < 2 {
			continue
		}
		pidStr := strings.Trim(cols[1], `"`)
		if pid, err := parseInt(pidStr); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}

	// Step 2 — verify which PID belongs to our napcat directory.
	abs, _ := filepath.Abs(dir)
	for _, pid := range pids {
		ps := fmt.Sprintf(`(Get-Process -Id %d -ErrorAction SilentlyContinue).Path`, pid)
		pathOut, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
		if err != nil {
			continue
		}
		exe := strings.TrimSpace(string(pathOut))
		if strings.HasPrefix(exe, abs) || strings.HasPrefix(exe, dir) {
			return pid, nil
		}
	}

	// Step 3 — if path verification failed but we have exactly one PID,
	// trust it (the machine likely only runs one NapCat instance).
	if len(pids) == 1 {
		return pids[0], nil
	}
	if len(pids) > 1 {
		return 0, fmt.Errorf(i18n.T("multiple NapCat processes found (%v); stop manually"), pids)
	}
	return 0, errors.New(i18n.T("napcat process not found (is it running?)"))
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf(i18n.T("not a number: %q"), s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
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

	fmt.Printf(i18n.T("Downloading %s...\n"), url)
	resp, err := http.Get(url)
	if err != nil {
		tmp.Close()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf(i18n.T("download failed: %s"), resp.Status)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	fmt.Println(i18n.T("Extracting..."))
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
		return "", fmt.Errorf(i18n.T("GitHub API returned %s"), resp.Status)
	}
	var rel struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf(i18n.T("failed to parse GitHub release: %w"), err)
	}
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, "NapCat.Shell.zip") {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", errors.New(i18n.T("NapCat.Shell.zip not found in latest release"))
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
