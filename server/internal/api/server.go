// Package api wires up the HTTP server and routes.
package api

import (
	"github.com/gin-gonic/gin"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/mcp"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
)

// Config holds runtime configuration for the API server.
type Config struct {
	ListenAddr string
}

// Server is the HTTP API server.
type Server struct {
	cfg      Config
	engine   *gin.Engine
	registry registry.Registry
	auth     *auth.Service
}

// Options configures non-default server dependencies. Both fields are
// optional: if Registry is nil the server falls back to an in-memory
// registry (used by unit tests and ephemeral dev mode); if Auth is nil
// the server runs without bearer-token enforcement and unit tests must
// inject a principal directly via `auth.InjectPrincipal`.
type Options struct {
	Registry registry.Registry
	Auth     *auth.Service
}

// NewServer constructs a Server with the given configuration. See Options
// for how to wire in production dependencies.
func NewServer(cfg Config, opts Options) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	reg := opts.Registry
	if reg == nil {
		reg = registry.NewInMemory()
	}

	s := &Server{cfg: cfg, engine: engine, registry: reg, auth: opts.Auth}
	s.routes()
	return s
}

// Handler exposes the underlying http.Handler for testing.
func (s *Server) Handler() *gin.Engine {
	return s.engine
}

// Run starts the HTTP server and blocks until it exits.
func (s *Server) Run() error {
	return s.engine.Run(s.cfg.ListenAddr)
}

func (s *Server) routes() {
	s.engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Bootstrap auth endpoints. These are intentionally unauthenticated
	// in the MVP so an operator can create the first org/user/api_key.
	// They will be moved behind an admin-only guard in a follow-up.
	if s.auth != nil {
		boot := s.engine.Group("/v1/auth")
		{
			boot.POST("/orgs", s.createOrg)
			boot.POST("/users", s.createUser)
			boot.POST("/api_keys", s.createAPIKey)
		}
	}

	v1 := s.engine.Group("/v1")
	if s.auth != nil {
		v1.Use(auth.Middleware(s.auth))
	}
	{
		v1.GET("/skills", s.listSkills)
		v1.GET("/skills/:namespace/:name", s.getSkill)
		v1.POST("/skills", s.createSkill)
		v1.POST("/skills/:namespace/:name/invoke", s.invokeSkill)
	}

	// MCP endpoint exposes registered skills as MCP tools so any
	// MCP-capable client can use them with zero modification. It accepts
	// the same Bearer token as /v1.
	mcpGroup := s.engine.Group("/mcp")
	if s.auth != nil {
		mcpGroup.Use(auth.Middleware(s.auth))
	}
	mcpGroup.POST("", mcp.Handler(s.registry))
}
