// Package client wraps the Skill Cloud REST API for the CLI. It mirrors
// the SDK shape but is internal to the CLI binary so we can keep
// command-line-specific concerns (e.g. user-friendly error messages,
// printing skill manifests) out of the public SDKs.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a single Skill Cloud server.
type Client struct {
	host   string
	apiKey string
	http   *http.Client
}

// New constructs a client. `host` must include the scheme (e.g.
// "http://localhost:8080"). A trailing slash is trimmed for callers.
// If httpClient is nil, a default client with a 30s timeout is used.
func New(host, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		host:   strings.TrimRight(host, "/"),
		apiKey: apiKey,
		http:   httpClient,
	}
}

// Skill is a thin projection of the server's manifest shape. The CLI
// doesn't need every field — we only decode what we print.
type Skill struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Runtime     struct {
		Type           string `json:"type"`
		TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
		MemoryMB       int    `json:"memory_mb,omitempty"`
	} `json:"runtime"`
}

// InvokeResult is the JSON returned from `/v1/skills/.../invoke`.
type InvokeResult struct {
	Skill  string         `json:"skill"`
	Status string         `json:"status"`
	Output map[string]any `json:"output"`
	Error  string         `json:"error,omitempty"`
}

// Stats matches the server's stats payload. All fields may be empty
// when a skill has never been invoked.
type Stats struct {
	Total          int    `json:"total"`
	Last24h        int    `json:"last_24h"`
	LastInvokedAt  string `json:"last_invoked_at,omitempty"`
	LastCallerIP   string `json:"last_caller_ip,omitempty"`
	LastStatusCode string `json:"last_status,omitempty"`
}

// LogEntry is one row from the invocation log.
type LogEntry struct {
	Status       string `json:"status"`
	LatencyMS    int    `json:"latency_ms"`
	InputBytes   int    `json:"input_bytes"`
	OutputBytes  int    `json:"output_bytes"`
	StartedAt    string `json:"started_at"`
	CallerIP     string `json:"caller_ip,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// PublishSkill registers (or updates) a skill manifest. The server
// accepts the raw manifest body and returns the stored representation.
func (c *Client) PublishSkill(ctx context.Context, manifest map[string]any) (Skill, error) {
	body, err := json.Marshal(manifest)
	if err != nil {
		return Skill{}, fmt.Errorf("marshal manifest: %w", err)
	}
	var out Skill
	if err := c.do(ctx, http.MethodPost, "/v1/skills", body, &out); err != nil {
		return Skill{}, err
	}
	return out, nil
}

// ListSkills returns every skill visible to the caller's org. Returns
// an empty slice (never nil) when nothing is registered.
func (c *Client) ListSkills(ctx context.Context) ([]Skill, error) {
	var raw struct {
		Skills []Skill `json:"skills"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/skills", nil, &raw); err != nil {
		return nil, err
	}
	if raw.Skills == nil {
		raw.Skills = []Skill{}
	}
	return raw.Skills, nil
}

// Invoke calls `/v1/skills/<ns>/<name>/invoke` with the supplied input.
// 200/502/504 all parse the body into InvokeResult; other statuses
// surface as RemoteError.
func (c *Client) Invoke(ctx context.Context, namespace, name string, input map[string]any) (InvokeResult, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("marshal input: %w", err)
	}
	var out InvokeResult
	if err := c.do(ctx, http.MethodPost, "/v1/skills/"+namespace+"/"+name+"/invoke", body, &out); err != nil {
		return InvokeResult{}, err
	}
	return out, nil
}

// GetStats fetches the per-skill stats payload.
func (c *Client) GetStats(ctx context.Context, namespace, name string) (Stats, error) {
	var s Stats
	if err := c.do(ctx, http.MethodGet, "/v1/skills/"+namespace+"/"+name+"/stats", nil, &s); err != nil {
		return Stats{}, err
	}
	return s, nil
}

// ListLogs returns the most recent invocation rows. The server caps
// the returned page server-side; the CLI doesn't paginate yet.
func (c *Client) ListLogs(ctx context.Context, namespace, name string) ([]LogEntry, error) {
	var raw struct {
		Invocations []LogEntry `json:"invocations"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/skills/"+namespace+"/"+name+"/logs", nil, &raw); err != nil {
		return nil, err
	}
	if raw.Invocations == nil {
		raw.Invocations = []LogEntry{}
	}
	return raw.Invocations, nil
}

// Healthz hits /healthz so login can confirm the host is reachable
// before saving credentials.
func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// RemoteError is returned when the server replies with a non-2xx
// response. The body is captured (truncated) so callers can render a
// useful message.
type RemoteError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *RemoteError) Error() string {
	body := e.Body
	if body == "" {
		body = "<empty body>"
	}
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, body)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	url := c.host + path
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// The invoke handler returns 502/504 with a body that conforms to
	// InvokeResult; we decode them on the success path so the CLI can
	// print the error message directly. Treat any other non-2xx as a
	// hard error.
	is2xx := resp.StatusCode >= 200 && resp.StatusCode < 300
	isInvokeFailure := strings.Contains(path, "/invoke") && (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout)
	if !is2xx && !isInvokeFailure {
		return &RemoteError{Status: resp.StatusCode, Method: method, Path: path, Body: truncate(string(raw), 500)}
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s %s: decode response: %w", method, path, err)
		}
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// IsAuthError reports whether the error is a 401/403 RemoteError.
func IsAuthError(err error) bool {
	var re *RemoteError
	if !errors.As(err, &re) {
		return false
	}
	return re.Status == http.StatusUnauthorized || re.Status == http.StatusForbidden
}
