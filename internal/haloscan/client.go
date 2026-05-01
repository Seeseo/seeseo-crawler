// Package haloscan is a minimal HTTP client for the Haloscan API
// (https://api.haloscan.com/api).
//
// The API key is read from the `HALOSCAN_API_KEY` env var, or falls back to
// the value stored in `~/Library/Application Support/Claude/claude_desktop_config.json`
// under `mcpServers.haloscan.env.HALOSCAN_API_KEY` (same place where the
// haloscan-lucky MCP keeps it). Cloudflare blocks the default Go user-agent,
// so we force a custom one.
package haloscan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	BaseURL          = "https://api.haloscan.com/api"
	defaultUserAgent = "SeeseoCrawler-Haloscan/1.0"
	defaultTimeout   = 60 * time.Second
)

type Client struct {
	apiKey    string
	baseURL   string
	userAgent string
	http      *http.Client
}

func NewClient(apiKey, appVersion string) *Client {
	ua := defaultUserAgent
	if appVersion != "" {
		ua = fmt.Sprintf("SeeseoCrawler-Haloscan/%s", appVersion)
	}
	return &Client{
		apiKey:    apiKey,
		baseURL:   BaseURL,
		userAgent: ua,
		http:      &http.Client{Timeout: defaultTimeout},
	}
}

// CallMeta captures metadata for logging into ClickHouse.
type CallMeta struct {
	Endpoint     string
	Method       string
	StatusCode   uint16
	DurationMs   uint32
	ResponseBody string // truncated
}

const maxResponseBodyLog = 10 * 1024

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, *CallMeta, error) {
	meta := &CallMeta{Endpoint: path, Method: method}
	start := time.Now()

	u := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, meta, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("haloscan-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	meta.DurationMs = uint32(time.Since(start).Milliseconds())
	if err != nil {
		return nil, meta, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	meta.StatusCode = uint16(resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, meta, fmt.Errorf("reading response: %w", err)
	}
	if len(data) <= maxResponseBodyLog {
		meta.ResponseBody = string(data)
	} else {
		meta.ResponseBody = string(data[:maxResponseBodyLog])
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, meta, fmt.Errorf("haloscan unauthorized (HTTP %d): %s", resp.StatusCode, truncate(string(data), 300))
	}
	// Haloscan returns HTTP 201 on successful POSTs (e.g. /domains/overview)
	// in addition to 200 — accept any 2xx as success.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, meta, fmt.Errorf("haloscan %s HTTP %d: %s", path, resp.StatusCode, truncate(string(data), 300))
	}
	return data, meta, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload interface{}) ([]byte, *CallMeta, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling payload: %w", err)
	}
	return c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(b))
}

func (c *Client) getJSON(ctx context.Context, path string, params map[string]string) ([]byte, *CallMeta, error) {
	if len(params) > 0 {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		path = path + "?" + q.Encode()
	}
	return c.doRequest(ctx, http.MethodGet, path, nil)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ============================================================================
// API key auto-discovery
// ============================================================================

// LoadAPIKeyFromClaudeConfig reads the Haloscan API key from the Claude
// Desktop config file (used by the haloscan-lucky MCP server). Returns ""
// if the file or the key is missing.
func LoadAPIKeyFromClaudeConfig() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	cfgPath := filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}
	var cfg struct {
		McpServers map[string]struct {
			Env map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	// Try a few candidate names: the MCP may be registered under "haloscan"
	// or "haloscan-lucky" depending on the user's setup.
	for _, name := range []string{"haloscan", "haloscan-lucky", "haloscan-server"} {
		if srv, ok := cfg.McpServers[name]; ok {
			if k := srv.Env["HALOSCAN_API_KEY"]; k != "" {
				return k
			}
		}
	}
	return ""
}

// ResolveAPIKey returns the first available API key from env var,
// then Claude Desktop config. Empty string if none found.
func ResolveAPIKey() string {
	if k := os.Getenv("HALOSCAN_API_KEY"); k != "" {
		return k
	}
	return LoadAPIKeyFromClaudeConfig()
}
