package runtime_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/runtime"
)

func httpProxyManifest(target string, timeoutSeconds int) models.SkillManifest {
	return models.SkillManifest{
		Namespace: "acme",
		Name:      "proxy",
		Version:   "0.1.0",
		Runtime: models.Runtime{
			Type:           models.RuntimeHTTPProxy,
			URL:            target,
			TimeoutSeconds: timeoutSeconds,
		},
	}
}

func TestHTTPProxyHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		require.NoError(t, json.Unmarshal(body, &in))
		assert.Equal(t, "world", in["name"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"greeting":"hello world"}`))
	}))
	defer srv.Close()

	proxy := runtime.NewHTTPProxy(nil)
	res, err := proxy.Run(context.Background(), runtime.Request{
		Skill: httpProxyManifest(srv.URL, 5),
		Input: map[string]any{"name": "world"},
	})
	require.NoError(t, err)
	assert.Equal(t, runtime.StatusOK, res.Status)
	assert.Equal(t, "hello world", res.Output["greeting"])
	assert.Positive(t, res.OutputBytes)
}

func TestHTTPProxyNon2xxBecomesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"upstream blew up"}`))
	}))
	defer srv.Close()

	proxy := runtime.NewHTTPProxy(nil)
	res, err := proxy.Run(context.Background(), runtime.Request{
		Skill: httpProxyManifest(srv.URL, 5),
	})
	require.NoError(t, err)
	assert.Equal(t, runtime.StatusError, res.Status)
	assert.Contains(t, res.ErrorMessage, "HTTP 500")
}

func TestHTTPProxyTimeoutsAreClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 1s manifest timeout, but the dispatcher does NOT apply defaults
	// when we hit the runner directly; pass a 1s timeout explicitly.
	skill := httpProxyManifest(srv.URL, 1)
	// Drop the manifest timeout to a value smaller than the server's
	// sleep so the proxy times out before the response arrives.
	skill.Runtime.TimeoutSeconds = 1
	proxy := runtime.NewHTTPProxy(&http.Client{Timeout: 50 * time.Millisecond})

	res, err := proxy.Run(context.Background(), runtime.Request{Skill: skill})
	require.NoError(t, err)
	// The http.Client's own timeout fires here — it surfaces as a
	// transport error rather than context-cancel. Either is acceptable;
	// we just need the runner to report a non-ok status with a clear
	// message.
	assert.NotEqual(t, runtime.StatusOK, res.Status)
	assert.NotEmpty(t, res.ErrorMessage)
}

func TestHTTPProxyContextTimeoutReportsTimeout(t *testing.T) {
	// Use a server that holds the connection longer than the manifest
	// timeout. The runner uses context.WithTimeout(manifest.TimeoutSeconds)
	// so context.DeadlineExceeded fires; the runner returns
	// StatusTimeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	skill := httpProxyManifest(srv.URL, 1) // 1s timeout
	proxy := runtime.NewHTTPProxy(nil)     // no transport-level timeout

	start := time.Now()
	res, err := proxy.Run(context.Background(), runtime.Request{Skill: skill})
	require.NoError(t, err)
	assert.Equal(t, runtime.StatusTimeout, res.Status)
	assert.Less(t, time.Since(start), 2*time.Second, "proxy must abort before the upstream responds")
}

func TestHTTPProxyOversizeResponse(t *testing.T) {
	// MaxOutputBytes + a few extra bytes — the runner should reject.
	huge := strings.Repeat("a", runtime.MaxOutputBytes+10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"blob":"`+huge+`"}`)
	}))
	defer srv.Close()

	proxy := runtime.NewHTTPProxy(nil)
	res, err := proxy.Run(context.Background(), runtime.Request{Skill: httpProxyManifest(srv.URL, 5)})
	require.NoError(t, err)
	assert.Equal(t, runtime.StatusError, res.Status)
	assert.Contains(t, res.ErrorMessage, "exceeded")
}

func TestHTTPProxyBadJSONBecomesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	proxy := runtime.NewHTTPProxy(nil)
	res, err := proxy.Run(context.Background(), runtime.Request{Skill: httpProxyManifest(srv.URL, 5)})
	require.NoError(t, err)
	assert.Equal(t, runtime.StatusError, res.Status)
	assert.Contains(t, res.ErrorMessage, "non-JSON")
}

func TestHTTPProxyRejectsEmptyURL(t *testing.T) {
	proxy := runtime.NewHTTPProxy(nil)
	res, err := proxy.Run(context.Background(), runtime.Request{Skill: httpProxyManifest("", 5)})
	require.Error(t, err)
	assert.Equal(t, runtime.StatusError, res.Status)
}
