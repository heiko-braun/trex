// Ported (trimmed) from
// com.sixt.service.managed-agents/internal/auth/claims.go. This service has
// no user/tenant store, so AuthUser and ContextWithUser/UserFromContext are
// intentionally smaller than the source: no ID (post-upsert local user id),
// no TenantID.
package auth

import (
	"context"
	"strings"
)

type contextKey int

const authUserKey contextKey = iota

// AuthUser represents the authenticated caller extracted from a validated
// JWT, with no persistence — this service does not provision users.
type AuthUser struct {
	Sub        string // Keycloak subject
	Email      string
	Username   string
	Name       string
	IsAdmin    bool
	RealmRoles []string
}

// ContextWithUser returns a new context carrying the authenticated user.
func ContextWithUser(ctx context.Context, user *AuthUser) context.Context {
	return context.WithValue(ctx, authUserKey, user)
}

// UserFromContext extracts the authenticated user from context.
// Returns nil if no user is set (unauthenticated).
func UserFromContext(ctx context.Context) *AuthUser {
	u, _ := ctx.Value(authUserKey).(*AuthUser)
	return u
}

// KeycloakClaims represents the JWT claims payload from Keycloak.
type KeycloakClaims struct {
	Sub               string      `json:"sub"`
	Email             string      `json:"email"`
	PreferredUsername string      `json:"preferred_username"`
	Name              string      `json:"name"`
	Issuer            string      `json:"iss"`
	ExpiresAt         int64       `json:"exp"`
	IssuedAt          int64       `json:"iat"`
	RealmAccess       RealmAccess `json:"realm_access"`
}

// RealmAccess holds the realm-level roles from the JWT.
type RealmAccess struct {
	Roles []string `json:"roles"`
}

// HasAdminRole checks if any of the user's realm roles match the configured admin roles.
func HasAdminRole(userRoles []string, adminRoles []string) bool {
	adminSet := make(map[string]bool, len(adminRoles))
	for _, r := range adminRoles {
		trimmed := strings.TrimSpace(r)
		if trimmed != "" {
			adminSet[trimmed] = true
		}
	}
	for _, role := range userRoles {
		if adminSet[role] {
			return true
		}
	}
	return false
}

// ParseAdminRoles splits a comma-separated admin roles string into a slice.
func ParseAdminRoles(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			roles = append(roles, trimmed)
		}
	}
	return roles
}
