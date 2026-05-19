// Package api wires up the HTTP server and routes.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/invocations"
	"github.com/yangjj-iso/skill-cloud/server/internal/mcp"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
	"github.com/yangjj-iso/skill-cloud/server/internal/runtime"
)

// devOrgID is the well-known org used when the server runs without a real
// auth service (SKILLCLOUD_DB_DSN unset). All requests are scoped to this
// org so the in-memory registry behaves like a single-tenant local sandbox.
// Production deployments MUST set a DSN and run the real auth middleware.
var devOrgID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// Config holds runtime configuration for the API server.
type Config struct {
	ListenAddr string
}

// Server is the HTTP API server.
type Server struct {
	cfg         Config
	engine      *gin.Engine
	registry    registry.Registry
	auth        *auth.Service
	invocations invocations.Store
	dispatcher  *runtime.Dispatcher
	rateLimit   RateLimitConfig
	trustProxy  bool
}

// Options configures non-default server dependencies. All fields are
// optional: if Registry is nil the server falls back to an in-memory
// registry (used by unit tests and ephemeral dev mode); if Auth is nil
// the server runs without bearer-token enforcement and unit tests must
// inject a principal directly via `auth.InjectPrincipal`; if Invocations
// is nil an in-memory store is used.
type Options struct {
	Registry    registry.Registry
	Auth        *auth.Service
	Invocations invocations.Store
	// Dispatcher executes skill invocations. When nil, the server
	// constructs a dispatcher that knows about the HTTP-proxy runtime
	// only; docker support is opt-in (main.go installs it when the
	// docker binary is available).
	Dispatcher *runtime.Dispatcher
	RateLimit  RateLimitConfig
	// TrustProxy controls whether X-Forwarded-For / X-Real-IP are honoured
	// when resolving the caller IP. Enable only when the server sits
	// behind a trusted load balancer.
	TrustProxy bool
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
	inv := opts.Invocations
	if inv == nil {
		inv = invocations.NewMemory()
	}
	disp := opts.Dispatcher
	if disp == nil {
		disp = runtime.NewDispatcher(nil, runtime.NewHTTPProxy(nil))
	}
	rl := opts.RateLimit
	if rl.RequestsPerMinute == 0 {
		rl = DefaultRateLimit
	}

	s := &Server{
		cfg:         cfg,
		engine:      engine,
		registry:    reg,
		auth:        opts.Auth,
		invocations: inv,
		dispatcher:  disp,
		rateLimit:   rl,
		trustProxy:  opts.TrustProxy,
	}
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
	s.engine.Use(clientIPMiddleware(s.trustProxy))

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

	authMW := s.principalMiddleware()
	rlMW := rateLimitMiddleware(s.rateLimit)

	v1 := s.engine.Group("/v1")
	v1.Use(authMW, rlMW)
	{
		v1.GET("/skills", s.listSkills)
		v1.GET("/skills/:namespace/:name", s.getSkill)
		v1.GET("/skills/:namespace/:name/runtime", s.getSkillRuntime)
		v1.GET("/skills/:namespace/:name/stats", s.getSkillStats)
		v1.GET("/skills/:namespace/:name/logs", s.listSkillLogs)
		v1.POST("/skills", s.createSkill)
		v1.POST("/skills/:namespace/:name/invoke", s.invokeSkill)
	}

	// MCP endpoint exposes registered skills as MCP tools so any
	// MCP-capable client can use them with zero modification. It accepts
	// the same Bearer token as /v1 (or, in no-DB dev mode, falls back to
	// the same anonymous principal). Runtime details are stripped from
	// tools/list responses (see internal/mcp/server.go).
	mcpGroup := s.engine.Group("/mcp")
	mcpGroup.Use(authMW, rlMW)
	mcpGroup.POST("", mcp.Handler(s.registry, mcp.Options{
		Invocations: s.invocations,
		CallerIP:    callerIPFromContext,
		Dispatcher:  s.dispatcher,
	}))
}

// principalMiddleware returns the middleware that injects a Principal into
// every authenticated request. When a real auth service is configured it
// delegates to the Bearer-token middleware; otherwise it injects a
// well-known anonymous principal so the in-memory dev mode is usable
// without authentication.
func (s *Server) principalMiddleware() gin.HandlerFunc {
	if s.auth != nil {
		return auth.Middleware(s.auth)
	}
	anonymous := auth.Principal{OrgID: devOrgID}
	return func(c *gin.Context) {
		// Honour a principal that's already been injected (unit tests
		// pre-populate one via auth.InjectPrincipal); otherwise fall
		// back to the anonymous dev principal.
		if _, ok := auth.PrincipalFromContext(c.Request.Context()); !ok {
			ctx := auth.InjectPrincipal(c.Request.Context(), anonymous)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
