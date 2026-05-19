// Command skillcloud is the entrypoint for the Skill Cloud API server.
//
// Configuration is read from environment variables:
//
//	SKILLCLOUD_LISTEN_ADDR  HTTP listen address (default ":8080")
//	SKILLCLOUD_DB_DSN       Postgres connection string. If empty the
//	                        server starts with the in-memory registry
//	                        and no auth — useful for local development
//	                        but not for production.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/yangjj-iso/skill-cloud/server/internal/api"
	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/db"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
)

func main() {
	addr := envOr("SKILLCLOUD_LISTEN_ADDR", ":8080")
	dsn := os.Getenv("SKILLCLOUD_DB_DSN")

	var opts api.Options
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
