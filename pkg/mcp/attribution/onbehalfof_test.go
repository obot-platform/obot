package attribution

import (
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

// onBehalfOfCaller returns agent 3, with its key, acting on behalf of user 7, Ann.
func onBehalfOfCaller(t *testing.T) *user.DefaultInfo {
	t.Helper()

	extra := map[string][]string{
		principal.APIKeyIDExtra:      {"42"},
		principal.APIKeyNameExtra:    {"assistant"},
		principal.APIKeyRawNameExtra: {"assistant"},
	}
	principal.RecordPrincipal(extra, principal.KindAgent, types.UserStatusActive)
	principal.RecordOnBehalfOf(extra, 7)
	require.NoError(t, agentprofile.RecordDelegation(extra, agentprofile.Delegation{
		ID:          "adl1test",
		User:        subjects.Caller{Principal: principal.User(7)},
		Email:       "ann@example.com",
		DisplayName: "Ann",
	}))
	return &user.DefaultInfo{UID: "agent:3", Name: "agent:3", Extra: extra}
}

// TestCallerObotHeadersOnBehalfOfAUser checks that an MCP server is told about the user an agent acts for beside the
// agent, in the on-behalf-of counterpart of each user header it selects, and only those.
func TestCallerObotHeadersOnBehalfOfAUser(t *testing.T) {
	caller := onBehalfOfCaller(t)

	headers, err := ForCaller(t.Context(), &headerIdentityReader{}, types.ObotHeaders(), caller)
	require.NoError(t, err)
	require.Equal(t, http.Header{
		"X-Obot-Api-Key-Name":            {"assistant"},
		"X-Obot-Api-Key-Id":              {"42"},
		"X-Obot-User-Id":                 {"agent:3"},
		"X-Obot-User-Name":               {"agent:3"},
		"X-Obot-On-Behalf-Of-User-Id":    {"7"},
		"X-Obot-On-Behalf-Of-User-Name":  {"Ann"},
		"X-Obot-On-Behalf-Of-User-Email": {"ann@example.com"},
	}, headers)

	headers, err = ForCaller(t.Context(), &headerIdentityReader{}, []types.ObotHeader{types.ObotHeaderUserID}, caller)
	require.NoError(t, err)
	require.Equal(t, http.Header{
		"X-Obot-User-Id":              {"agent:3"},
		"X-Obot-On-Behalf-Of-User-Id": {"7"},
	}, headers)

	// A caller acting for no one gets no on-behalf-of header.
	headers, err = ForCaller(t.Context(), &headerIdentityReader{}, types.ObotHeaders(), headerTestCaller("42", "jira-writer"))
	require.NoError(t, err)
	for _, header := range []types.ObotHeader{types.ObotHeaderOnBehalfOfUserID, types.ObotHeaderOnBehalfOfUserName, types.ObotHeaderOnBehalfOfUserEmail} {
		require.Empty(t, headers.Get(string(header)))
	}
}

// TestOnBehalfOfHeadersAreReserved checks that the on-behalf-of headers are never taken from a client, are forwarded
// from a vMCP to its components with the user header they accompany, and can never be configured.
func TestOnBehalfOfHeadersAreReserved(t *testing.T) {
	headers := http.Header{
		"X-Obot-User-Id":                 {"agent:3"},
		"X-Obot-On-Behalf-Of-User-Id":    {"7"},
		"X-Obot-On-Behalf-Of-User-Email": {"ann@example.com"},
		"X-Tenant":                       {"tenant"},
	}
	require.Equal(t, http.Header{
		"X-Obot-User-Id":              {"agent:3"},
		"X-Obot-On-Behalf-Of-User-Id": {"7"},
	}, Selected(headers, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderUserID}))
	require.Empty(t, Selected(headers, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderOnBehalfOfUserID}), "an on-behalf-of header is not selected on its own")

	spoofed := http.Header{
		"X-OBOT-ON-BEHALF-OF-USER-ID":    {"8"},
		"x-obot-on-behalf-of-user-name":  {"spoof"},
		"X-Obot-On-Behalf-Of-User-Email": {"spoof@example.com"},
		"X-Tenant":                       {"tenant"},
	}
	Strip(spoofed)
	require.Equal(t, http.Header{"X-Tenant": {"tenant"}}, spoofed)

	require.Empty(t, Configured([]string{"X-Obot-On-Behalf-Of-User-Id=8"}, []string{"x-obot-on-behalf-of-user-email"}, []string{"spoof@example.com"}))
}
