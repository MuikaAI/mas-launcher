package models

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

// ModelSeedComment is written to configs/models.yml when no model configs
// remain, so the file stays parseable and the next run starts fresh.
const ModelSeedComment = "# Configure a model with mas-launcher model\n"

// ModelEntry is a single model config block in configs/models.yml.
// It is a map so unknown/legacy keys (think, function_call, ...) survive
// load-and-rewrite cycles losslessly.
type ModelEntry map[string]any

// ModelFile is configs/models.yml: an alias (config name) -> model entry.
type ModelFile map[string]ModelEntry

// ProviderSpec describes one embedded provider for the model wizard.
type ProviderSpec struct {
	Key         string // menu id / flag value
	Label       string // human label
	Provider    string // value written to the provider: field
	DefaultHost string // api_host written to the file; "" = omit
	ListBase    string // fixed list URL; "{host}" is substituted; "" = derive openai-compatible
	Auth        string // bearer | query | none
}

// EmbeddedProviders are the built-in providers offered by the wizard. Only
// openai/dashscope/gemini/ollama map to an app provider module; kimi/glm/
// deepseek are OpenAI-compatible and must be written as provider "Openai"
// plus api_host.
var EmbeddedProviders = []ProviderSpec{
	{Key: "openai", Label: "OpenAI", Provider: "Openai", DefaultHost: "https://api.openai.com/v1", Auth: "bearer"},
	{Key: "kimi", Label: "Kimi (Moonshot)", Provider: "Openai", DefaultHost: "https://api.moonshot.cn/v1", Auth: "bearer"},
	{Key: "glm", Label: "GLM (Zhipu AI)", Provider: "Openai", DefaultHost: "https://open.bigmodel.cn/api/paas/v4", Auth: "bearer"},
	{Key: "deepseek", Label: "DeepSeek", Provider: "Openai", DefaultHost: "https://api.deepseek.com", Auth: "bearer"},
	{Key: "dashscope", Label: "DashScope (Alibaba)", Provider: "Dashscope", ListBase: "https://dashscope.aliyuncs.com/compatible-mode/v1/models", Auth: "bearer"},
	{Key: "gemini", Label: "Gemini (Google)", Provider: "Gemini", ListBase: "https://generativelanguage.googleapis.com/v1beta/models", Auth: "query"},
	{Key: "ollama", Label: "Ollama (local)", Provider: "Ollama", DefaultHost: "http://localhost:11434", ListBase: "{host}/api/tags", Auth: "none"},
}

// FindProvider returns the embedded provider with the given key, or nil.
func FindProvider(key string) *ProviderSpec {
	for i := range EmbeddedProviders {
		if EmbeddedProviders[i].Key == key {
			return &EmbeddedProviders[i]
		}
	}
	return nil
}

// listURLs returns candidate URLs to fetch the model list for a provider.
func (s ProviderSpec) listURLs(host string) []string {
	if s.ListBase != "" {
		return []string{strings.ReplaceAll(s.ListBase, "{host}", host)}
	}
	return OpenaiModelURLs(host)
}

// ModelsPath returns the configs/models.yml path for a repo checkout.
func ModelsPath(repo string) string {
	return filepath.Join(repo, "configs", "models.yml")
}

// LoadModelFile reads and parses configs/models.yml. A missing file yields
// an empty map.
func LoadModelFile(path string) (ModelFile, error) {
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

// SaveModelFile marshals and writes configs/models.yml (2-space indent, 0600).
func SaveModelFile(path string, mf ModelFile) error {
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

// GetStr returns a key as a string, falling back to %v for non-strings.
func (e ModelEntry) GetStr(k string) string {
	v, ok := e[k]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// GetBool returns a key as a bool (false when absent or non-bool).
func (e ModelEntry) GetBool(k string) bool {
	v, ok := e[k].(bool)
	return ok && v
}

// GetInt returns a key as an int, tolerating int/int64/float64 scalars.
func (e ModelEntry) GetInt(k string) int {
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

// GetFloat returns a key as a float64, tolerating int/int64/float64 scalars.
func (e ModelEntry) GetFloat(k string) float64 {
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

// ModelDraft collects the fields the wizard knows about for one entry.
type ModelDraft struct {
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
func (d ModelDraft) ToEntry() ModelEntry {
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
func (d ModelDraft) MergeInto(existing ModelEntry) ModelEntry {
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
func (e ModelEntry) ToDraft() ModelDraft {
	d := ModelDraft{
		Provider:     e.GetStr("provider"),
		ModelName:    e.GetStr("model_name"),
		APIKey:       e.GetStr("api_key"),
		APIHost:      e.GetStr("api_host"),
		Default:      e.GetBool("default"),
		Stream:       e.GetBool("stream"),
		Multimodal:   e.GetBool("multimodal"),
		OnlineSearch: e.GetBool("online_search"),
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
	v := e.GetFloat(k)
	return &v
}

func intPtr(e ModelEntry, k string) *int {
	if _, ok := e[k]; !ok {
		return nil
	}
	v := e.GetInt(k)
	return &v
}

func boolPtr(e ModelEntry, k string) *bool {
	if _, ok := e[k]; !ok {
		return nil
	}
	v := e.GetBool(k)
	return &v
}

// ModelNames returns the config names in sorted order (yaml.v3 re-marshals
// map keys sorted, so this matches the on-disk order).
func ModelNames(mf ModelFile) []string {
	out := make([]string, 0, len(mf))
	for n := range mf {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// HasAnyDefault reports whether any config is marked default.
func HasAnyDefault(mf ModelFile) bool {
	for _, e := range mf {
		if e.GetBool("default") {
			return true
		}
	}
	return false
}

// FirstDefault returns the first (alphabetical) config marked default.
func FirstDefault(mf ModelFile) string {
	for _, n := range ModelNames(mf) {
		if mf[n].GetBool("default") {
			return n
		}
	}
	return ""
}

// SetDefault marks name as the only default config, clearing the others.
func SetDefault(mf ModelFile, name string) {
	for n, e := range mf {
		if n == name {
			e["default"] = true
		} else {
			delete(e, "default")
		}
	}
}

// EnsureDefault guarantees a default exists when entries remain. It returns
// the effective default name ("" when there are no entries).
func EnsureDefault(mf ModelFile) string {
	if len(mf) == 0 {
		return ""
	}
	if HasAnyDefault(mf) {
		return FirstDefault(mf)
	}
	names := ModelNames(mf)
	SetDefault(mf, names[0])
	return names[0]
}

// DefaultFromModelName suggests a config alias from a model id, e.g.
// "deepseek-chat" -> "deepseek", "qwen2.5:7b" -> "qwen2".
func DefaultFromModelName(s string) string {
	for _, sep := range []string{"/", ":", "-", "."} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return s
}
