package api

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/metrics"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

// httpMetricsMiddleware counts every served request by method, route
// template, and status code. Using c.FullPath() (the route template)
// instead of c.Request.URL.Path keeps the cardinality bounded: a
// thousand distinct `/v1/skills/acme/foo` requests collapse to a single
// series labelled `/v1/skills/:namespace/:name`.
func httpMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		route := c.FullPath()
		if route == "" {
			// 404s and other unmatched routes — record as a single
			// series so a probe storm doesn't blow up the registry.
			route = "unmatched"
		}
		metrics.RecordHTTPRequest(c.Request.Method, route, strconv.Itoa(c.Writer.Status()))
	}
}

// rateLimitKeyPrefix returns the leading characters of the API key (or
// "anonymous" in dev mode) for use as a low-cardinality metric label.
// We deliberately pass only the prefix; the secret half of the token is
// never observed by the metrics package.
func rateLimitKeyPrefix(c *gin.Context) string {
	if p, ok := auth.PrincipalFromContext(c.Request.Context()); ok && p.APIKeyID != uuid.Nil {
		s := p.APIKeyID.String()
		if len(s) > 12 {
			return s[:12]
		}
		return s
	}
	return "anonymous"
}

// orgSlugForMetrics returns the org's slug if the auth service can
// resolve it; otherwise the UUID. The slug is a much friendlier label.
// Errors fall back to the UUID so a Postgres blip doesn't break metric
// emission.
func (s *Server) orgSlugForMetrics(ctx context.Context, orgID uuid.UUID) string {
	if s.auth == nil {
		return orgID.String()
	}
	if slug, err := s.auth.LookupOrgSlug(ctx, orgID); err == nil && slug != "" {
		return slug
	}
	return orgID.String()
}

// refreshSkillsGauge recounts the skills currently in the registry and
// updates the SkillsRegistered gauge. Called on Upsert and at a slow
// background tick so the gauge tracks the registry even when restarts
// or out-of-band writes happen.
func (s *Server) refreshSkillsGauge(ctx context.Context, orgID uuid.UUID) {
	skills, err := s.registry.List(ctx, orgID)
	if err != nil {
		return
	}
	counts := map[string]int{}
	for _, sk := range skills {
		counts[string(sk.Runtime.Type)]++
	}
	org := s.orgSlugForMetrics(ctx, orgID)
	for _, rt := range []models.RuntimeType{models.RuntimeDocker, models.RuntimeHTTPProxy} {
		metrics.SetSkillsRegistered(org, string(rt), float64(counts[string(rt)]))
	}
}

// startMetricsBackgroundRefresh runs refreshSkillsGauge periodically
// so the gauge stays accurate after restarts and on long-lived
// processes where no Upsert has happened recently. The goroutine
// terminates when ctx is cancelled; production passes a background
// context.
func (s *Server) startMetricsBackgroundRefresh(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Without auth we still want to surface dev-mode
				// skills; the dev org id is the only one that exists.
				s.refreshSkillsGauge(ctx, devOrgID)
			}
		}
	}()
}
