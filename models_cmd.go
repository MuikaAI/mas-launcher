package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// modelCmd implements `mas-launcher model [name]`.
func (m *Manager) modelCmd(args []string) error {
	name, rest := instanceName(args)
	_, repo, err := m.instance(name)
	if err != nil {
		return err
	}
	path := modelsPath(repo)

	fs := flag.NewFlagSet("model", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	list := fs.Bool("list", false, "list configured models")
	del := fs.String("delete", "", "delete a model config")
	yes := fs.Bool("yes", false, "skip confirmation for --delete")
	setDefault := fs.String("set-default", "", "set the default model config")
	cname := fs.String("name", "", "config name (non-interactive create/modify)")
	provider := fs.String("provider", "", "provider key or value (non-interactive)")
	model := fs.String("model", "", "model name (non-interactive)")
	apiKey := fs.String("api-key", "", "API key (non-interactive)")
	apiHost := fs.String("api-host", "", "custom API host (non-interactive)")
	stream := fs.Bool("stream", false, "enable streaming (non-interactive)")
	temperature := fs.Float64("temperature", 0, "temperature (non-interactive)")
	maxTokens := fs.Int("max-tokens", 0, "max tokens (non-interactive)")
	makeDefault := fs.Bool("default", false, "make this config the default (non-interactive)")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	switch {
	case *list:
		return m.modelList(path)
	case *del != "":
		return m.modelDelete(path, *del, *yes)
	case *setDefault != "":
		return m.modelSetDefault(path, *setDefault)
	case *cname != "":
		if *provider == "" || *model == "" {
			return errors.New("--provider and --model are required with --name")
		}
		return m.modelWrite(path, *cname, *provider, *model, *apiKey, *apiHost, *stream, *temperature, *maxTokens, *makeDefault)
	}
	return m.modelWizard(path)
}

// modelWizard routes to the fresh-init flow (Flow A) or the CRUD menu
// (Flow B) depending on whether any configs exist.
func (m *Manager) modelWizard(path string) error {
	mf, err := loadModelFile(path)
	if err != nil {
		return err
	}
	if len(mf) == 0 {
		fmt.Println("No model configs found in configs/models.yml.")
		p := newPrompt()
		name, err := m.wizardCreate(p, mf)
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if err := saveModelFile(path, mf); err != nil {
			return err
		}
		fmt.Printf("Saved model config %q. Run `mas-launcher model` to manage models.\n", name)
		return nil
	}
	return m.wizardFlowB(path, mf)
}

// modelList prints the configured models.
func (m *Manager) modelList(path string) error {
	mf, err := loadModelFile(path)
	if err != nil {
		return err
	}
	if len(mf) == 0 {
		fmt.Println("No model configs found in configs/models.yml.")
		return nil
	}
	for _, n := range modelNames(mf) {
		e := mf[n]
		line := fmt.Sprintf("%-12s provider=%-9s model=%s", n, e.getStr("provider"), e.getStr("model_name"))
		if host := e.getStr("api_host"); host != "" {
			line += "  api_host=" + host
		}
		if e.getBool("default") {
			line += "  (default)"
		}
		fmt.Println(line)
	}
	return nil
}

// modelDelete removes a config non-interactively.
func (m *Manager) modelDelete(path, name string, yes bool) error {
	mf, err := loadModelFile(path)
	if err != nil {
		return err
	}
	if _, ok := mf[name]; !ok {
		return fmt.Errorf("model config %q not found", name)
	}
	if !yes {
		fmt.Printf("Type yes to delete %q: ", name)
		var answer string
		if _, e := fmt.Scanln(&answer); e != nil {
			return e
		}
		if strings.ToLower(answer) != "yes" {
			return errors.New("cancelled")
		}
	}
	wasDefault := mf[name].getBool("default")
	delete(mf, name)
	if len(mf) == 0 {
		if err := os.WriteFile(path, []byte(modelSeedComment), 0o600); err != nil {
			return err
		}
		fmt.Printf("Deleted %q. No model configs remain.\n", name)
		return nil
	}
	newDefault := ensureDefault(mf)
	if err := saveModelFile(path, mf); err != nil {
		return err
	}
	fmt.Printf("Deleted %q.\n", name)
	if wasDefault && newDefault != "" {
		fmt.Printf("Default is now %q.\n", newDefault)
	}
	return nil
}

// modelSetDefault marks a config as the default non-interactively.
func (m *Manager) modelSetDefault(path, name string) error {
	mf, err := loadModelFile(path)
	if err != nil {
		return err
	}
	if _, ok := mf[name]; !ok {
		return fmt.Errorf("model config %q not found", name)
	}
	setDefault(mf, name)
	if err := saveModelFile(path, mf); err != nil {
		return err
	}
	fmt.Printf("Default model is now %q.\n", name)
	return nil
}

// modelWrite creates or overwrites a config non-interactively, sharing the
// wizard's write path.
func (m *Manager) modelWrite(path, cname, providerKey, modelName, apiKey, apiHost string, stream bool, temperature float64, maxTokens int, makeDefault bool) error {
	mf, err := loadModelFile(path)
	if err != nil {
		return err
	}
	d := modelDraft{ModelName: modelName, APIKey: apiKey, Stream: stream}
	if temperature != 0 {
		d.Temperature = &temperature
	}
	if maxTokens != 0 {
		d.MaxTokens = &maxTokens
	}
	if sp := findProvider(providerKey); sp != nil {
		d.Provider = sp.provider
		d.APIHost = sp.defaultHost
		if apiHost != "" {
			d.APIHost = apiHost
		}
	} else {
		d.Provider = providerKey
		d.APIHost = apiHost
	}
	if existing, ok := mf[cname]; ok {
		if d.APIHost == "" {
			d.APIHost = existing.getStr("api_host")
		}
		mf[cname] = d.MergeInto(existing)
	} else {
		mf[cname] = d.ToEntry()
	}
	if makeDefault {
		setDefault(mf, cname)
	}
	ensureDefault(mf)
	if err := saveModelFile(path, mf); err != nil {
		return err
	}
	fmt.Printf("Saved model config %q.\n", cname)
	return nil
}

// wizardFlowB is the CRUD menu shown when models.yml already has configs.
func (m *Manager) wizardFlowB(path string, mf ModelFile) error {
	p := newPrompt()
	for {
		names := modelNames(mf)
		if len(names) == 0 {
			fmt.Println("No model configs remain. Run `mas-launcher model` to set one up.")
			return nil
		}
		fmt.Println("Existing model configs:")
		for i, n := range names {
			def := ""
			if mf[n].getBool("default") {
				def = "  (default)"
			}
			fmt.Printf("  %d) %-12s %-9s %s%s\n", i+1, n, mf[n].getStr("provider"), mf[n].getStr("model_name"), def)
		}
		fmt.Println("  0) Create new model")
		fmt.Println("  d) Delete a model")
		fmt.Println("  s) Set default model")
		fmt.Println("  q) Back")
		fmt.Print("> ")
		line, err := p.r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		switch line {
		case "q":
			return nil
		case "d":
			if err := m.wizardDelete(p, path, mf); err != nil {
				return err
			}
		case "s":
			if err := m.wizardSetDefault(p, path, mf); err != nil {
				return err
			}
		case "0":
			if _, err := m.wizardCreate(p, mf); err != nil {
				return err
			}
			if err := saveModelFile(path, mf); err != nil {
				return err
			}
		default:
			n, err := strconv.Atoi(line)
			if err == nil && n >= 1 && n <= len(names) {
				if err := m.wizardModify(p, path, mf, names[n-1]); err != nil {
					return err
				}
			} else {
				fmt.Println("Invalid choice.")
			}
		}
	}
}

// wizardCreate walks provider -> model list -> params and writes the new
// entry into mf. It returns the created config name ("" if cancelled).
func (m *Manager) wizardCreate(p *prompt, mf ModelFile) (string, error) {
	d, err := m.wizardProviderAndModel(p)
	if err != nil {
		return "", err
	}
	if d == nil {
		return "", nil
	}
	for {
		d.Name, err = p.ask("Config name (alias)", defaultFromModelName(d.ModelName))
		if err != nil {
			return "", err
		}
		d.Name = strings.TrimSpace(d.Name)
		if d.Name == "" {
			fmt.Println("Config name required.")
			continue
		}
		if _, exists := mf[d.Name]; exists {
			fmt.Printf("Config %q already exists.\n", d.Name)
			continue
		}
		break
	}
	if err := m.wizardParams(p, d); err != nil {
		return "", err
	}
	if !hasAnyDefault(mf) {
		d.Default = true
		fmt.Println("This will be the default model.")
	} else {
		d.Default, err = p.askBool("Set as default model", false)
		if err != nil {
			return "", err
		}
	}
	mf[d.Name] = d.ToEntry()
	if d.Default {
		setDefault(mf, d.Name)
	}
	ensureDefault(mf)
	return d.Name, nil
}

// wizardProviderAndModel guides provider selection, API key, and model
// picking (from the fetched list, with a manual fallback).
func (m *Manager) wizardProviderAndModel(p *prompt) (*modelDraft, error) {
	keys := make([]string, 0, len(embeddedProviders)+1)
	labels := make([]string, 0, len(embeddedProviders)+1)
	for _, sp := range embeddedProviders {
		keys = append(keys, sp.key)
		labels = append(labels, sp.label)
	}
	keys = append(keys, "custom")
	labels = append(labels, "Custom API host")

	idx, err := p.pickIndex("Select provider:", labels, "Manual entry / other")
	if err != nil {
		return nil, err
	}
	if idx == -1 {
		return nil, nil
	}
	d := &modelDraft{}
	var spec providerSpec
	switch {
	case idx == 0:
		prov, err := p.ask("Provider value (e.g. openai, azure)", "")
		if err != nil {
			return nil, err
		}
		prov = strings.TrimSpace(prov)
		if prov == "" {
			return nil, errors.New("provider required")
		}
		host, err := p.ask("API host (optional)", "")
		if err != nil {
			return nil, err
		}
		d.Provider = prov
		d.APIHost = strings.TrimSpace(host)
		spec = providerSpec{key: "manual", provider: prov, auth: "bearer"}
	case idx == len(keys):
		host, err := p.ask("API host (e.g. https://your-host/v1)", "")
		if err != nil {
			return nil, err
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return nil, errors.New("API host required for a custom provider")
		}
		d.Provider = "Openai"
		d.APIHost = host
		spec = providerSpec{key: "custom", provider: "Openai", auth: "bearer"}
	default:
		spec = embeddedProviders[idx-1]
		d.Provider = spec.provider
		d.APIHost = spec.defaultHost
	}
	if spec.key != "ollama" {
		key, err := p.ask("API key", "")
		if err != nil {
			return nil, err
		}
		d.APIKey = strings.TrimSpace(key)
		if d.APIKey != "" {
			fmt.Println("Note: api_key is stored in plaintext in configs/models.yml.")
		}
	}
	models, err := fetchModelList(spec, d.APIHost, d.APIKey)
	if err != nil || len(models) == 0 {
		fmt.Printf("Could not fetch model list: %v\n", err)
		mid, err := p.ask("Model id", "")
		if err != nil {
			return nil, err
		}
		d.ModelName = strings.TrimSpace(mid)
	} else {
		idx, err := p.pickIndex("Select model:", models, "Enter model id manually")
		if err != nil {
			return nil, err
		}
		if idx == -1 {
			return nil, nil
		}
		if idx == 0 {
			mid, err := p.ask("Model id", "")
			if err != nil {
				return nil, err
			}
			d.ModelName = strings.TrimSpace(mid)
		} else {
			d.ModelName = models[idx-1]
		}
	}
	if strings.TrimSpace(d.ModelName) == "" {
		return nil, errors.New("model name required")
	}
	return d, nil
}

// wizardParams collects the generation params, with an optional advanced
// section gated behind a prompt.
func (m *Manager) wizardParams(p *prompt, d *modelDraft) error {
	stream, err := p.askBool("Enable streaming", d.Stream)
	if err != nil {
		return err
	}
	d.Stream = stream

	t := 0.75
	if d.Temperature != nil {
		t = *d.Temperature
	}
	t, err = p.askFloat("Temperature", t)
	if err != nil {
		return err
	}
	d.Temperature = &t

	mt := 4096
	if d.MaxTokens != nil {
		mt = *d.MaxTokens
	}
	mt, err = p.askInt("Max tokens", mt)
	if err != nil {
		return err
	}
	d.MaxTokens = &mt

	tp := 0.95
	if d.TopP != nil {
		tp = *d.TopP
	}
	tp, err = p.askFloat("Top P", tp)
	if err != nil {
		return err
	}
	d.TopP = &tp

	adv, err := p.askBool("Configure advanced params", false)
	if err != nil {
		return err
	}
	if !adv {
		return nil
	}

	mm, err := p.askBool("Multimodal", d.Multimodal)
	if err != nil {
		return err
	}
	d.Multimodal = mm

	osr, err := p.askBool("Online search", d.OnlineSearch)
	if err != nil {
		return err
	}
	d.OnlineSearch = osr

	et := false
	if d.EnableThinking != nil {
		et = *d.EnableThinking
	}
	et, err = p.askBool("Enable thinking", et)
	if err != nil {
		return err
	}
	d.EnableThinking = &et

	tb := 0
	if d.ThinkingBudget != nil {
		tb = *d.ThinkingBudget
	}
	tb, err = p.askInt("Thinking budget", tb)
	if err != nil {
		return err
	}
	if tb != 0 || d.ThinkingBudget != nil {
		d.ThinkingBudget = &tb
	}

	tk := 3.0
	if d.TopK != nil {
		tk = *d.TopK
	}
	tk, err = p.askFloat("Top K", tk)
	if err != nil {
		return err
	}
	if tk != 0 || d.TopK != nil {
		d.TopK = &tk
	}

	eb, err := p.ask("extra_body (JSON, e.g. {\"thinking\":{\"type\":\"enabled\"}})", d.ExtraBody)
	if err != nil {
		return err
	}
	if eb = strings.TrimSpace(eb); eb != "" {
		d.ExtraBody = eb
	}

	for _, cfg := range []struct {
		label string
		ptr   **float64
	}{
		{"Input price (USD/1M tokens)", &d.InputPrice},
		{"Output price (USD/1M tokens)", &d.OutputPrice},
		{"Cached price (USD/1M tokens)", &d.CachedPrice},
	} {
		cur := 0.0
		if *cfg.ptr != nil {
			cur = **cfg.ptr
		}
		v, err := p.askFloat(cfg.label, cur)
		if err != nil {
			return err
		}
		if v != 0 || *cfg.ptr != nil {
			*cfg.ptr = &v
		}
	}
	return nil
}

// wizardModify edits an existing config, pre-filling values and preserving
// unknown/legacy keys via MergeInto.
func (m *Manager) wizardModify(p *prompt, path string, mf ModelFile, name string) error {
	existing := mf[name]
	d := existing.ToDraft()
	d.Name = name
	fmt.Printf("Editing %q (%s / %s).\n", name, d.Provider, d.ModelName)
	chg, err := p.askBool("Change provider or model", false)
	if err != nil {
		return err
	}
	if chg {
		nd, err := m.wizardProviderAndModel(p)
		if err != nil {
			return err
		}
		if nd == nil {
			return nil
		}
		d.Provider, d.ModelName, d.APIKey, d.APIHost = nd.Provider, nd.ModelName, nd.APIKey, nd.APIHost
	}
	cur := maskKey(d.APIKey)
	nk, err := p.ask("API key", cur)
	if err != nil {
		return err
	}
	if nk != cur {
		d.APIKey = nk
	}
	nn, err := p.ask("Config name", d.Name)
	if err != nil {
		return err
	}
	nn = strings.TrimSpace(nn)
	if nn == "" {
		nn = d.Name
	}
	if nn != d.Name {
		if _, exists := mf[nn]; exists {
			return fmt.Errorf("config %q already exists", nn)
		}
		fmt.Printf("Renamed %q to %q.\n", d.Name, nn)
	}
	if err := m.wizardParams(p, &d); err != nil {
		return err
	}
	wasDefault := d.Default
	isDefault, err := p.askBool("Set as default model", wasDefault)
	if err != nil {
		return err
	}
	d.Default = isDefault

	oldName := d.Name
	d.Name = nn
	entry := d.MergeInto(existing)
	if oldName != nn {
		delete(mf, oldName)
	}
	mf[nn] = entry
	if d.Default {
		setDefault(mf, nn)
	} else if wasDefault {
		delete(entry, "default")
	}
	ensureDefault(mf)
	if err := saveModelFile(path, mf); err != nil {
		return err
	}
	fmt.Printf("Saved model config %q.\n", nn)
	return nil
}

// wizardDelete removes a config after a confirmation prompt.
func (m *Manager) wizardDelete(p *prompt, path string, mf ModelFile) error {
	names := modelNames(mf)
	idx, err := p.pickIndex("Select a model to delete:", names, "Cancel")
	if err != nil {
		return err
	}
	if idx == -1 || idx == 0 {
		return nil
	}
	name := names[idx-1]
	fmt.Printf("Type yes to delete %q: ", name)
	ans, err := p.r.ReadString('\n')
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(ans)) != "yes" {
		fmt.Println("cancelled")
		return nil
	}
	wasDefault := mf[name].getBool("default")
	delete(mf, name)
	if len(mf) == 0 {
		if err := os.WriteFile(path, []byte(modelSeedComment), 0o600); err != nil {
			return err
		}
		fmt.Printf("Deleted %q. No model configs remain.\n", name)
		return nil
	}
	newDefault := ensureDefault(mf)
	if err := saveModelFile(path, mf); err != nil {
		return err
	}
	fmt.Printf("Deleted %q.\n", name)
	if wasDefault && newDefault != "" {
		fmt.Printf("Default is now %q.\n", newDefault)
	}
	return nil
}

// wizardSetDefault picks the default config interactively.
func (m *Manager) wizardSetDefault(p *prompt, path string, mf ModelFile) error {
	names := modelNames(mf)
	idx, err := p.pickIndex("Select the default model:", names, "Cancel")
	if err != nil {
		return err
	}
	if idx == -1 || idx == 0 {
		return nil
	}
	setDefault(mf, names[idx-1])
	if err := saveModelFile(path, mf); err != nil {
		return err
	}
	fmt.Printf("Default model is now %q.\n", names[idx-1])
	return nil
}

// prompt wraps interactive stdin helpers. It duplicates the ask/pick style
// of configureCmd (whose closures are function-scoped and left untouched).
type prompt struct {
	r *bufio.Reader
}

func newPrompt() *prompt {
	return &prompt{r: bufio.NewReader(os.Stdin)}
}

// ask prints a label with the current value as default and returns Enter-
// accepted input or the current value on empty input.
func (p *prompt) ask(label, current string) (string, error) {
	fmt.Printf("%s [%s]: ", label, current)
	s, err := p.r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return current, nil
	}
	return s, nil
}

// askBool parses y/n-ish input, defaulting to current.
func (p *prompt) askBool(label string, current bool) (bool, error) {
	cur := "N"
	if current {
		cur = "Y"
	}
	s, err := p.ask(label, cur)
	if err != nil {
		return current, err
	}
	switch strings.ToLower(s) {
	case "y", "yes", "true":
		return true, nil
	case "n", "no", "false":
		return false, nil
	}
	return current, nil
}

// askFloat parses a float, keeping current on unparseable input.
func (p *prompt) askFloat(label string, current float64) (float64, error) {
	s, err := p.ask(label, fmt.Sprintf("%g", current))
	if err != nil {
		return current, err
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return current, nil
	}
	return v, nil
}

// askInt parses an int, keeping current on unparseable input.
func (p *prompt) askInt(label string, current int) (int, error) {
	s, err := p.ask(label, strconv.Itoa(current))
	if err != nil {
		return current, err
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return current, nil
	}
	return v, nil
}

// pickIndex prints a numbered menu plus a zero option and q for back.
// It returns the 1-based menu index, 0 for the zero option, or -1 for back.
func (p *prompt) pickIndex(label string, items []string, zeroLabel string) (int, error) {
	if label != "" {
		fmt.Println(label)
	}
	for i, it := range items {
		fmt.Printf("  %d) %s\n", i+1, it)
	}
	fmt.Printf("  0) %s\n", zeroLabel)
	fmt.Println("  q) quit/back")
	for {
		fmt.Print("> ")
		s, err := p.r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return -1, nil
			}
			return -1, err
		}
		s = strings.TrimSpace(s)
		if s == "q" {
			return -1, nil
		}
		if s == "0" {
			return 0, nil
		}
		n, err := strconv.Atoi(s)
		if err == nil && n >= 1 && n <= len(items) {
			return n, nil
		}
		fmt.Println("Invalid choice.")
	}
}

// maskKey shows a partial API key for edit prompts.
func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "****"
	}
	return k[:3] + "****" + k[len(k)-4:]
}
