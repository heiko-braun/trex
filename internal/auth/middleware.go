// Ported (trimmed) from
// com.sixt.service.managed-agents/internal/auth/middleware.go. No
// UserStore/TenantMap here — this service neither provisions users nor
// resolves a tenant from roles (see specs/workflow-server-auth.md's Out of
// Scope), it only validates the bearer token and rejects on failure.
package auth

import (
	"log/slog"
	"net/http"
	"strings"
)

// MiddlewareConfig holds configuration for the auth middleware.
type MiddlewareConfig struct {
	Validator   *JWKSValidator
	AdminRoles  []string
	ExemptPaths []string // exact paths that skip auth (e.g. "/api/v1/auth/config")
}

// Middleware returns an HTTP middleware that validates Bearer tokens and
// sets the AuthUser in the request context.
func Middleware(cfg MiddlewareConfig) func(http.Handler) http.Handler {
	exemptSet := make(map[string]bool, len(cfg.ExemptPaths))
	for _, p := range cfg.ExemptPaths {
		exemptSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if exemptSet[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			if cfg.Validator == nil {
				slog.Warn("auth middleware has no JWKS validator — rejecting request")
				http.Error(w, `{"error":"authentication not configured"}`, http.StatusUnauthorized)
				return
			}

			claims, err := cfg.Validator.ValidateToken(tokenString)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			username := claims.PreferredUsername
			if username == "" {
				username = claims.Email
			}

			authUser := &AuthUser{
				Sub:        claims.Sub,
				Email:      claims.Email,
				Username:   username,
				Name:       claims.Name,
				IsAdmin:    HasAdminRole(claims.RealmAccess.Roles, cfg.AdminRoles),
				RealmRoles: claims.RealmAccess.Roles,
			}

			ctx := ContextWithUser(r.Context(), authUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
