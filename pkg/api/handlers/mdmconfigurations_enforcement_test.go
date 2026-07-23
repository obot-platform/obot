package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

type mdmEnforcementTestEnv struct {
	gateway *gatewayclient.Client
	storage storage.Client
	handler *MDMConfigurationsHandler
}

func newMDMEnforcementTestEnv(t *testing.T) *mdmEnforcementTestEnv {
	t.Helper()
	return &mdmEnforcementTestEnv{
		gateway: newEnforcementTestGatewayClient(t),
		storage: storage.Client(newFakeStorage(t)),
		handler: NewMDMConfigurationsHandler("https://obot.example"),
	}
}

func (e *mdmEnforcementTestEnv) context(t *testing.T, method, target string, body any) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewReader(mustMarshal(t, body)))
	return api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        e.storage,
		GatewayClient:  e.gateway,
		User:           &user.DefaultInfo{UID: "42", Groups: []string{types.GroupAuthenticated, types.GroupAdmin}},
	}, rec
}

func (e *mdmEnforcementTestEnv) create(t *testing.T, body types.MDMConfiguration) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()
	return e.context(t, http.MethodPost, "/api/mdm/configurations", body)
}

func (e *mdmEnforcementTestEnv) updateEnforcement(t *testing.T, id uint, body types.MDMConfigurationEnforcementRequest) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()
	idStr := strconv.FormatUint(uint64(id), 10)
	ctx, rec := e.context(t, http.MethodPut, "/api/mdm/configurations/"+idStr+"/enforcement", body)
	ctx.SetPathValue("id", idStr)
	return ctx, rec
}

func (e *mdmEnforcementTestEnv) stored(t *testing.T, id uint) *gtypes.MDMConfiguration {
	t.Helper()
	configuration, err := e.gateway.GetMDMConfiguration(t.Context(), id)
	require.NoError(t, err)
	return configuration
}

func decodeMDMConfiguration(t *testing.T, rec *httptest.ResponseRecorder) types.MDMConfiguration {
	t.Helper()
	var configuration types.MDMConfiguration
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &configuration))
	return configuration
}

func requireMDMHTTPError(t *testing.T, err error, code int) {
	t.Helper()
	var httpErr *types.ErrHTTP
	require.True(t, errors.As(err, &httpErr), "error = %v, want HTTP %d", err, code)
	assert.Equal(t, code, httpErr.Code)
}

func TestMDMConfigurationCreateWithEnforcementAppliesDefaultAllowlist(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)

	ctx, rec := env.create(t, types.MDMConfiguration{EnforcementEnabled: true})
	require.NoError(t, env.handler.Create(ctx))
	require.Equal(t, http.StatusCreated, rec.Code)

	created := decodeMDMConfiguration(t, rec)
	require.True(t, created.EnforcementEnabled)
	allowlist := created.EnforcementAllowlist
	assert.True(t, allowlist.AllowAllObotHostedMCP)
	assert.True(t, allowlist.AllowAllBuiltinAgentTools)
	assert.True(t, allowlist.AllowAllBuiltinAgentMCP)
	assert.False(t, allowlist.AllowEverything)
	assert.Empty(t, allowlist.Servers)

	stored := env.stored(t, created.ID)
	assert.True(t, stored.EnforcementEnabled)
	assert.True(t, stored.EnforcementAllowlist.AllowAllObotHostedMCP)
	assert.True(t, stored.EnforcementAllowlist.AllowAllBuiltinAgentTools)
	assert.True(t, stored.EnforcementAllowlist.AllowAllBuiltinAgentMCP)
}

func TestMDMConfigurationCreateWithoutEnforcementHasNoAllowlist(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)

	ctx, rec := env.create(t, types.MDMConfiguration{})
	require.NoError(t, env.handler.Create(ctx))
	require.Equal(t, http.StatusCreated, rec.Code)

	created := decodeMDMConfiguration(t, rec)
	assert.False(t, created.EnforcementEnabled)
	assert.Empty(t, created.EnforcementAllowlist)

	stored := env.stored(t, created.ID)
	assert.False(t, stored.EnforcementEnabled)
	assert.Empty(t, stored.EnforcementAllowlist)
}

func TestMDMConfigurationCreateRejectsMalformedAllowlist(t *testing.T) {
	cases := map[string]types.EnforcementAllowlist{
		"two dimensions set": {Servers: []types.AllowlistServer{{URL: "https://a.example.com", Hostname: "a.example.com"}}},
		"no dimension set":   {Servers: []types.AllowlistServer{{Tools: []string{"x"}}}},
		"bad package source": {Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: "cargo", Name: "thing"}}}},
		"package no name":    {Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM}}}},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			env := newMDMEnforcementTestEnv(t)
			ctx, _ := env.create(t, types.MDMConfiguration{
				EnforcementEnabled:   true,
				EnforcementAllowlist: allowlist,
			})
			requireMDMHTTPError(t, env.handler.Create(ctx), http.StatusBadRequest)
		})
	}
}

func TestMDMConfigurationUpdateEnforcementRoundTripsCustomAllowlist(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)

	// A blank configuration with enforcement off.
	createCtx, createRec := env.create(t, types.MDMConfiguration{})
	require.NoError(t, env.handler.Create(createCtx))
	base := decodeMDMConfiguration(t, createRec)
	require.False(t, base.EnforcementEnabled)

	custom := types.EnforcementAllowlist{
		AllowAllBuiltinAgentTools: true,
		Servers: []types.AllowlistServer{
			{URL: "https://mcp.example.com/sse", Tools: []string{"search", "fetch"}},
			{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "@scope/server", Version: "1.2.3"}},
			{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: "some-server"}},
			{Hostname: "gitmcp.io"},
		},
	}
	ctx, rec := env.updateEnforcement(t, base.ID, types.MDMConfigurationEnforcementRequest{
		EnforcementEnabled:   true,
		EnforcementAllowlist: custom,
	})
	require.NoError(t, env.handler.UpdateEnforcement(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	updated := decodeMDMConfiguration(t, rec)
	assert.True(t, updated.EnforcementEnabled)
	allowlist := updated.EnforcementAllowlist
	assert.True(t, allowlist.AllowAllBuiltinAgentTools)
	require.Len(t, allowlist.Servers, 4)
	assert.Equal(t, "https://mcp.example.com/sse", allowlist.Servers[0].URL)
	assert.Equal(t, []string{"search", "fetch"}, allowlist.Servers[0].Tools)
	require.NotNil(t, allowlist.Servers[1].Package)
	assert.Equal(t, types.AllowlistServerPackageSourceNPM, allowlist.Servers[1].Package.Source)
	assert.Equal(t, "@scope/server", allowlist.Servers[1].Package.Name)
	assert.Equal(t, "1.2.3", allowlist.Servers[1].Package.Version)
	require.NotNil(t, allowlist.Servers[2].Package)
	assert.Equal(t, types.AllowlistServerPackageSourcePyPI, allowlist.Servers[2].Package.Source)
	assert.Empty(t, allowlist.Servers[2].Package.Version)
	assert.Equal(t, "gitmcp.io", allowlist.Servers[3].Hostname)

	stored := env.stored(t, base.ID)
	assert.True(t, stored.EnforcementEnabled)
	assert.Len(t, stored.EnforcementAllowlist.Servers, 4)
}

func TestMDMConfigurationUpdateEnforcementRejectsMalformedAllowlist(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)

	createCtx, createRec := env.create(t, types.MDMConfiguration{})
	require.NoError(t, env.handler.Create(createCtx))
	base := decodeMDMConfiguration(t, createRec)

	ctx, _ := env.updateEnforcement(t, base.ID, types.MDMConfigurationEnforcementRequest{
		EnforcementEnabled:   true,
		EnforcementAllowlist: types.EnforcementAllowlist{Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM}}}},
	})
	requireMDMHTTPError(t, env.handler.UpdateEnforcement(ctx), http.StatusBadRequest)

	// The rejected update leaves the stored policy untouched.
	stored := env.stored(t, base.ID)
	assert.False(t, stored.EnforcementEnabled)
	assert.Empty(t, stored.EnforcementAllowlist)
}

// TestMDMConfigurationUpdateEnforcementOnEnableKeepsEmptyAllowlist confirms the
// create-time default is not re-applied on the update route: enabling
// enforcement with an empty allowlist stores an empty (fail-closed) policy.
func TestMDMConfigurationUpdateEnforcementOnEnableKeepsEmptyAllowlist(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)

	createCtx, createRec := env.create(t, types.MDMConfiguration{})
	require.NoError(t, env.handler.Create(createCtx))
	base := decodeMDMConfiguration(t, createRec)

	ctx, rec := env.updateEnforcement(t, base.ID, types.MDMConfigurationEnforcementRequest{EnforcementEnabled: true})
	require.NoError(t, env.handler.UpdateEnforcement(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	updated := decodeMDMConfiguration(t, rec)
	assert.True(t, updated.EnforcementEnabled)
	assert.False(t, updated.EnforcementAllowlist.AllowAllObotHostedMCP)
	assert.Empty(t, updated.EnforcementAllowlist.Servers)
}
