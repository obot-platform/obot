package authz

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// IsClaudeEnvironment reports whether the authenticated caller holds a server-issued Claude runner credential.
// Use authenticated groups only, never client-supplied headers or MCP client names.
func IsClaudeEnvironment(u kuser.Info) bool {
	return u != nil && slices.Contains(u.GetGroups(), types.GroupClaudeEnvironment)
}
