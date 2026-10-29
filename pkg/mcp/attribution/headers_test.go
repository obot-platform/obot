package attribution

import (
	"context"
	"encoding/csv"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

type headerIdentityReader struct {
	identities []gatewaytypes.Identity
	err        error
	calls      int
	userID     uint
}

func headerTestCaller(id, name string) *user.DefaultInfo {
	return &user.DefaultInfo{
		UID:  "7",
		Name: "alice",
		Extra: map[string][]string{
			"email":                      {"alice@example.com"},
			principal.APIKeyIDExtra:      {id},
			principal.APIKeyNameExtra:    {name},
			principal.APIKeyRawNameExtra: {name},
		},
	}
}

func allObotHeaders() []types.ObotHeader {
	return types.ObotHeaders()
}

func (r *headerIdentityReader) FindIdentitiesForUser(_ context.Context, userID uint) ([]gatewaytypes.Identity, error) {
	r.calls++
	r.userID = userID
	return r.identities, r.err
}

func TestCallerObotHeaders(t *testing.T) {
	caller := headerTestCaller("42", "jira-writer")
	want := http.Header{
		"X-Obot-Api-Key-Name": {"jira-writer"},
		"X-Obot-Api-Key-Id":   {"42"},
		"X-Obot-User-Id":      {"7"},
		"X-Obot-User-Name":    {"alice"},
		"X-Obot-User-Email":   {"alice@example.com"},
	}
	t.Run("remote", func(t *testing.T) {
		got, err := ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, allObotHeaders(), caller)
		require.NoError(t, err)
		require.Equal(t, want, got)
		for _, selected := range allObotHeaders() {
			got, err := ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, []types.ObotHeader{selected}, caller)
			require.NoError(t, err)
			if value := want.Get(string(selected)); value != "" {
				require.Equal(t, http.Header{string(selected): {value}}, got)
			} else {
				require.Empty(t, got)
			}
		}
	})
	got, err := ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, nil, caller)
	require.NoError(t, err)
	require.Empty(t, got)
	got, err = ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, allObotHeaders(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
	got, err = ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeVMCP, allObotHeaders(), caller)
	require.NoError(t, err)
	require.Empty(t, got, "a parent vMCP cannot opt its components in")
	reader := &headerIdentityReader{err: fmt.Errorf("must not look up unsupported attribution")}
	got, err = ForCaller(t.Context(), reader, types.RuntimeContainerized, allObotHeaders(), caller)
	require.NoError(t, err)
	require.Empty(t, got, "containerized servers must not inject caller headers")
	require.Zero(t, reader.calls)
}

func TestCallerObotHeadersOmitUnavailableValues(t *testing.T) {
	caller := headerTestCaller("42", "")
	caller.Extra[principal.APIKeyNameExtra] = []string{"ok1-7-42-*****"}
	got, err := ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, allObotHeaders(), caller)
	require.NoError(t, err)
	require.Empty(t, got.Get(string(types.ObotHeaderAPIKeyName)), "audit fallback must not become a key name")
	require.Equal(t, "42", got.Get(string(types.ObotHeaderAPIKeyID)))
	delete(caller.Extra, principal.APIKeyIDExtra)
	delete(caller.Extra, "email")
	got, err = ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, allObotHeaders(), caller)
	require.NoError(t, err)
	require.Equal(t, http.Header{"X-Obot-User-Id": {"7"}, "X-Obot-User-Name": {"alice"}}, got)
}

func TestCallerObotHeadersOmitInvalidSelectedValues(t *testing.T) {
	caller := headerTestCaller("42", "invalid\r\nX-Injected: true")
	headers, err := ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderAPIKeyName}, caller)
	require.NoError(t, err)
	require.Empty(t, headers)
	_, err = ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderUserID}, caller)
	require.NoError(t, err, "unselected values do not affect forwarding")
}

func TestSelectedIDPUserID(t *testing.T) {
	first := gatewaytypes.Identity{AuthProviderName: "okta", AuthProviderNamespace: "default", ProviderUserID: "00u-alice"}
	second := gatewaytypes.Identity{AuthProviderName: "old-provider", AuthProviderNamespace: "default", ProviderUserID: "old-alice"}
	for _, tt := range []struct {
		name       string
		identities []gatewaytypes.Identity
		provider   string
		want       string
	}{
		{
			name:       "API key owner with one identity",
			identities: []gatewaytypes.Identity{first},
			want:       "00u-alice",
		},
		{
			name: "missing identity",
		},
		{
			name:       "ambiguous identities",
			identities: []gatewaytypes.Identity{first, second},
		},
		{
			name:       "request provider selects identity",
			identities: []gatewaytypes.Identity{first, second},
			provider:   "okta",
			want:       "00u-alice",
		},
		{
			name:       "provider mismatch",
			identities: []gatewaytypes.Identity{first},
			provider:   "other",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := headerTestCaller("42", "writer")
			caller.Extra["auth_provider_name"] = []string{tt.provider}
			reader := &headerIdentityReader{identities: tt.identities}
			got, err := ForCaller(t.Context(), reader, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderIDPUserID}, caller)
			require.NoError(t, err)
			require.Equal(t, tt.want, got.Get(string(types.ObotHeaderIDPUserID)))
			require.Equal(t, 1, reader.calls)
			require.Equal(t, uint(7), reader.userID, "look up the key owner, not a server owner")
		})
	}
	reader := &headerIdentityReader{err: fmt.Errorf("database unavailable")}
	caller := headerTestCaller("42", "writer")
	_, err := ForCaller(t.Context(), reader, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderIDPGroups}, caller)
	require.NoError(t, err)
	require.Zero(t, reader.calls, "unselected IdP ID must not cause identity lookups")
	_, err = ForCaller(t.Context(), reader, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderIDPUserID}, caller)
	require.ErrorContains(t, err, "resolve IdP user ID")
}

func TestCallerIDPHeaders(t *testing.T) {
	caller := headerTestCaller("42", "writer")
	caller.Groups = []string{types.GroupAdmin, types.GroupAuthenticated}
	groups := []string{"okta:z", "okta:a,comma", "okta:z", "", "okta:quote\""}
	caller.Extra["auth_provider_groups"] = groups
	selected := []types.ObotHeader{types.ObotHeaderIDPUserID, types.ObotHeaderIDPGroups}
	headers, err := ForCaller(t.Context(), &headerIdentityReader{identities: []gatewaytypes.Identity{{ProviderUserID: "00u-alice"}}}, types.RuntimeRemote, selected, caller)
	require.NoError(t, err)
	require.Equal(t, "00u-alice", headers.Get(string(types.ObotHeaderIDPUserID)))
	decoded, err := csv.NewReader(strings.NewReader(headers.Get(string(types.ObotHeaderIDPGroups)))).Read()
	require.NoError(t, err)
	require.Equal(t, []string{"okta:a,comma", "okta:quote\"", "okta:z"}, decoded)
	require.Equal(t, groups, caller.Extra["auth_provider_groups"], "must not mutate the authenticated principal")
	headers, err = ForCaller(t.Context(), &headerIdentityReader{identities: []gatewaytypes.Identity{{ProviderUserID: "id\r\nX-Spoof: true"}}}, types.RuntimeRemote, selected, caller)
	require.NoError(t, err)
	require.Empty(t, headers.Get(string(types.ObotHeaderIDPUserID)))
	require.NotEmpty(t, headers.Get(string(types.ObotHeaderIDPGroups)))
	delete(caller.Extra, "auth_provider_groups")
	headers, err = ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, selected, caller)
	require.NoError(t, err)
	require.Empty(t, headers, "Obot role groups must never become IdP groups")
}

func TestCallerIDPGroupsCommaSeparated(t *testing.T) {
	for _, tt := range []struct {
		name   string
		groups []string
		want   string
	}{
		{
			name:   "sorted and deduplicated without extra spaces or quotes",
			groups: []string{"okta:z", "", "okta:a", "okta:z"},
			want:   "okta:a,okta:z",
		},
		{
			name:   "commas and quotes use CSV escaping",
			groups: []string{"okta:z", "okta:a,comma", "okta:quote\""},
			want:   `"okta:a,comma","okta:quote""",okta:z`,
		},
		{
			name:   "invalid HTTP characters omit the whole list",
			groups: []string{"okta:safe", "okta:line\nbreak"},
		},
		{
			name:   "512 characters fit without JSON overhead",
			groups: []string{strings.Repeat("a", 255), strings.Repeat("z", 256)},
			want:   strings.Repeat("a", 255) + "," + strings.Repeat("z", 256),
		},
		{
			name:   "513 characters omit the whole list",
			groups: []string{strings.Repeat("a", 256), strings.Repeat("z", 256)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := headerTestCaller("42", "writer")
			caller.Extra["auth_provider_groups"] = tt.groups
			headers, err := ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, []types.ObotHeader{types.ObotHeaderIDPGroups}, caller)
			require.NoError(t, err)
			require.Equal(t, tt.want, headers.Get(string(types.ObotHeaderIDPGroups)))
		})
	}
}

func TestCallerObotHeaderSizeLimits(t *testing.T) {
	caller := headerTestCaller("42", "writer")
	caller.Name = strings.Repeat("界", maxObotHeaderValueCharacters)
	caller.Extra["email"] = []string{strings.Repeat("b", maxObotHeaderValueCharacters)}
	headers, err := ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, []types.ObotHeader{
		types.ObotHeaderUserName,
		types.ObotHeaderUserEmail,
		types.ObotHeaderUserID,
	}, caller)
	require.NoError(t, err)
	require.Equal(t, caller.Name, headers.Get(string(types.ObotHeaderUserName)), "a value at the per-field limit fits")
	require.Equal(t, caller.Extra["email"][0], headers.Get(string(types.ObotHeaderUserEmail)), "each header has an independent limit")
	require.Equal(t, "7", headers.Get(string(types.ObotHeaderUserID)), "other selected headers are still forwarded")

	caller.Name += "a"
	caller.Extra["auth_provider_groups"] = []string{"okta:small", strings.Repeat("g", maxObotHeaderValueCharacters)}
	headers, err = ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, []types.ObotHeader{
		types.ObotHeaderUserName,
		types.ObotHeaderIDPGroups,
		types.ObotHeaderUserID,
	}, caller)
	require.NoError(t, err)
	require.Empty(t, headers.Get(string(types.ObotHeaderUserName)))
	require.Empty(t, headers.Get(string(types.ObotHeaderIDPGroups)), "omit the entire group list instead of forwarding partial memberships")
	require.Equal(t, "7", headers.Get(string(types.ObotHeaderUserID)))
}

func TestStrip(t *testing.T) {
	headers := http.Header{
		"x-obot-api-key-name": {"spoof", "another spoof"},
		"X-Obot-Api-Key-Id":   {"999"},
		"X-OBOT-USER-ID":      {"999"},
		"X-Obot-User-Name":    {"spoof"},
		"X-Obot-User-Email":   {"spoof@example.com"},
		"X-Tenant":            {"tenant"},
		"X-OBOT-IDP-USER-ID":  {"spoof"},
		"x-obot-idp-groups":   {"[\"spoof\"]"},
	}
	Strip(headers)
	require.Equal(t, http.Header{"X-Tenant": {"tenant"}}, headers)
}

func TestConfiguredHeadersCanonicalizesNames(t *testing.T) {
	headers := Configured([]string{"AUTHORIZATION=Bearer token"}, []string{"x-request-id"}, []string{"request-1"})

	require.Equal(t, []string{"Bearer token"}, headers["Authorization"])
	require.Equal(t, []string{"request-1"}, headers["X-Request-Id"])
	require.NotContains(t, headers, "AUTHORIZATION")
}

func TestConfiguredPreservesTrustedAttribution(t *testing.T) {
	headers := http.Header{"X-Tenant": {"incoming"}}
	static := []string{"X-Tenant=static=value", "malformed"}
	keys := []string{"x-tenant"}
	values := []string{"per-user"}
	for _, name := range types.ObotHeaders() {
		headers.Set(string(name), "trusted")
		static = append(static, strings.ToLower(string(name))+"=static-spoof")
		keys = append(keys, strings.ToUpper(string(name)))
		values = append(values, "per-user-spoof")
	}
	keys = append(keys, "X-Missing-Value")
	configured := Configured(static, keys, values)
	require.Equal(t, http.Header{"X-Tenant": {"static=value"}}, configured)
	maps.Copy(headers, configured)
	for _, name := range types.ObotHeaders() {
		require.Equal(t, "trusted", headers.Get(string(name)))
	}
	require.Equal(t, "static=value", headers.Get("X-Tenant"))
}
