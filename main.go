package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	defaultRepo = "https://github.com/Moemu/Muika-After-Story.git"
	defaultRef  = "main"
	defaultPort = 8765
)

var version = "dev"
var errNotRunning = errors.New("instance is not running")

type Config struct {
	SchemaVersion int                 `json:"schema_version"`
	Repo          string              `json:"repo"`
	Ref           string              `json:"ref"`
	Instances     map[string]Instance `json:"instances"`
}
type Instance struct {
	Path      string `json:"path"`
	Host      string `json:"core_host"`
	Port      int    `json:"core_port"`
	Bot       bool   `json:"bot"`
	CreatedAt string `json:"created_at"`
}
type State struct {
	SchemaVersion int       `json:"schema_version"`
	Instance      string    `json:"instance"`
	CorePID       int       `json:"core_pid,omitempty"`
	BotPID        int       `json:"bot_pid,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	Commit        string    `json:"commit,omitempty"`
}
type Manager struct {
	Root   string
	Config Config
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		usage()
		return nil
	}
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println("mas-launcher " + version)
		return nil
	}
	m, err := newManager()
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		return m.initCmd(args[1:])
	case "configure", "config":
		return m.configureCmd(args[1:])
	case "model", "models":
		return m.modelCmd(args[1:])
	case "license":
		return m.licenseCmd(args[1:])
	case "start":
		return m.startCmd(args[1:])
	case "stop":
		return m.stopCmd(args[1:])
	case "restart":
		if err := m.stopCmd(args[1:]); err != nil && !errors.Is(err, errNotRunning) {
			return err
		}
		return m.startCmd(args[1:])
	case "status":
		return m.statusCmd(args[1:])
	case "logs":
		return m.logsCmd(args[1:])
	case "update":
		return m.updateCmd(args[1:])
	case "doctor":
		return m.doctorCmd(args[1:])
	case "remove":
		return m.removeCmd(args[1:])
	default:
		return fmt.Errorf("unknown command %q; run mas-launcher help", args[0])
	}
}

func usage() {
	fmt.Println(`Muika-After-Story launcher

  mas-launcher init [name]                         clone and prepare an instance
  mas-launcher configure [name]                    configure .env and model
  mas-launcher model [name]                        configure models.yml (wizard or CRUD)
  mas-launcher license [name] [--status]           view/sign the license agreement
  mas-launcher start [name] [--foreground]         start Core and Bot
  mas-launcher stop|restart [name]                 manage processes
  mas-launcher status [name] [--json]              inspect state
  mas-launcher logs [name] [--service core|bot]    view logs
  mas-launcher update [name] [--ref REF]           update the checkout
  mas-launcher doctor [name]                       diagnose environment
  mas-launcher remove [name]                       remove an instance`)
}

func newManager() (*Manager, error) {
	root, err := dataRoot()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	c := Config{SchemaVersion: 1, Repo: defaultRepo, Ref: defaultRef, Instances: map[string]Instance{}}
	if b, e := os.ReadFile(filepath.Join(root, "launcher.json")); e == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.Repo == "" {
		c.Repo = defaultRepo
	}
	if c.Ref == "" {
		c.Ref = defaultRef
	}
	if c.Instances == nil {
		c.Instances = map[string]Instance{}
	}
	return &Manager{Root: root, Config: c}, nil
}
func dataRoot() (string, error) {
	if v := os.Getenv("MUIKA_HOME"); v != "" {
		return v, nil
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "Muika-After-Story"), nil
		}
	}
	if runtime.GOOS == "darwin" {
		h, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		return filepath.Join(h, "Library", "Application Support", "Muika-After-Story"), nil
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "muika-after-story"), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(h, "AppData", "Local", "Muika-After-Story"), nil
	}
	return filepath.Join(h, ".local", "share", "muika-after-story"), nil
}
func (m *Manager) save() error {
	b, e := json.MarshalIndent(m.Config, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(m.Root, "launcher.json"), append(b, '\n'), 0o600)
}
func instanceName(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "default", args
}
func (m *Manager) instance(name string) (Instance, string, error) {
	i, ok := m.Config.Instances[name]
	if !ok {
		return Instance{}, "", fmt.Errorf("instance %q does not exist; run init first", name)
	}
	return i, filepath.Join(i.Path, "repo"), nil
}
func secret() string { b := make([]byte, 24); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func python(repo string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(repo, ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(repo, ".venv", "bin", "python")
}
func command(dir, program string, args ...string) error {
	c := exec.Command(program, args...)
	c.Dir, c.Stdout, c.Stderr = dir, os.Stdout, os.Stderr
	return c.Run()
}
func commandContext(ctx context.Context, dir, program string, args ...string) error {
	c := exec.CommandContext(ctx, program, args...)
	c.Dir, c.Stdout, c.Stderr = dir, os.Stdout, os.Stderr
	return c.Run()
}
func randomPort() int { return defaultPort }
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func parseEnv(path string) map[string]string {
	out := map[string]string{}
	b, e := os.ReadFile(path)
	if e != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		p := strings.SplitN(line, "=", 2)
		if len(p) == 2 && !strings.HasPrefix(line, "#") {
			out[strings.TrimSpace(p[0])] = strings.TrimSpace(p[1])
		}
	}
	return out
}
func writeEnv(path string, values map[string]string) error {
	var b strings.Builder
	for _, k := range sortedKeys(values) {
		b.WriteString(k + "=" + values[k] + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
func waitSignal() context.Context {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	return ctx
}
func _unusedIO() { _, _ = io.Copy(io.Discard, strings.NewReader("")) }
