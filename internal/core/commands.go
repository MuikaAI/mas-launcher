package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MuikaAI/mas-launcher/internal/i18n"
	"github.com/MuikaAI/mas-launcher/internal/models"
	"github.com/MuikaAI/mas-launcher/internal/repository"
)

func (m *Manager) initCmd(args []string) error {
	name, rest := instanceName(args)
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repo := fs.String("repo", m.Config.Repo, "repository")
	ref := fs.String("ref", m.Config.Ref, "git ref")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if _, ok := m.Config.Instances[name]; ok {
		return fmt.Errorf(i18n.T("instance %q already exists"), name)
	}
	path := filepath.Join(m.Root, "instances", name)
	repoDir := filepath.Join(path, "repo")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	fmt.Println(i18n.T("Fetching project..."))
	if err := repository.FetchRepository(*repo, *ref, repoDir); err != nil {
		return err
	}
	for _, dir := range []string{"logs", "runtime"} {
		if err := os.MkdirAll(filepath.Join(path, dir), 0o700); err != nil {
			return err
		}
	}
	i := Instance{Path: path, Host: "127.0.0.1", Port: randomPort(), Bot: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	m.Config.Repo, m.Config.Ref, m.Config.Instances[name] = *repo, *ref, i
	if err := m.save(); err != nil {
		return err
	}
	if err := m.ensureEnvironment(repoDir); err != nil {
		return err
	}
	if err := m.defaultFiles(repoDir, i); err != nil {
		return err
	}
	fmt.Printf(i18n.T("Instance %s created. Run: mas-launcher configure %s\n"), name, name)
	return nil
}

func (m *Manager) ensureEnvironment(repo string) error {
	if _, err := os.Stat(python(repo)); err == nil {
		return nil
	}
	if uv, e := exec.LookPath("uv"); e == nil {
		fmt.Println(i18n.T("Installing dependencies with uv..."))
		return command(repo, uv, "sync", "--extra", "standard", "--extra", "nonebot")
	}
	for _, candidate := range []struct {
		program string
		args    []string
	}{{"python", nil}, {"python3", nil}, {"py", []string{"-3.12"}}} {
		p, e := exec.LookPath(candidate.program)
		if e != nil {
			continue
		}
		fmt.Println(i18n.T("uv not found; using system Python..."))
		venv := append(append([]string{}, candidate.args...), "-m", "venv", ".venv")
		if e = command(repo, p, venv...); e != nil {
			continue
		}
		return command(repo, python(repo), "-m", "pip", "install", "-e", ".[standard,nonebot]")
	}
	return errors.New(i18n.T("uv or Python 3.10+ is required; install uv and retry"))
}
func (m *Manager) defaultFiles(repo string, i Instance) error {
	if _, e := os.Stat(filepath.Join(repo, ".env")); errors.Is(e, os.ErrNotExist) {
		text := fmt.Sprintf("MASTER_ID=\nSUPERUSERS=[]\nMUIKA_CORE_WS_URL=ws://%s:%d/ws\nMUIKA_DATA_DIR=./data\nCORE_WS_URL=ws://%s:%d/ws\nONEBOT_WS_URLS=[\"ws://127.0.0.1:3001\"]\nDRIVER=~fastapi+~httpx+~websockets\nIPC_SECRET=%s\n", i.Host, i.Port, i.Host, i.Port, secret())
		if e = os.WriteFile(filepath.Join(repo, ".env"), []byte(text), 0o600); e != nil {
			return e
		}
	}
	modelsPath := filepath.Join(repo, "configs", "models.yml")
	if _, e := os.Stat(modelsPath); errors.Is(e, os.ErrNotExist) {
		if e = os.MkdirAll(filepath.Dir(modelsPath), 0o700); e != nil {
			return e
		}
		return os.WriteFile(modelsPath, []byte(models.ModelSeedComment), 0o600)
	}
	return nil
}
func (m *Manager) configureCmd(args []string) error {
	name, rest := instanceName(args)
	fs := flag.NewFlagSet("configure", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	master := fs.String("master-id", "", "master ID")
	ipc := fs.String("ipc-secret", "", "IPC secret")
	core := fs.String("core-ws-url", "", "Core WebSocket URL")
	provider := fs.String("provider", "", "model provider")
	model := fs.String("model", "", "model name")
	key := fs.String("api-key", "", "API key")
	base := fs.String("base-url", "", "base URL")
	if e := fs.Parse(rest); e != nil {
		return e
	}
	_, repo, e := m.instance(name)
	if e != nil {
		return e
	}
	values := parseEnv(filepath.Join(repo, ".env"))
	in := bufio.NewReader(os.Stdin)
	ask := func(label, current string) (string, error) {
		fmt.Printf("%s [%s]: ", label, current)
		s, e := in.ReadString('\n')
		if e != nil && e != io.EOF {
			return "", e
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return current, nil
		}
		return s, nil
	}
	pick := func(label, current, provided string) (string, error) {
		if provided != "" {
			return provided, nil
		}
		return ask(label, current)
	}
	if values["MASTER_ID"], e = pick(i18n.T("Master ID"), values["MASTER_ID"], *master); e != nil {
		return e
	}
	values["SUPERUSERS"] = fmt.Sprintf("[%q]", values["MASTER_ID"])
	if values["IPC_SECRET"], e = pick(i18n.T("IPC_SECRET"), values["IPC_SECRET"], *ipc); e != nil {
		return e
	}
	if values["CORE_WS_URL"], e = pick(i18n.T("Core WebSocket URL"), values["CORE_WS_URL"], *core); e != nil {
		return e
	}
	if e = writeEnv(filepath.Join(repo, ".env"), values); e != nil {
		return e
	}
	if *provider != "" || *model != "" || *key != "" {
		if *provider == "" || *model == "" || *key == "" {
			return errors.New(i18n.T("--provider, --model and --api-key are required together"))
		}
		text := fmt.Sprintf("default:\n  provider: %s\n  model_name: %s\n  api_key: %s\n  default: true\n", *provider, *model, *key)
		if *base != "" {
			text += fmt.Sprintf("  api_host: %s\n", *base)
		}
		return os.WriteFile(filepath.Join(repo, "configs", "models.yml"), []byte(text), 0o600)
	}
	fmt.Println(i18n.T("Configuration saved. Add model settings with: mas-launcher model"))
	return nil
}

func (m *Manager) startCmd(args []string) error {
	name, rest := instanceName(args)
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	foreground := fs.Bool("foreground", false, "run in foreground")
	noBot := fs.Bool("no-bot", false, "do not start Bot")
	if e := fs.Parse(rest); e != nil {
		return e
	}
	i, repo, e := m.instance(name)
	if e != nil {
		return e
	}
	if *noBot {
		i.Bot = false
	}
	if _, e = os.Stat(python(repo)); e != nil {
		return errors.New(i18n.T("Python environment missing; run init first"))
	}
	if s, e := m.readState(name); e == nil && (processAlive(s.CorePID) || processAlive(s.BotPID)) {
		return errors.New(i18n.T("instance is already running"))
	}
	if e := m.checkAndSign(repo); e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Join(i.Path, "logs"), 0o700); e != nil {
		return e
	}
	state := State{SchemaVersion: 1, Instance: name, StartedAt: time.Now().UTC(), Commit: repository.GitOutput(repo, "rev-parse", "HEAD")}
	start := func(label, script, log string, args ...string) (int, error) {
		f, e := os.OpenFile(filepath.Join(i.Path, "logs", log), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if e != nil {
			return 0, e
		}
		c := exec.Command(python(repo), append([]string{script}, args...)...)
		c.Dir, c.Stdout, c.Stderr = repo, f, f
		if e = c.Start(); e != nil {
			f.Close()
			return 0, e
		}
		f.Close()
		fmt.Printf(i18n.T("%s started (PID %d)\n"), label, c.Process.Pid)
		return c.Process.Pid, nil
	}
	if state.CorePID, e = start(i18n.T("Core"), "core_main.py", "core.log", "--host", i.Host, "--port", fmt.Sprint(i.Port)); e != nil {
		return e
	}
	if i.Bot {
		if state.BotPID, e = start(i18n.T("Bot"), "bot.py", "bot.log"); e != nil {
			_ = killPID(state.CorePID)
			return e
		}
	}
	if e = m.writeState(name, state); e != nil {
		return e
	}
	napcatConfigInfo(repo, i)
	if !*foreground {
		fmt.Printf(i18n.T("Instance %s is running in background.\n"), name)
		return nil
	}
	<-waitSignal().Done()
	return m.stopState(name, state)
}
func (m *Manager) stopCmd(args []string) error {
	name, _ := instanceName(args)
	s, e := m.readState(name)
	if e != nil {
		return errNotRunning
	}
	// Best-effort: also stop NapCat if configured.
	if i, ok := m.Config.Instances[name]; ok && i.NapCatDir != "" {
		_ = stopNapCat(i.NapCatDir)
	}
	return m.stopState(name, s)
}
func (m *Manager) stopState(name string, s State) error {
	if !processAlive(s.CorePID) && !processAlive(s.BotPID) {
		_ = m.clearState(name)
		return errNotRunning
	}
	if s.BotPID != 0 {
		_ = killPID(s.BotPID)
	}
	if s.CorePID != 0 {
		_ = killPID(s.CorePID)
	}
	_ = m.clearState(name)
	fmt.Printf(i18n.T("Instance %s stopped.\n"), name)
	return nil
}
func (m *Manager) statusCmd(args []string) error {
	name, _ := instanceName(args)
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		}
	}
	i, repo, e := m.instance(name)
	if e != nil {
		return e
	}
	s, _ := m.readState(name)
	v := map[string]any{"instance": name, "path": i.Path, "repo": repo, "core_running": processAlive(s.CorePID), "bot_running": processAlive(s.BotPID), "core_pid": s.CorePID, "bot_pid": s.BotPID, "commit": s.Commit}
	if jsonOut {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf(i18n.T("Instance: %s\nPath: %s\nCore: %v (PID %d)\nBot: %v (PID %d)\nCommit: %s\n"), name, i.Path, v["core_running"], s.CorePID, v["bot_running"], s.BotPID, s.Commit)
	return nil
}
func (m *Manager) logsCmd(args []string) error {
	name := "default"
	service := "core"
	follow := false
	for n := 0; n < len(args); n++ {
		switch {
		case args[n] == "-f" || args[n] == "--follow":
			follow = true
		case args[n] == "--service" && n+1 < len(args):
			n++
			service = args[n]
		case strings.HasPrefix(args[n], "--service="):
			service = strings.TrimPrefix(args[n], "--service=")
		case !strings.HasPrefix(args[n], "-"):
			name = args[n]
		}
	}
	if service != "core" && service != "bot" {
		return errors.New(i18n.T("service must be core or bot"))
	}
	i, _, e := m.instance(name)
	if e != nil {
		return e
	}
	p := filepath.Join(i.Path, "logs", service+".log")
	if _, e = os.Stat(p); e != nil {
		return e
	}
	for {
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		fmt.Print(string(b))
		if !follow {
			return nil
		}
		time.Sleep(time.Second)
	}
}
func (m *Manager) updateCmd(args []string) error {
	name := "default"
	ref := ""
	for _, a := range args {
		if strings.HasPrefix(a, "--ref=") {
			ref = strings.TrimPrefix(a, "--ref=")
		} else if !strings.HasPrefix(a, "-") {
			name = a
		}
	}
	_, repo, e := m.instance(name)
	if e != nil {
		return e
	}
	if s, e := m.readState(name); e == nil && (processAlive(s.CorePID) || processAlive(s.BotPID)) {
		return errors.New(i18n.T("stop the instance before updating"))
	}
	if repository.GitOutput(repo, "status", "--porcelain") != "" {
		return errors.New(i18n.T("working tree is dirty; update refused"))
	}
	if ref == "" {
		ref = m.Config.Ref
	}
	if _, e = exec.LookPath("git"); e != nil {
		return errors.New(i18n.T("Git is required for update"))
	}
	for _, args := range [][]string{{"fetch", "--depth", "1", "origin", ref}, {"checkout", "-B", ref, "FETCH_HEAD"}} {
		c := exec.Command("git", args...)
		c.Dir, c.Stdout, c.Stderr = repo, os.Stdout, os.Stderr
		if e = c.Run(); e != nil {
			return e
		}
	}
	return m.ensureEnvironment(repo)
}
func (m *Manager) doctorCmd(args []string) error {
	name, _ := instanceName(args)
	_, repo, e := m.instance(name)
	if e != nil {
		return e
	}
	for _, check := range []struct{ label, path string }{{"project", filepath.Join(repo, "pyproject.toml")}, {"Python", python(repo)}, {"config", filepath.Join(repo, ".env")}} {
		if _, e = os.Stat(check.path); e == nil {
			fmt.Printf(i18n.T("%s: ok\n"), i18n.T(check.label))
		} else {
			fmt.Printf(i18n.T("%s: missing\n"), i18n.T(check.label))
		}
	}
	return nil
}
func (m *Manager) removeCmd(args []string) error {
	name, _ := instanceName(args)
	i, _, e := m.instance(name)
	if e != nil {
		return e
	}
	if s, e := m.readState(name); e == nil && (processAlive(s.CorePID) || processAlive(s.BotPID)) {
		return errors.New(i18n.T("stop the instance before removing it"))
	}
	fmt.Printf(i18n.T("Type yes to remove %s: "), name)
	var answer string
	if _, e = fmt.Scanln(&answer); e != nil {
		return e
	}
	if strings.ToLower(answer) != "yes" {
		return errors.New(i18n.T("cancelled"))
	}
	if e = os.RemoveAll(i.Path); e != nil {
		return e
	}
	delete(m.Config.Instances, name)
	return m.save()
}
func (m *Manager) statePath(name string) (string, error) {
	i, _, e := m.instance(name)
	if e != nil {
		return "", e
	}
	return filepath.Join(i.Path, "runtime", "state.json"), nil
}
func (m *Manager) readState(name string) (State, error) {
	p, e := m.statePath(name)
	if e != nil {
		return State{}, e
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return State{}, e
	}
	var s State
	e = json.Unmarshal(b, &s)
	return s, e
}
func (m *Manager) writeState(name string, s State) error {
	p, e := m.statePath(name)
	if e != nil {
		return e
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}
func (m *Manager) clearState(name string) error {
	p, e := m.statePath(name)
	if e != nil {
		return e
	}
	return os.Remove(p)
}
