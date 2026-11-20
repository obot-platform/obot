package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
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

func TestClaudeOrganizationReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		groups   []string
		readOnly bool
		allowed  bool
	}{
		{
			name:     "admin writes",
			groups:   []string{types.GroupAdmin, types.GroupBasic},
			readOnly: false,
			allowed:  true,
		},
		{
			name:     "admin read-only user reads",
			groups:   []string{types.GroupAdminReadonly, types.GroupBasic},
			readOnly: true,
			allowed:  true,
		},
		{
			name:     "admin read-only user writes",
			groups:   []string{types.GroupAdminReadonly, types.GroupBasic},
			readOnly: false,
			allowed:  false,
		},
		{
			name:     "auditor reads",
			groups:   []string{types.GroupAuditor, types.GroupBasic},
			readOnly: true,
			allowed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := api.Context{User: &kuser.DefaultInfo{UID: "1", Groups: tt.groups}}
			require.Equal(t, tt.allowed, canUseClaudeOrganization(request, tt.readOnly))
		})
	}
}
