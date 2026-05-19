package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPProxy forwards an invocation to an externally hosted skill. The
// remote endpoint is expected to:
//
//   - accept POST with `Content-Type: application/json`
//   - read the input JSON as the request body
//   - return a JSON object as the response body (status code 2xx)
//
// Non-2xx responses are treated as `error`; a context-deadline exceeded
// is reported as `timeout`.
type HTTPProxy struct {
	client *http.Client
}

// NewHTTPProxy returns a proxy that uses the supplied client. Passing
// nil uses a default client with no global timeout (per-call timeouts
// are enforced via context.WithTimeout in Run).
func NewHTTPProxy(client *http.Client) *HTTPProxy {
	if client == nil {
		client = &http.Client{}
	}
	return &HTTPProxy{client: client}
}

// Run executes the HTTP proxy.
func (p *HTTPProxy) Run(ctx context.Context, req Request) (Result, error) {
	target := req.Skill.Runtime.URL
	if target == "" {
		return Result{Status: StatusError, ErrorMessage: "runtime.url is empty"},
			errors.New("http_proxy: runtime.url is empty")
	}
	if _, err := url.ParseRequestURI(target); err != nil {
		return Result{Status: StatusError, ErrorMessage: fmt.Sprintf("invalid runtime.url: %v", err)},
			fmt.Errorf("http_proxy: invalid runtime.url %q: %w", target, err)
	}

	input := req.Input
	if input == nil {
		input = map[string]any{}
	}
	body, err := json.Marshal(input)
	if err != nil {
		return Result{Status: StatusError, ErrorMessage: err.Error()}, err
	}

	timeout := time.Duration(req.Skill.Runtime.TimeoutSeconds) * time.Second
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return Result{Status: StatusError, ErrorMessage: err.Error()}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "skill-cloud/1.0 (+http-proxy)")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		// Distinguish timeout from other transport errors so the audit
		// log records the right status.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return Result{
				Status:       StatusTimeout,
				ErrorMessage: fmt.Sprintf("proxy timeout after %s", timeout),
			}, nil
		}
		return Result{Status: StatusError, ErrorMessage: err.Error()}, err
	}
	defer resp.Body.Close()

	// Cap the body we accept so a misbehaving upstream can't OOM us.
	rawOutput, err := io.ReadAll(io.LimitReader(resp.Body, MaxOutputBytes+1))
	if err != nil {
		return Result{Status: StatusError, ErrorMessage: err.Error()}, err
	}
	if len(rawOutput) > MaxOutputBytes {
		return Result{
			Status:       StatusError,
			ErrorMessage: fmt.Sprintf("upstream response exceeded %d bytes", MaxOutputBytes),
		}, nil
	}

	if resp.StatusCode >= 400 {
		return Result{
			Status:       StatusError,
			ErrorMessage: fmt.Sprintf("upstream returned HTTP %d: %s", resp.StatusCode, truncate(string(rawOutput), 200)),
			OutputBytes:  len(rawOutput),
		}, nil
	}

	out := map[string]any{}
	if len(rawOutput) > 0 {
		if err := json.Unmarshal(rawOutput, &out); err != nil {
			return Result{
				Status:       StatusError,
				ErrorMessage: fmt.Sprintf("upstream returned non-JSON body: %v", err),
				OutputBytes:  len(rawOutput),
			}, nil
		}
	}

	return Result{
		Status:      StatusOK,
		Output:      out,
		OutputBytes: len(rawOutput),
	}, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
