package api

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// callerIPKey is the gin.Context key under which the resolved caller IP
// is stored by clientIPMiddleware. Handlers and other middleware read it
// via callerIPFromContext.
const callerIPKey = "skillcloud.caller_ip"

// clientIPMiddleware resolves the caller's IP once per request and
// stores it on the gin context. When trustProxy is true, the left-most
// entry of X-Forwarded-For is used (the original client behind any
// trusted load balancer); otherwise only the direct connection peer is
// trusted, which prevents header-spoofing in direct-exposure deployments.
func clientIPMiddleware(trustProxy bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(callerIPKey, resolveCallerIP(c, trustProxy))
		c.Next()
	}
}

func resolveCallerIP(c *gin.Context, trustProxy bool) string {
	if trustProxy {
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			// Left-most entry is the original client.
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
		if real := c.GetHeader("X-Real-IP"); real != "" {
			return strings.TrimSpace(real)
		}
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}

// callerIPFromContext returns the caller IP previously set by
// clientIPMiddleware, or "" if the middleware did not run.
func callerIPFromContext(c *gin.Context) string {
	if v, ok := c.Get(callerIPKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
