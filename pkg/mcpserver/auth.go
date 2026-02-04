package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

var log = logger.Package()

// mcpUserInfo wraps a gateway User to implement k8s user.Info interface.
// This allows ACR checks to work with MCP token authenticated users.
type mcpUserInfo struct {
	user          *types.User
	authGroupIDs  []string
	effectiveRole types2.Role
}

func (u *mcpUserInfo) GetName() string {
	return u.user.Username
}

func (u *mcpUserInfo) GetUID() string {
	return fmt.Sprintf("%d", u.user.ID)
}

func (u *mcpUserInfo) GetGroups() []string {
	return u.effectiveRole.Groups()
}

func (u *mcpUserInfo) GetExtra() map[string][]string {
	return map[string][]string{
		"auth_provider_groups": u.authGroupIDs,
	}
}

// contextKey is used for storing values in context
type contextKey string

const (
	userContextKey          contextKey = "mcpserver-user"
	groupIDsContextKey      contextKey = "mcpserver-group-ids"
	effectiveRoleContextKey contextKey = "mcpserver-effective-role"
)

// userFromContext returns the user from the context.
func userFromContext(ctx context.Context) *types.User {
	user, _ := ctx.Value(userContextKey).(*types.User)
	return user
}

// groupIDsFromContext returns the auth provider group IDs from the context.
func groupIDsFromContext(ctx context.Context) []string {
	groupIDs, _ := ctx.Value(groupIDsContextKey).([]string)
	return groupIDs
}

// effectiveRoleFromContext returns the effective role from the context.
func effectiveRoleFromContext(ctx context.Context) types2.Role {
	role, _ := ctx.Value(effectiveRoleContextKey).(types2.Role)
	return role
}

// authMiddleware validates MCP tokens and adds user info to the request context.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Check for Bearer token
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok {
			http.Error(w, "invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		// Check for MCP token prefix
		if !strings.HasPrefix(token, "mt1-") {
			http.Error(w, "invalid token type", http.StatusUnauthorized)
			return
		}

		// Validate the MCP token
		mcpToken, err := s.gatewayClient.ValidateObotMCPToken(r.Context(), token)
		if err != nil {
			log.Debugf("MCP token validation failed: %v", err)
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Get user info
		user, err := s.gatewayClient.UserByID(r.Context(), strconv.FormatUint(uint64(mcpToken.UserID), 10))
		if err != nil {
			log.Errorf("Failed to get user for MCP token: %v", err)
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Get user's group memberships from the database
		groupIDs, err := s.gatewayClient.ListGroupIDsForUser(r.Context(), user.ID)
		if err != nil {
			log.Warnf("Failed to get groups for user %d: %v", user.ID, err)
			// Continue without groups - don't fail auth
			groupIDs = nil
		}

		// Resolve effective role by merging individual + group roles
		effectiveRole, err := s.gatewayClient.ResolveUserEffectiveRole(r.Context(), user, groupIDs)
		if err != nil {
			log.Warnf("Failed to resolve effective role for user %d: %v", user.ID, err)
			effectiveRole = user.Role
		}

		// Store user, token, groups, and effective role in context
		ctx := context.WithValue(r.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, groupIDsContextKey, groupIDs)
		ctx = context.WithValue(ctx, effectiveRoleContextKey, effectiveRole)

		// Call the next handler with the enriched context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
