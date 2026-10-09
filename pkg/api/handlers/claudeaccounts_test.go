package handlers

import (
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestClaudeMappingsRequireAdmin(t *testing.T) {
	handler := &ClaudeRunnerHandler{}
	request := api.Context{
		User: &kuser.DefaultInfo{
			UID:    "1",
			Groups: []string{"basic"},
		},
	}
	// Both reads and writes reject non-admins before accessing the database.
	require.ErrorContains(t, handler.SaveClaudeTag(request), "Administrator access required")
	require.ErrorContains(t, handler.OrganizationAccounts(request), "Administrator access required")
	require.ErrorContains(t, handler.SaveOrganizationAccounts(request), "Administrator access required")
}
