package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestRecordInvocationCountsAndObserves(t *testing.T) {
	InvocationsTotal.Reset()
	InvocationLatency.Reset()

	RecordInvocation("acme", "acme", "hello", "ok", 0.2)
	RecordInvocation("acme", "acme", "hello", "ok", 0.4)
	RecordInvocation("acme", "acme", "hello", "error", 0.05)

	if got := testutil.ToFloat64(InvocationsTotal.WithLabelValues("acme", "acme", "hello", "ok")); got != 2 {
		t.Fatalf("ok counter = %v, want 2", got)
	}
	if got := testutil.ToFloat64(InvocationsTotal.WithLabelValues("acme", "acme", "hello", "error")); got != 1 {
		t.Fatalf("error counter = %v, want 1", got)
	}

	count := testutil.CollectAndCount(InvocationLatency)
	if count == 0 {
		t.Fatalf("expected at least one latency series, got 0")
	}
}

func TestRecordRateLimitDropDefaultsAnonymous(t *testing.T) {
	RateLimitDropped.Reset()
	RecordRateLimitDrop("")
	RecordRateLimitDrop("sc_live_abc1")

	if got := testutil.ToFloat64(RateLimitDropped.WithLabelValues("anonymous")); got != 1 {
		t.Fatalf("anonymous counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(RateLimitDropped.WithLabelValues("sc_live_abc1")); got != 1 {
		t.Fatalf("keyed counter = %v, want 1", got)
	}
}

func TestSetSkillsRegisteredOverwrites(t *testing.T) {
	SkillsRegistered.Reset()
	SetSkillsRegistered("acme", "docker", 3)
	SetSkillsRegistered("acme", "docker", 2)
	if got := testutil.ToFloat64(SkillsRegistered.WithLabelValues("acme", "docker")); got != 2 {
		t.Fatalf("gauge = %v, want 2 (gauge should be overwritten, not incremented)", got)
	}
}

func TestRecordHTTPRequestCountsByLabels(t *testing.T) {
	HTTPRequestsTotal.Reset()
	RecordHTTPRequest("GET", "/healthz", "200")
	RecordHTTPRequest("GET", "/healthz", "200")
	RecordHTTPRequest("POST", "/v1/skills/:ns/:name/invoke", "502")

	if got := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("GET", "/healthz", "200")); got != 2 {
		t.Fatalf("healthz 200 counter = %v, want 2", got)
	}
	if got := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("POST", "/v1/skills/:ns/:name/invoke", "502")); got != 1 {
		t.Fatalf("invoke 502 counter = %v, want 1", got)
	}
}

func TestRegistryGathersAllExpectedSeries(t *testing.T) {
	// Touch one series in each metric so they show up in the registry dump.
	RecordInvocation("acme", "acme", "hello", "ok", 0.1)
	RecordRateLimitDrop("sc_live_x")
	SetSkillsRegistered("acme", "docker", 1)
	RecordHTTPRequest("GET", "/healthz", "200")

	mfs, err := Registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	wantNames := []string{
		"skillcloud_invocations_total",
		"skillcloud_invocation_latency_seconds",
		"skillcloud_rate_limit_dropped_total",
		"skillcloud_skills_registered",
		"skillcloud_http_requests_total",
	}
	seen := make(map[string]bool, len(wantNames))
	for _, mf := range mfs {
		seen[mf.GetName()] = true
	}
	for _, n := range wantNames {
		if !seen[n] {
			t.Errorf("metric %q missing from registry; have %s", n, strings.Join(metricNames(mfs), ","))
		}
	}
}

func metricNames(mfs []*dto.MetricFamily) []string {
	out := make([]string, 0, len(mfs))
	for _, mf := range mfs {
		out = append(out, mf.GetName())
	}
	return out
}
