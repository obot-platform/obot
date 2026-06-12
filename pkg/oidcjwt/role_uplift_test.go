package oidcjwt

import (
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestRoleUpliftAddsRequestTimeAdminWithoutOwner(t *testing.T) {
	wrapped := NewRoleUplift(staticAuthenticator{
		response: &authenticator.Response{
			User: &user.DefaultInfo{
				Name:   "alice",
				UID:    "42",
				Groups: types.RoleBasic.Groups(),
				Extra: map[string][]string{
					jwtRolesExtraKey: {"admin"},
				},
			},
		},
	}, Config{AdminRoles: []string{"admin"}})

	resp, ok, err := wrapped.AuthenticateRequest(&http.Request{})
	require.NoError(t, err)
	require.True(t, ok)

	groups := resp.User.GetGroups()
	assert.Contains(t, groups, types.GroupAdmin)
	assert.Contains(t, groups, types.GroupMCP)
	assert.Contains(t, groups, types.GroupLLM)
	assert.Contains(t, groups, types.GroupSkills)
	assert.Contains(t, groups, types.GroupPublishedArtifacts)
	assert.Contains(t, groups, types.GroupAuthenticated)
	assert.NotContains(t, groups, types.GroupOwner)
}

func TestRoleUpliftPreservesDecoratedOwnerRole(t *testing.T) {
	wrapped := NewRoleUplift(staticAuthenticator{
		response: &authenticator.Response{
			User: &user.DefaultInfo{
				Name:   "owner",
				UID:    "1",
				Groups: types.RoleOwner.Groups(),
				Extra: map[string][]string{
					jwtRolesExtraKey: {"user"},
				},
			},
		},
	}, Config{AdminRoles: []string{"admin"}})

	resp, ok, err := wrapped.AuthenticateRequest(&http.Request{})
	require.NoError(t, err)
	require.True(t, ok)

	assert.Contains(t, resp.User.GetGroups(), types.GroupOwner)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAdmin)
}

func TestRoleUpliftDoesNotPromoteNonAdminJWT(t *testing.T) {
	wrapped := NewRoleUplift(staticAuthenticator{
		response: &authenticator.Response{
			User: &user.DefaultInfo{
				Name:   "bob",
				UID:    "43",
				Groups: types.RoleBasic.Groups(),
				Extra: map[string][]string{
					jwtRolesExtraKey: {"user"},
				},
			},
		},
	}, Config{AdminRoles: []string{"admin"}})

	resp, ok, err := wrapped.AuthenticateRequest(&http.Request{})
	require.NoError(t, err)
	require.True(t, ok)

	assert.NotContains(t, resp.User.GetGroups(), types.GroupAdmin)
	assert.NotContains(t, resp.User.GetGroups(), types.GroupOwner)
	assert.Contains(t, resp.User.GetGroups(), types.GroupBasic)
}

type staticAuthenticator struct {
	response *authenticator.Response
	ok       bool
	err      error
}

func (s staticAuthenticator) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	return s.response, s.response != nil || s.ok, nil
}
