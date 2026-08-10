package core

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNeedsSign(t *testing.T) {
	cases := []struct {
		hasAgreed bool
		stored    string
		updated   string
		want      bool
	}{
		{false, "2026-02-01", "2026-02-01", true},
		{true, "2026-02-01", "2026-02-01", false},
		{true, "2026-01-01", "2026-02-01", true},
		{true, "2026-03-01", "2026-02-01", false},
		{true, "", "2026-02-01", true},
		{true, "garbage", "2026-02-01", true},
		{true, "2026-02-01", "", true},
	}
	for _, c := range cases {
		if got := needsSign(c.hasAgreed, c.stored, c.updated); got != c.want {
			t.Errorf("needsSign(%v,%q,%q) = %v want %v", c.hasAgreed, c.stored, c.updated, got, c.want)
		}
	}
}

func TestAgreementStateDirResolution(t *testing.T) {
	repo := t.TempDir()
	if got := agreementStateDir(repo); got != filepath.Join(repo, "data") {
		t.Errorf("absent MUIKA_DATA_DIR: got %q want %q", got, filepath.Join(repo, "data"))
	}
	writeEnv(filepath.Join(repo, ".env"), map[string]string{"MUIKA_DATA_DIR": "./data"})
	if got := agreementStateDir(repo); got != filepath.Join(repo, "data") {
		t.Errorf("./data: got %q", got)
	}
	writeEnv(filepath.Join(repo, ".env"), map[string]string{"MUIKA_DATA_DIR": "data"})
	if got := agreementStateDir(repo); got != filepath.Join(repo, "data") {
		t.Errorf("data: got %q", got)
	}
	abs := t.TempDir()
	writeEnv(filepath.Join(repo, ".env"), map[string]string{"MUIKA_DATA_DIR": abs})
	if got := agreementStateDir(repo); got != abs {
		t.Errorf("absolute: got %q want %q", got, abs)
	}
}

func TestAgreementContentMissingErrors(t *testing.T) {
	repo := t.TempDir()
	if _, err := loadAgreementContent(repo); err == nil {
		t.Fatal("expected error for missing agreement file")
	}
	path := agreementContentPath(repo)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"title":"T","text":"X","updated":"2026-02-01"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := loadAgreementContent(repo)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "T" || c.Updated != "2026-02-01" {
		t.Fatalf("bad content: %+v", c)
	}
	if err := os.WriteFile(path, []byte(`{"title":"T","text":"X","updated":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAgreementContent(repo); err == nil {
		t.Fatal("expected error for empty updated")
	}
	if err := os.WriteFile(path, []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAgreementContent(repo); err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestAgreementStateRoundTrip(t *testing.T) {
	repo := t.TempDir()
	writeEnv(filepath.Join(repo, ".env"), map[string]string{"MUIKA_DATA_DIR": "./data"})
	content := AgreementContent{Title: "T", Text: "X", Updated: "2026-02-01"}
	if err := sign(repo, content); err != nil {
		t.Fatal(err)
	}
	st, err := loadAgreementState(agreementStatePath(repo))
	if err != nil {
		t.Fatal(err)
	}
	if !st.HasAgreed || st.Version != "2026-02-01" {
		t.Fatalf("bad state: %+v", st)
	}
	if _, err := time.Parse(isoStampLayout, st.Timestamp); err != nil {
		t.Fatalf("timestamp %q not parseable: %v", st.Timestamp, err)
	}
	if strings.ContainsAny(st.Timestamp, "Z+") {
		t.Fatalf("timestamp %q has a timezone offset; Python fromisoformat needs tz-less", st.Timestamp)
	}
	b, err := os.ReadFile(agreementStatePath(repo))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"has_agreed", "timestamp", "version"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in state JSON", k)
		}
	}
	if len(m) != 3 {
		t.Errorf("unexpected extra keys in state JSON: %v", m)
	}
}

func TestConfirmAgreement(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"是\n", true},
		{"y\n", true},
		{"YES\n", true},
		{"否\n", false},
		{"\n", false},
		{"", false}, // EOF counts as decline
	}
	for _, c := range cases {
		p := &prompt{r: bufio.NewReader(strings.NewReader(c.in))}
		got, err := confirmAgreement(p)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("%q: got %v want %v", c.in, got, c.want)
		}
	}
}
