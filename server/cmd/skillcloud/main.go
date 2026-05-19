// Command skillcloud is the entrypoint for the Skill Cloud API server.
//
// Configuration is read from environment variables:
//
//	SKILLCLOUD_LISTEN_ADDR    HTTP listen address (default ":8080")
//	SKILLCLOUD_DB_DSN         Postgres connection string. If empty the
//	                          server starts with the in-memory registry
//	                          and no auth — useful for local development
//	                          but not for production.
//	SKILLCLOUD_TRUST_PROXY    When "true", the server honours
//	                          X-Forwarded-For / X-Real-IP when resolving
//	                          the caller IP. Enable only when sitting
//	                          behind a trusted load balancer.
//	SKILLCLOUD_RATE_LIMIT     Per-API-key rate limit in requests-per-minute
//	                          (integer). Empty / 0 keeps the default 60.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yangjj-iso/skill-cloud/server/internal/api"
	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/db"
	"github.com/yangjj-iso/skill-cloud/server/internal/invocations"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
)

func main() {
	addr := envOr("SKILLCLOUD_LISTEN_ADDR", ":8080")
	dsn := os.Getenv("SKILLCLOUD_DB_DSN")

	var opts api.Options
	opts.TrustProxy = strings.EqualFold(os.Getenv("SKILLCLOUD_TRUST_PROXY"), "true")
	if raw := os.Getenv("SKILLCLOUD_RATE_LIMIT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			opts.RateLimit = api.RateLimitConfig{RequestsPerMinute: n}
		} else {
			log.Printf("warning: invalid SKILLCLOUD_RATE_LIMIT=%q, falling back to default", raw)
		}
	}

	if dsn == "" {
		log.Print("warning: SKILLCLOUD_DB_DSN not set; starting in unauthenticated in-memory mode")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pool, err := db.Connect(ctx, dsn)
		if err != nil {
			log.Fatalf("connect to db: %v", err)
		}
		if err := db.Migrate(ctx, pool); err != nil {
			log.Fatalf("apply migrations: %v", err)
		}
		opts.Registry = registry.NewPostgres(pool)
		opts.Auth = auth.NewService(pool)
		opts.Invocations = invocations.NewPostgres(pool)
	}

	srv := api.NewServer(api.Config{ListenAddr: addr}, opts)
	log.Printf("skill-cloud server listening on %s", addr)
	if err := srv.Run(); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
