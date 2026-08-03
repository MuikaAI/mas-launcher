package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// trimSlash removes trailing slashes from a host URL.
func trimSlash(h string) string {
	return strings.TrimRight(h, "/")
}

// openaiModelURLs returns candidate model-list endpoints for an
// OpenAI-compatible host, in order of preference. Hosts already ending in
// /v1 need no fallback; bare roots (e.g. DeepSeek) try /models first.
func openaiModelURLs(host string) []string {
	base := trimSlash(host)
	if strings.HasSuffix(base, "/v1") {
		return []string{base + "/models"}
	}
	return []string{base + "/models", base + "/v1/models"}
}

// fetchModelList retrieves the model list for a provider, trying each
// candidate URL in order. It returns a deduplicated, sorted list of model
// ids, or an error if every candidate failed or returned nothing useful.
func fetchModelList(spec providerSpec, host, apiKey string) ([]string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	var lastErr error
	seen := map[string]bool{}
	var out []string
	for _, u := range spec.listURLs(host) {
		ids, err := fetchOneModelList(client, spec, u, apiKey)
		if err != nil {
			lastErr = err
			continue
		}
		for _, id := range ids {
			if id == "list" || id == "retrieve" {
				continue
			}
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		if len(out) > 0 {
			break
		}
	}
	if len(out) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("empty model list from %s", host)
		}
		return nil, lastErr
	}
	sort.Strings(out)
	return out, nil
}

// fetchOneModelList performs one GET and parses either OpenAI-style
// {"data":[{"id":...}]} or gemini/ollama-style {"models":[{"name":...}]}.
func fetchOneModelList(client *http.Client, spec providerSpec, u, apiKey string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if spec.auth == "bearer" && apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if spec.auth == "query" && apiKey != "" {
		q := req.URL.Query()
		q.Set("key", apiKey)
		req.URL.RawQuery = q.Encode()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", u, err)
	}
	var ids []string
	for _, m := range payload.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	for _, m := range payload.Models {
		name := strings.TrimPrefix(m.Name, "models/")
		if name != "" {
			ids = append(ids, name)
		}
	}
	return ids, nil
}
