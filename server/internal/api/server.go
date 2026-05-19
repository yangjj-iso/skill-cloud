// Package api wires up the HTTP server and routes.
package api

import (
	"github.com/gin-gonic/gin"

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
	registry *registry.InMemory
}

// NewServer constructs a Server with the given configuration.
func NewServer(cfg Config) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	reg := registry.NewInMemory()
	s := &Server{cfg: cfg, engine: engine, registry: reg}
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

	v1 := s.engine.Group("/v1")
	{
		v1.GET("/skills", s.listSkills)
		v1.GET("/skills/:namespace/:name", s.getSkill)
		v1.POST("/skills", s.createSkill)
		v1.POST("/skills/:namespace/:name/invoke", s.invokeSkill)
	}

	// MCP endpoint exposes registered skills as MCP tools so any
	// MCP-capable client can use them with zero modification.
	s.engine.POST("/mcp", mcp.Handler(s.registry))
}
