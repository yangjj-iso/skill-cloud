// Package metrics registers the platform's Prometheus collectors and exposes
// the helpers that the API and runtime layers call to record activity.
//
// Cardinality discipline:
//   - org, namespace and name labels are bounded by the registry; the registry
//     itself caps namespace/name shape via regex.
//   - status is a small enumerated set (ok / error / timeout / 4xx /5xx).
//   - runtime_type is one of {docker, http_proxy}.
//   - api_key_prefix is a 12-character prefix so a compromised key can be
//     surfaced in dashboards without leaking the secret half.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry is the application-wide Prometheus registry. Tests can swap it for
// an isolated instance, and the /metrics handler scrapes from it directly.
var Registry = prometheus.NewRegistry()

var factory = promauto.With(Registry)

var (
	// InvocationsTotal counts every dispatcher attempt, partitioned by
	// outcome. The status label is one of: ok, error, timeout.
	InvocationsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "skillcloud_invocations_total",
		Help: "Total number of skill invocations dispatched, by outcome.",
	}, []string{"org", "namespace", "name", "status"})

	// InvocationLatency measures the wall-clock latency of a dispatcher
	// call (including container startup or upstream HTTP). Buckets are
	// chosen to span the realistic range of skill runtimes (a few ms for
	// http_proxy mocks to a few minutes for slow LLM-backed skills).
	InvocationLatency = factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "skillcloud_invocation_latency_seconds",
		Help:    "Latency of skill invocations in seconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
	}, []string{"org", "namespace", "name"})

	// RateLimitDropped tracks 429 responses. Useful both for spotting
	// abusive callers and for tuning SKILLCLOUD_RATE_LIMIT for legit ones.
	RateLimitDropped = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "skillcloud_rate_limit_dropped_total",
		Help: "Total number of requests rejected by the per-key rate limiter.",
	}, []string{"api_key_prefix"})

	// SkillsRegistered exposes the population of the registry per org and
	// runtime type. Updated by the registry on every Upsert.
	SkillsRegistered = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "skillcloud_skills_registered",
		Help: "Number of skill versions registered, by org and runtime type.",
	}, []string{"org", "runtime_type"})

	// HTTPRequestsTotal is a low-cardinality view of the API surface; it
	// counts requests by method + route + status_code so an operator can
	// see error spikes without joining against the per-skill series.
	HTTPRequestsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "skillcloud_http_requests_total",
		Help: "Total number of HTTP requests handled by the API server.",
	}, []string{"method", "route", "status_code"})
)

// RecordInvocation increments both the counter and the histogram for one
// dispatcher result. It is safe to call from any goroutine.
func RecordInvocation(org, namespace, name, status string, latencySeconds float64) {
	InvocationsTotal.WithLabelValues(org, namespace, name, status).Inc()
	InvocationLatency.WithLabelValues(org, namespace, name).Observe(latencySeconds)
}

// RecordRateLimitDrop increments the 429 counter for the given API-key
// prefix. The caller is expected to pass the already-truncated prefix so this
// package never sees the secret half of a token.
func RecordRateLimitDrop(prefix string) {
	if prefix == "" {
		prefix = "anonymous"
	}
	RateLimitDropped.WithLabelValues(prefix).Inc()
}

// SetSkillsRegistered overwrites the gauge with the current count for the
// given org/runtime pair. The registry recomputes this on Upsert.
func SetSkillsRegistered(org, runtimeType string, count float64) {
	SkillsRegistered.WithLabelValues(org, runtimeType).Set(count)
}

// RecordHTTPRequest increments the request counter for an API call.
func RecordHTTPRequest(method, route, statusCode string) {
	HTTPRequestsTotal.WithLabelValues(method, route, statusCode).Inc()
}
