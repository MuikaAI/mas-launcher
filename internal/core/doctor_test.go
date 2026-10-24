package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestModelConfigProblems(t *testing.T) {
	t.Setenv("MUIKA_LANG", "en")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")

	repo := t.TempDir()
	modelsPath := filepath.Join(repo, "configs", "models.yml")
	providerPath := filepath.Join(repo, "muika", "llm", "providers", "openai.py")

	problems, err := modelConfigProblems(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "no model configured") {
		t.Fatalf("missing models.yml: got %v", problems)
	}

	writeTestFile(t, providerPath, "")
	writeTestFile(t, modelsPath, "main:\n  provider: Openai\n  default: true\n")
	if problems, err = modelConfigProblems(repo); err != nil || len(problems) != 0 {
		t.Fatalf("valid config: %v err=%v", problems, err)
	}

	writeTestFile(t, modelsPath, "main:\n  provider: Azure\n  default: true\n")
	if problems, err = modelConfigProblems(repo); err != nil || len(problems) != 1 || !strings.Contains(problems[0], "azure") {
		t.Fatalf("unknown provider: %v err=%v", problems, err)
	}

	writeTestFile(t, modelsPath, "main:\n  model_name: x\n  default: true\n")
	if problems, err = modelConfigProblems(repo); err != nil || len(problems) != 1 || !strings.Contains(problems[0], "provider is empty") {
		t.Fatalf("empty provider: %v err=%v", problems, err)
	}

	writeTestFile(t, modelsPath, "main:\n  provider: Openai\n")
	if problems, err = modelConfigProblems(repo); err != nil || len(problems) != 1 || !strings.Contains(problems[0], "no default model") {
		t.Fatalf("no default: %v err=%v", problems, err)
	}
}
