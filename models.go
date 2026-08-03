package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// modelSeedComment is written to configs/models.yml when no model configs
// remain, so the file stays parseable and the next run starts fresh.
const modelSeedComment = "# Configure a model with mas-launcher model\n"

// ModelEntry is a single model config block in configs/models.yml.
// It is a map so unknown/legacy keys (think, function_call, ...) survive
// load-and-rewrite cycles losslessly.
type ModelEntry map[string]any

// ModelFile is configs/models.yml: an alias (config name) -> model entry.
type ModelFile map[string]ModelEntry

// providerSpec describes one embedded provider for the model wizard.
type providerSpec struct {
	key         string // menu id / flag value
	label       string // human label
	provider    string // value written to the provider: field
	defaultHost string // api_host written to the file; "" = omit
	listBase    string // fixed list URL; "{host}" is substituted; "" = derive openai-compatible
	auth        string // bearer | query | none
}

// embeddedProviders are the built-in providers offered by the wizard. Only
// openai/dashscope/gemini/ollama map to an app provider module; kimi/glm/
// deepseek are OpenAI-compatible and must be written as provider "Openai"
// plus api_host.
var embeddedProviders = []providerSpec{
	{key: "openai", label: "OpenAI", provider: "Openai", defaultHost: "https://api.openai.com/v1", auth: "bearer"},
	{key: "kimi", label: "Kimi (Moonshot)", provider: "Openai", defaultHost: "https://api.moonshot.cn/v1", auth: "bearer"},
	{key: "glm", label: "GLM (Zhipu AI)", provider: "Openai", defaultHost: "https://open.bigmodel.cn/api/paas/v4", auth: "bearer"},
	{key: "deepseek", label: "DeepSeek", provider: "Openai", defaultHost: "https://api.deepseek.com", auth: "bearer"},
	{key: "dashscope", label: "DashScope (Alibaba)", provider: "Dashscope", listBase: "https://dashscope.aliyuncs.com/compatible-mode/v1/models", auth: "bearer"},
	{key: "gemini", label: "Gemini (Google)", provider: "Gemini", listBase: "https://generativelanguage.googleapis.com/v1beta/models", auth: "query"},
	{key: "ollama", label: "Ollama (local)", provider: "Ollama", defaultHost: "http://localhost:11434", listBase: "{host}/api/tags", auth: "none"},
}

// findProvider returns the embedded provider with the given key, or nil.
func findProvider(key string) *providerSpec {
	for i := range embeddedProviders {
		if embeddedProviders[i].key == key {
			return &embeddedProviders[i]
		}
	}
	return nil
}

// listURLs returns candidate URLs to fetch the model list for a provider.
func (s providerSpec) listURLs(host string) []string {
	if s.listBase != "" {
		return []string{strings.ReplaceAll(s.listBase, "{host}", host)}
	}
	return openaiModelURLs(host)
}

// modelsPath returns the configs/models.yml path for a repo checkout.
func modelsPath(repo string) string {
	return filepath.Join(repo, "configs", "models.yml")
}

// loadModelFile reads and parses configs/models.yml. A missing file yields
// an empty map.
func loadModelFile(path string) (ModelFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ModelFile{}, nil
		}
		return nil, err
	}
	mf := ModelFile{}
	if err := yaml.Unmarshal(b, &mf); err != nil {
		return nil, err
	}
	if mf == nil {
		mf = ModelFile{}
	}
	return mf, nil
}

// saveModelFile marshals and writes configs/models.yml (2-space indent, 0600).
func saveModelFile(path string, mf ModelFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(mf); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// getStr returns a key as a string, falling back to %v for non-strings.
func (e ModelEntry) getStr(k string) string {
	v, ok := e[k]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// getBool returns a key as a bool (false when absent or non-bool).
func (e ModelEntry) getBool(k string) bool {
	v, ok := e[k].(bool)
	return ok && v
}

// getInt returns a key as an int, tolerating int/int64/float64 scalars.
func (e ModelEntry) getInt(k string) int {
	switch v := e[k].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// getFloat returns a key as a float64, tolerating int/int64/float64 scalars.
func (e ModelEntry) getFloat(k string) float64 {
	switch v := e[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

// modelDraft collects the fields the wizard knows about for one entry.
type modelDraft struct {
	Name         string
	Provider     string
	ModelName    string
	APIKey       string
	APIHost      string
	Default      bool
	Stream       bool
	Multimodal   bool
	OnlineSearch bool

	EnableThinking *bool
	ThinkingBudget *int
	TopK           *float64
	Temperature    *float64
	TopP           *float64
	MaxTokens      *int
	ExtraBody      string
	InputPrice     *float64
	OutputPrice    *float64
	CachedPrice    *float64
}

// ToEntry builds a fresh ModelEntry from the draft, omitting unset optionals.
func (d modelDraft) ToEntry() ModelEntry {
	e := ModelEntry{}
	e["provider"] = d.Provider
	if d.ModelName != "" {
		e["model_name"] = d.ModelName
	}
	if d.APIKey != "" {
		e["api_key"] = d.APIKey
	}
	if d.APIHost != "" {
		e["api_host"] = d.APIHost
	}
	if d.Default {
		e["default"] = true
	}
	if d.Stream {
		e["stream"] = true
	}
	if d.Multimodal {
		e["multimodal"] = true
	}
	if d.OnlineSearch {
		e["online_search"] = true
	}
	if d.EnableThinking != nil {
		e["enable_thinking"] = *d.EnableThinking
	}
	if d.ThinkingBudget != nil {
		e["thinking_budget"] = *d.ThinkingBudget
	}
	if d.TopK != nil {
		e["top_k"] = *d.TopK
	}
	if d.Temperature != nil {
		e["temperature"] = *d.Temperature
	}
	if d.TopP != nil {
		e["top_p"] = *d.TopP
	}
	if d.MaxTokens != nil {
		e["max_tokens"] = *d.MaxTokens
	}
	if d.ExtraBody != "" {
		var v any
		if err := json.Unmarshal([]byte(d.ExtraBody), &v); err == nil {
			e["extra_body"] = v
		}
	}
	if d.InputPrice != nil {
		e["input_price"] = *d.InputPrice
	}
	if d.OutputPrice != nil {
		e["output_price"] = *d.OutputPrice
	}
	if d.CachedPrice != nil {
		e["cached_price"] = *d.CachedPrice
	}
	return e
}

// MergeInto overlays the draft's known keys onto an existing entry,
// preserving any unknown/legacy keys for lossless modification.
func (d modelDraft) MergeInto(existing ModelEntry) ModelEntry {
	out := make(ModelEntry, len(existing)+8)
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range d.ToEntry() {
		out[k] = v
	}
	return out
}

// ToDraft pre-fills a draft from an existing entry (for the modify flow).
func (e ModelEntry) ToDraft() modelDraft {
	d := modelDraft{
		Provider:     e.getStr("provider"),
		ModelName:    e.getStr("model_name"),
		APIKey:       e.getStr("api_key"),
		APIHost:      e.getStr("api_host"),
		Default:      e.getBool("default"),
		Stream:       e.getBool("stream"),
		Multimodal:   e.getBool("multimodal"),
		OnlineSearch: e.getBool("online_search"),
	}
	d.EnableThinking = boolPtr(e, "enable_thinking")
	d.ThinkingBudget = intPtr(e, "thinking_budget")
	d.TopK = floatPtr(e, "top_k")
	d.Temperature = floatPtr(e, "temperature")
	d.TopP = floatPtr(e, "top_p")
	d.MaxTokens = intPtr(e, "max_tokens")
	d.InputPrice = floatPtr(e, "input_price")
	d.OutputPrice = floatPtr(e, "output_price")
	d.CachedPrice = floatPtr(e, "cached_price")
	if v, ok := e["extra_body"]; ok {
		if b, err := json.Marshal(v); err == nil {
			d.ExtraBody = string(b)
		}
	}
	return d
}

func floatPtr(e ModelEntry, k string) *float64 {
	if _, ok := e[k]; !ok {
		return nil
	}
	v := e.getFloat(k)
	return &v
}

func intPtr(e ModelEntry, k string) *int {
	if _, ok := e[k]; !ok {
		return nil
	}
	v := e.getInt(k)
	return &v
}

func boolPtr(e ModelEntry, k string) *bool {
	if _, ok := e[k]; !ok {
		return nil
	}
	v := e.getBool(k)
	return &v
}

// modelNames returns the config names in sorted order (yaml.v3 re-marshals
// map keys sorted, so this matches the on-disk order).
func modelNames(mf ModelFile) []string {
	out := make([]string, 0, len(mf))
	for n := range mf {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// hasAnyDefault reports whether any config is marked default.
func hasAnyDefault(mf ModelFile) bool {
	for _, e := range mf {
		if e.getBool("default") {
			return true
		}
	}
	return false
}

// firstDefault returns the first (alphabetical) config marked default.
func firstDefault(mf ModelFile) string {
	for _, n := range modelNames(mf) {
		if mf[n].getBool("default") {
			return n
		}
	}
	return ""
}

// setDefault marks name as the only default config, clearing the others.
func setDefault(mf ModelFile, name string) {
	for n, e := range mf {
		if n == name {
			e["default"] = true
		} else {
			delete(e, "default")
		}
	}
}

// ensureDefault guarantees a default exists when entries remain. It returns
// the effective default name ("" when there are no entries).
func ensureDefault(mf ModelFile) string {
	if len(mf) == 0 {
		return ""
	}
	if hasAnyDefault(mf) {
		return firstDefault(mf)
	}
	names := modelNames(mf)
	setDefault(mf, names[0])
	return names[0]
}

// defaultFromModelName suggests a config alias from a model id, e.g.
// "deepseek-chat" -> "deepseek", "qwen2.5:7b" -> "qwen2".
func defaultFromModelName(s string) string {
	for _, sep := range []string{"/", ":", "-", "."} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return s
}
