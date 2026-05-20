package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type principalKey struct{}

// Authenticator resolves an opaque token into a Principal. It is the only
// dependency the middleware has on the auth service, which makes it easy to
// stub in tests.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Principal, error)
}

// Middleware returns a gin middleware that enforces `Authorization: Bearer <token>`
// on every request and injects the resolved Principal into the request
// context. If the request is unauthenticated, it returns 401 and aborts.
func Middleware(a Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		p, err := a.Authenticate(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, ErrInvalidKey) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		ctx := context.WithValue(c.Request.Context(), principalKey{}, p)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// PrincipalFromContext returns the Principal stored by Middleware, or
// (zero, false) if none is present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// InjectPrincipal returns a context with the given Principal attached.
// Useful for tests that need to call handlers without going through the
// middleware.
func InjectPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalForOrg is a convenience for tests.
func PrincipalForOrg(orgID uuid.UUID) Principal {
	return Principal{OrgID: orgID, UserID: uuid.New(), APIKeyID: uuid.New()}
}
