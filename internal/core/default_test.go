package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// newEnvStatusManager returns a Manager rooted at a temp dir and a fresh
// repo checkout directory.
func newEnvStatusManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("MUIKA_HOME", root)
	m, err := newManager()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	return m, repo
}

// fakePython creates a venv python at python(repo) (GOOS-aware), so tests
// are portable across platforms.
func fakePython(t *testing.T, repo string) {
	t.Helper()
	p := python(repo)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeFile creates parent directories and writes content with 0600 perms.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const defaultedModelYAML = "m:\n  provider: Openai\n  model_name: x\n  default: true\n"

func TestEnvStatusAllMissing(t *testing.T) {
	m, repo := newEnvStatusManager(t)
	got, err := m.envStatus(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"python", "env-file", "model"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEnvStatusComplete(t *testing.T) {
	m, repo := newEnvStatusManager(t)
	fakePython(t, repo)
	writeFile(t, filepath.Join(repo, ".env"), "MASTER_ID=123\n")
	writeFile(t, filepath.Join(repo, "configs", "models.yml"), defaultedModelYAML)
	got, err := m.envStatus(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v want empty", got)
	}
}

func TestEnvStatusMasterIDBlank(t *testing.T) {
	for name, env := range map[string]string{
		"empty value": "MASTER_ID=\n",
		"key absent":  "OTHER=1\n",
	} {
		t.Run(name, func(t *testing.T) {
			m, repo := newEnvStatusManager(t)
			fakePython(t, repo)
			writeFile(t, filepath.Join(repo, ".env"), env)
			writeFile(t, filepath.Join(repo, "configs", "models.yml"), defaultedModelYAML)
			got, err := m.envStatus(repo)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"master-id"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

func TestEnvStatusNoPython(t *testing.T) {
	m, repo := newEnvStatusManager(t)
	writeFile(t, filepath.Join(repo, ".env"), "MASTER_ID=123\n")
	writeFile(t, filepath.Join(repo, "configs", "models.yml"), defaultedModelYAML)
	got, err := m.envStatus(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"python"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEnvStatusModelMissing(t *testing.T) {
	for name, content := range map[string]string{
		"comment only": "# Configure a model with mas-launcher model\n",
		"no default":   "m:\n  provider: Openai\n  model_name: x\n",
	} {
		t.Run(name, func(t *testing.T) {
			m, repo := newEnvStatusManager(t)
			fakePython(t, repo)
			writeFile(t, filepath.Join(repo, ".env"), "MASTER_ID=123\n")
			writeFile(t, filepath.Join(repo, "configs", "models.yml"), content)
			got, err := m.envStatus(repo)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"model"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

func TestEnvStatusCorruptModelFile(t *testing.T) {
	m, repo := newEnvStatusManager(t)
	fakePython(t, repo)
	writeFile(t, filepath.Join(repo, ".env"), "MASTER_ID=123\n")
	writeFile(t, filepath.Join(repo, "configs", "models.yml"), "::: not yaml\n")
	if _, err := m.envStatus(repo); err == nil {
		t.Fatal("expected error for corrupt models.yml")
	}
}

// TestDefaultCmdAlreadyRunning builds a complete synthetic default instance
// whose state claims a live Core PID (our own process) and asserts that
// defaultCmd short-circuits before starting any subprocess.
func TestDefaultCmdAlreadyRunning(t *testing.T) {
	m, _ := newEnvStatusManager(t)
	path := filepath.Join(m.Root, "instances", "default")
	repo := filepath.Join(path, "repo")
	fakePython(t, repo)
	writeFile(t, filepath.Join(repo, ".env"), "MASTER_ID=123\n")
	writeFile(t, filepath.Join(repo, "configs", "models.yml"), defaultedModelYAML)
	m.Config.Instances["default"] = Instance{Path: path}
	state := fmt.Sprintf(`{"schema_version":1,"instance":"default","core_pid":%d}`, os.Getpid())
	writeFile(t, filepath.Join(path, "runtime", "state.json"), state)
	if err := m.defaultCmd(nil); err != nil {
		t.Fatal(err)
	}
}
