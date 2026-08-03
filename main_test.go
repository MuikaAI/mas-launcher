package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataRootOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MUIKA_HOME", root)
	got, e := dataRoot()
	if e != nil {
		t.Fatal(e)
	}
	if got != root {
		t.Fatalf("got %q want %q", got, root)
	}
}
func TestEnvRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	want := map[string]string{"MASTER_ID": "123", "IPC_SECRET": "secret"}
	if e := writeEnv(p, want); e != nil {
		t.Fatal(e)
	}
	got := parseEnv(p)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s=%q want %q", k, got[k], v)
		}
	}
}
func TestWithin(t *testing.T) {
	r := t.TempDir()
	if !within(r, filepath.Join(r, "nested", "file")) {
		t.Fatal("nested path rejected")
	}
	if within(r, filepath.Join(r, "..", "outside")) {
		t.Fatal("traversal accepted")
	}
}
func TestStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MUIKA_HOME", root)
	m, e := newManager()
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(root, "instances", "demo")
	if e = os.MkdirAll(filepath.Join(p, "runtime"), 0o700); e != nil {
		t.Fatal(e)
	}
	m.Config.Instances["demo"] = Instance{Path: p}
	want := State{SchemaVersion: 1, Instance: "demo", CorePID: 42}
	if e = m.writeState("demo", want); e != nil {
		t.Fatal(e)
	}
	got, e := m.readState("demo")
	if e != nil {
		t.Fatal(e)
	}
	if got.CorePID != want.CorePID {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
