package models

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestYAMLRoundTripPreservesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.yml")
	mf := ModelFile{
		"deepseek": ModelEntry{
			"provider":    "Openai",
			"model_name":  "deepseek-chat",
			"api_key":     "sk-test",
			"api_host":    "https://api.deepseek.com",
			"default":     true,
			"extra_body":  map[string]any{"thinking": map[string]any{"type": "enabled"}},
			"think":       2,
			"input_price": 3.0,
			"unknown_key": "legacy-value",
		},
	}
	if err := SaveModelFile(path, mf); err != nil {
		t.Fatal(err)
	}
	got, err := LoadModelFile(path)
	if err != nil {
		t.Fatal(err)
	}
	e := got["deepseek"]
	if e.GetStr("provider") != "Openai" || e.GetStr("model_name") != "deepseek-chat" {
		t.Errorf("core fields lost: %#v", e)
	}
	if !e.GetBool("default") {
		t.Error("default lost")
	}
	if e.GetInt("think") != 2 {
		t.Errorf("unknown int field lost: %#v", e["think"])
	}
	if e.GetStr("unknown_key") != "legacy-value" {
		t.Errorf("unknown string field lost: %#v", e["unknown_key"])
	}
	if e.GetFloat("input_price") != 3.0 {
		t.Errorf("price lost: %#v", e["input_price"])
	}
	if _, ok := e["extra_body"]; !ok {
		t.Error("extra_body lost")
	}
}

func TestOpenaiModelURLs(t *testing.T) {
	cases := []struct {
		host string
		want []string
	}{
		{"https://api.deepseek.com", []string{"https://api.deepseek.com/models", "https://api.deepseek.com/v1/models"}},
		{"https://api.moonshot.cn/v1", []string{"https://api.moonshot.cn/v1/models"}},
		{"https://api.openai.com/v1/", []string{"https://api.openai.com/v1/models"}},
	}
	for _, c := range cases {
		got := OpenaiModelURLs(c.host)
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v want %v", c.host, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: got %v want %v", c.host, got, c.want)
			}
		}
	}
}

func TestSetDefaultAndEnsure(t *testing.T) {
	mf := ModelFile{
		"a": ModelEntry{"default": true},
		"b": ModelEntry{},
	}
	SetDefault(mf, "b")
	if mf["a"].GetBool("default") || !mf["b"].GetBool("default") {
		t.Fatal("SetDefault did not move default")
	}
	delete(mf, "b")
	if def := EnsureDefault(mf); def != "a" {
		t.Fatalf("EnsureDefault after delete = %q want a", def)
	}
	mf2 := ModelFile{"z": ModelEntry{}, "a": ModelEntry{}}
	if def := EnsureDefault(mf2); def != "a" {
		t.Fatalf("EnsureDefault fresh = %q want a", def)
	}
	if !mf2["a"].GetBool("default") {
		t.Fatal("default not auto-assigned")
	}
}

func TestDefaultFromModelName(t *testing.T) {
	cases := map[string]string{
		"deepseek-chat":    "deepseek",
		"deepseek-v4-pro":  "deepseek",
		"qwen2.5:7b":       "qwen2",
		"gemini-2.5-flash": "gemini",
		"gpt-4o":           "gpt",
		"llama3:8b":        "llama3",
	}
	for in, want := range cases {
		if got := DefaultFromModelName(in); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
}

func TestMergeIntoPreservesUnknown(t *testing.T) {
	existing := ModelEntry{"think": 2, "provider": "Openai", "model_name": "deepseek-chat"}
	d := ModelDraft{Provider: "Openai", ModelName: "deepseek-v4", Default: true}
	got := d.MergeInto(existing)
	if got.GetInt("think") != 2 {
		t.Error("unknown field lost on merge")
	}
	if got.GetStr("model_name") != "deepseek-v4" {
		t.Error("known field not updated")
	}
	if !got.GetBool("default") {
		t.Error("default not set")
	}
}

func TestFetchModelListOpenAICompatible(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Write([]byte(`{"data":[{"id":"a-model"},{"id":"deepseek-chat"},{"id":"deepseek-chat"},{"id":"list"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	spec := ProviderSpec{Provider: "Openai", Auth: "bearer"}
	got, err := FetchModelList(spec, srv.URL, "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", gotAuth)
	}
	want := "a-model,deepseek-chat"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFetchModelListGemini(t *testing.T) {
	gem := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "gem-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-flash"},{"name":"models/gemini-pro"}]}`))
	}))
	defer gem.Close()

	spec := ProviderSpec{Provider: "Gemini", ListBase: gem.URL + "/v1beta/models", Auth: "query"}
	got, err := FetchModelList(spec, "", "gem-key")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "gemini-2.5-flash,gemini-pro" {
		t.Fatalf("got %v", got)
	}
}

func TestEmbeddedProvidersMapToAppModules(t *testing.T) {
	dir := filepath.Join("..", "..", "muika", "llm", "providers")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("app provider dir not present in this checkout")
	}
	for _, sp := range EmbeddedProviders {
		name := strings.ToLower(sp.Provider)
		if _, err := os.Stat(filepath.Join(dir, name+".py")); err != nil {
			t.Errorf("provider %q writes %q which has no muika/llm/providers/%s.py", sp.Key, sp.Provider, name)
		}
	}
}
