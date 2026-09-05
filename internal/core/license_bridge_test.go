package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("MAS_TEST_AGREEMENT_HELPER"); mode != "" {
		if len(os.Args) != 3 || os.Args[1] != "-c" || !strings.Contains(os.Args[2], "muika.agreement") {
			os.Exit(3)
		}
		if _, err := os.Stat("instance-marker"); err != nil {
			os.Exit(4)
		}
		switch mode {
		case "missing":
			os.Exit(42)
		case "broken":
			fmt.Fprintln(os.Stderr, "broken agreement resource")
			os.Exit(1)
		case "incomplete":
			fmt.Println(`{"content":{"title":"T","text":"X","updated":"2026-02-01"}}`)
		case "shared":
			fmt.Println(`{"content":{"title":"Python","text":"X","updated":"2026-02-01"},"state":{"has_agreed":true,"version":"older"},"needs_acceptance":false}`)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func bridgeFixture(t *testing.T, mode string) string {
	t.Helper()
	repo := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(python(repo)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(python(repo), contents, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "instance-marker"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "configs"), 0o700); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(AgreementContent{Title: "legacy", Text: "X", Updated: "2026-02-01"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agreementContentPath(repo), content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAS_TEST_AGREEMENT_HELPER", mode)
	return repo
}

func TestBridgeOwnsAgreementDecision(t *testing.T) {
	status, err := readAgreementStatus(bridgeFixture(t, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	if !status.shared || *status.NeedsAcceptance || status.Content.Title != "Python" {
		t.Fatalf("Python decision was not used: %+v", status)
	}
}

func TestMissingBridgeUsesLegacyContent(t *testing.T) {
	status, err := readAgreementStatus(bridgeFixture(t, "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if status.shared || !*status.NeedsAcceptance || status.Content.Title != "legacy" {
		t.Fatalf("legacy fallback failed: %+v", status)
	}
}

func TestBrokenBridgeDoesNotFallBack(t *testing.T) {
	for _, mode := range []string{"broken", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			_, err := readAgreementStatus(bridgeFixture(t, mode))
			if err == nil {
				t.Fatal("bridge failure must not fall back")
			}
			if mode == "broken" && !strings.Contains(err.Error(), "broken agreement resource") {
				t.Fatalf("lost diagnostic: %v", err)
			}
		})
	}
}
