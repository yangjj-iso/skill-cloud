// Skill Cloud server entrypoint.
package main

import (
	"log"
	"os"

	"github.com/yangjj-iso/skill-cloud/server/internal/api"
)

func main() {
	addr := os.Getenv("SKILLCLOUD_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := api.NewServer(api.Config{
		ListenAddr: addr,
	})

	log.Printf("skill-cloud server listening on %s", addr)
	if err := srv.Run(); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
