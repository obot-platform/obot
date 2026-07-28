package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mdmassets"
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

// createConfigWithAsset stores a minimal asset bundle whose instructions
// template bakes in --enforce when enforcementEnabled is set, then creates a
// configuration pinned to that bundle. It returns the configuration id.
func (e *mdmEnforcementTestEnv) createConfigWithAsset(t *testing.T) uint {
	t.Helper()
	dir := t.TempDir()
	manifest := `{
  "schemaVersion":"v1",
  "obotSentryVersion":"1.0.0",
  "fields":{"type":"object","additionalProperties":false,"required":["serverURL"],"properties":{"serverURL":{"type":"string","format":"uri","readOnly":true}}},
  "platforms":[{"id":"intune","label":"Intune"}],
  "configurations":[{"platform":"intune","os":"windows","osLabel":"Windows","instructions":"instructions.md.tmpl","assets":["package.bin","instructions.md.tmpl"]}]
}`
	for name, content := range map[string]string{
		"manifest.json":        manifest,
		"package.bin":          "package",
		"instructions.md.tmpl": "install=hook-install{{if .enforcementEnabled}} --enforce{{end}}",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	content, err := mdmassets.Import(t.Context(), dir)
	require.NoError(t, err)
	digest, err := e.gateway.StoreMDMAssetBundle(t.Context(), content)
	require.NoError(t, err)
	config, err := e.gateway.CreateMDMConfiguration(t.Context(), 42, &gtypes.MDMConfiguration{
		AssetDigest: digest,
		Values:      `{}`,
		Artifacts: []gtypes.MDMConfigurationArtifact{{
			Slug: "intune-windows", Platform: "intune", OS: "windows",
			Instructions: "install=hook-install", Content: []byte("initial-zip"),
		}},
	})
	require.NoError(t, err)
	return config.ID
}

// TestMDMConfigurationUpdateEnforcementReRendersArtifacts verifies that toggling
// enforcement on a configuration with a pinned asset re-renders its artifacts
// from stored state — the baked-in --enforce flag follows the toggle — without
// any asset digest in the request body.
func TestMDMConfigurationUpdateEnforcementReRendersArtifacts(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)
	id := env.createConfigWithAsset(t)

	// Enabling enforcement re-renders and bakes in --enforce.
	ctx, rec := env.updateEnforcement(t, id, types.MDMConfigurationEnforcementRequest{EnforcementEnabled: true})
	require.NoError(t, env.handler.UpdateEnforcement(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	stored := env.stored(t, id)
	require.True(t, stored.EnforcementEnabled)
	require.Len(t, stored.Artifacts, 1)
	assert.Contains(t, stored.Artifacts[0].Instructions, "hook-install --enforce")
	assert.NotEqual(t, "initial-zip", string(stored.Artifacts[0].Content), "artifacts were not re-rendered")
	// The re-render used the config's own stored digest + values, unchanged.
	assert.Equal(t, `{}`, stored.Values)

	// Disabling enforcement re-renders and drops --enforce.
	ctx, rec = env.updateEnforcement(t, id, types.MDMConfigurationEnforcementRequest{EnforcementEnabled: false})
	require.NoError(t, env.handler.UpdateEnforcement(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	stored = env.stored(t, id)
	require.False(t, stored.EnforcementEnabled)
	require.Len(t, stored.Artifacts, 1)
	assert.NotContains(t, stored.Artifacts[0].Instructions, "--enforce")
	assert.Contains(t, stored.Artifacts[0].Instructions, "install=hook-install")
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
		"two dimensions set":       {Servers: []types.AllowlistServer{{URL: "https://a.example.com", Hostname: "a.example.com"}}},
		"no dimension set":         {Servers: []types.AllowlistServer{{Tools: []string{"x"}}}},
		"bad package source":       {Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: "cargo", Name: "thing"}}}},
		"package no name":          {Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM}}}},
		"connector and hostname":   {Servers: []types.AllowlistServer{{Connector: "claude.ai Linear", Hostname: "mcp.linear.app"}}},
		"connector and url":        {Servers: []types.AllowlistServer{{Connector: "claude.ai Linear", URL: "https://mcp.linear.app/sse"}}},
		"blank connector only":     {Servers: []types.AllowlistServer{{Connector: "  \t "}}},
		"connector and package":    {Servers: []types.AllowlistServer{{Connector: "claude.ai Linear", Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "linear-mcp"}}}},
		"all four dimensions":      {Servers: []types.AllowlistServer{{URL: "https://a.example.com", Hostname: "a.example.com", Connector: "c", Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "p"}}}},
		"blank connector no other": {Servers: []types.AllowlistServer{{Connector: "", Tools: []string{"x"}}}},
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

func TestMDMConfigurationEnforcementAcceptsConnectorDimension(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)

	ctx, rec := env.create(t, types.MDMConfiguration{
		EnforcementEnabled: true,
		EnforcementAllowlist: types.EnforcementAllowlist{
			Servers: []types.AllowlistServer{
				{Connector: "  claude.ai Linear  ", Tools: []string{"search_issues"}},
			},
		},
	})
	require.NoError(t, env.handler.Create(ctx))
	require.Equal(t, http.StatusCreated, rec.Code)

	for label, allowlist := range map[string]types.EnforcementAllowlist{
		"response": decodeMDMConfiguration(t, rec).EnforcementAllowlist,
		"stored":   env.stored(t, decodeMDMConfiguration(t, rec).ID).EnforcementAllowlist,
	} {
		t.Run(label, func(t *testing.T) {
			require.Len(t, allowlist.Servers, 1)
			assert.Equal(t, "claude.ai Linear", allowlist.Servers[0].Connector)
			assert.Equal(t, []string{"search_issues"}, allowlist.Servers[0].Tools)
			assert.Empty(t, allowlist.Servers[0].URL)
			assert.Nil(t, allowlist.Servers[0].Package)
		})
	}
}

func TestMDMConfigurationEnforcementCanonicalizesPackageNames(t *testing.T) {
	for name, tt := range map[string]struct {
		in   types.AllowlistServerPackage
		want string
	}{
		"pypi underscores and dots": {
			in:   types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: " Mcp_Server.Git "},
			want: "mcp-server-git",
		},
		"pypi dotted namespace": {
			in:   types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: "awslabs.core-mcp-server"},
			want: "awslabs-core-mcp-server",
		},
		"npm uppercase scope": {
			in:   types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "@Scope/Pkg"},
			want: "@scope/pkg",
		},
		"npm separators preserved": {
			in:   types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "Linear_MCP"},
			want: "linear_mcp",
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := newMDMEnforcementTestEnv(t)
			pkg := tt.in
			ctx, rec := env.create(t, types.MDMConfiguration{
				EnforcementEnabled: true,
				EnforcementAllowlist: types.EnforcementAllowlist{
					Servers: []types.AllowlistServer{{Package: &pkg}},
				},
			})
			require.NoError(t, env.handler.Create(ctx))
			require.Equal(t, http.StatusCreated, rec.Code)

			created := decodeMDMConfiguration(t, rec)
			require.Len(t, created.EnforcementAllowlist.Servers, 1)
			require.NotNil(t, created.EnforcementAllowlist.Servers[0].Package)
			assert.Equal(t, tt.want, created.EnforcementAllowlist.Servers[0].Package.Name)

			stored := env.stored(t, created.ID)
			require.Len(t, stored.EnforcementAllowlist.Servers, 1)
			require.NotNil(t, stored.EnforcementAllowlist.Servers[0].Package)
			assert.Equal(t, tt.want, stored.EnforcementAllowlist.Servers[0].Package.Name)
		})
	}
}

func TestMDMConfigurationEnforcementCanonicalizationDoesNotHideABadSource(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)
	ctx, _ := env.create(t, types.MDMConfiguration{
		EnforcementEnabled: true,
		EnforcementAllowlist: types.EnforcementAllowlist{
			Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: "cargo", Name: "Some_Crate"}}},
		},
	})
	requireMDMHTTPError(t, env.handler.Create(ctx), http.StatusBadRequest)
}

func TestMDMConfigurationEnforcementLowercasesHostname(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)
	ctx, rec := env.create(t, types.MDMConfiguration{
		EnforcementEnabled: true,
		EnforcementAllowlist: types.EnforcementAllowlist{
			Servers: []types.AllowlistServer{{Hostname: "  GitMCP.IO  "}},
		},
	})
	require.NoError(t, env.handler.Create(ctx))

	created := decodeMDMConfiguration(t, rec)
	require.Len(t, created.EnforcementAllowlist.Servers, 1)
	assert.Equal(t, "gitmcp.io", created.EnforcementAllowlist.Servers[0].Hostname)
}

func TestMDMConfigurationUpdateEnforcementNormalizesAllowlist(t *testing.T) {
	env := newMDMEnforcementTestEnv(t)

	createCtx, createRec := env.create(t, types.MDMConfiguration{})
	require.NoError(t, env.handler.Create(createCtx))
	base := decodeMDMConfiguration(t, createRec)

	ctx, rec := env.updateEnforcement(t, base.ID, types.MDMConfigurationEnforcementRequest{
		EnforcementEnabled: true,
		EnforcementAllowlist: types.EnforcementAllowlist{
			Servers: []types.AllowlistServer{
				{URL: "  https://mcp.example.com/sse  ", Tools: []string{" search ", "fetch", "  "}},
				{Hostname: "\tgitmcp.io "},
				{Package: &types.AllowlistServerPackage{
					Source:  types.AllowlistServerPackageSourceNPM,
					Name:    " @scope/server ",
					Version: " 1.2.3 ",
				}},
			},
		},
	})
	require.NoError(t, env.handler.UpdateEnforcement(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	for label, allowlist := range map[string]types.EnforcementAllowlist{
		"response": decodeMDMConfiguration(t, rec).EnforcementAllowlist,
		"stored":   env.stored(t, base.ID).EnforcementAllowlist,
	} {
		t.Run(label, func(t *testing.T) {
			require.Len(t, allowlist.Servers, 3)
			assert.Equal(t, "https://mcp.example.com/sse", allowlist.Servers[0].URL)
			// The blank tool name is dropped; the real ones are trimmed.
			assert.Equal(t, []string{"search", "fetch"}, allowlist.Servers[0].Tools)
			assert.Equal(t, "gitmcp.io", allowlist.Servers[1].Hostname)
			require.NotNil(t, allowlist.Servers[2].Package)
			assert.Equal(t, "@scope/server", allowlist.Servers[2].Package.Name)
			assert.Equal(t, "1.2.3", allowlist.Servers[2].Package.Version)
		})
	}
}

func TestMDMConfigurationEnforcementRejectsAllBlankToolsList(t *testing.T) {
	for name, tools := range map[string][]string{
		"single empty string": {""},
		"several blanks":      {"", "   ", "\t"},
	} {
		t.Run(name, func(t *testing.T) {
			allowlist := types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://mcp.example.com/sse", Tools: tools}},
			}

			t.Run("on create", func(t *testing.T) {
				env := newMDMEnforcementTestEnv(t)
				ctx, _ := env.create(t, types.MDMConfiguration{
					EnforcementEnabled:   true,
					EnforcementAllowlist: allowlist,
				})
				requireMDMHTTPError(t, env.handler.Create(ctx), http.StatusBadRequest)
			})

			t.Run("on enforcement update", func(t *testing.T) {
				env := newMDMEnforcementTestEnv(t)
				createCtx, createRec := env.create(t, types.MDMConfiguration{})
				require.NoError(t, env.handler.Create(createCtx))
				base := decodeMDMConfiguration(t, createRec)

				ctx, _ := env.updateEnforcement(t, base.ID, types.MDMConfigurationEnforcementRequest{
					EnforcementEnabled:   true,
					EnforcementAllowlist: allowlist,
				})
				requireMDMHTTPError(t, env.handler.UpdateEnforcement(ctx), http.StatusBadRequest)

				// The rejected write must not have changed anything.
				stored := env.stored(t, base.ID)
				assert.False(t, stored.EnforcementEnabled)
				assert.Empty(t, stored.EnforcementAllowlist.Servers)
			})
		})
	}
}

func TestMDMConfigurationEnforcementWhitespaceOnlyDimensionRejected(t *testing.T) {
	for name, server := range map[string]types.AllowlistServer{
		"blank url":      {URL: "   "},
		"blank hostname": {Hostname: "\t "},
	} {
		t.Run(name, func(t *testing.T) {
			env := newMDMEnforcementTestEnv(t)
			ctx, _ := env.create(t, types.MDMConfiguration{
				EnforcementEnabled:   true,
				EnforcementAllowlist: types.EnforcementAllowlist{Servers: []types.AllowlistServer{server}},
			})
			requireMDMHTTPError(t, env.handler.Create(ctx), http.StatusBadRequest)
		})
	}
}

func TestMDMConfigurationEnforcementRejectsUnmatchableDimensions(t *testing.T) {
	for name, server := range map[string]types.AllowlistServer{
		"url without scheme":     {URL: "mcp.example.com/sse"},
		"scheme-relative url":    {URL: "//mcp.example.com/sse"},
		"url with typo'd scheme": {URL: "htp://mcp.example.com"},
		"non-http scheme":        {URL: "ftp://mcp.example.com"},
		"url without hostname":   {URL: "https://"},
		"unparseable url":        {URL: "https://exa mple.com"},
		"url with userinfo":      {URL: "https://user:pass@mcp.example.com"},
		"url with query":         {URL: "https://mcp.example.com/sse?tenant=a"},
		"url with fragment":      {URL: "https://mcp.example.com/sse#frag"},
		"hostname as url":        {Hostname: "https://gitmcp.io"},
		"hostname with port":     {Hostname: "gitmcp.io:443"},
		"hostname with path":     {Hostname: "gitmcp.io/mcp"},
	} {
		t.Run(name, func(t *testing.T) {
			allowlist := types.EnforcementAllowlist{Servers: []types.AllowlistServer{server}}

			t.Run("on create", func(t *testing.T) {
				env := newMDMEnforcementTestEnv(t)
				ctx, _ := env.create(t, types.MDMConfiguration{
					EnforcementEnabled:   true,
					EnforcementAllowlist: allowlist,
				})
				requireMDMHTTPError(t, env.handler.Create(ctx), http.StatusBadRequest)
			})

			t.Run("on enforcement update", func(t *testing.T) {
				env := newMDMEnforcementTestEnv(t)
				createCtx, createRec := env.create(t, types.MDMConfiguration{})
				require.NoError(t, env.handler.Create(createCtx))
				base := decodeMDMConfiguration(t, createRec)

				ctx, _ := env.updateEnforcement(t, base.ID, types.MDMConfigurationEnforcementRequest{
					EnforcementEnabled:   true,
					EnforcementAllowlist: allowlist,
				})
				requireMDMHTTPError(t, env.handler.UpdateEnforcement(ctx), http.StatusBadRequest)

				// The rejected write must not have changed anything.
				stored := env.stored(t, base.ID)
				assert.False(t, stored.EnforcementEnabled)
				assert.Empty(t, stored.EnforcementAllowlist.Servers)
			})
		})
	}
}

func TestMDMConfigurationEnforcementAcceptsMatchableDimensions(t *testing.T) {
	for name, server := range map[string]types.AllowlistServer{
		"host only":          {URL: "https://mcp.example.com"},
		"host with path":     {URL: "https://mcp.example.com/sse"},
		"trailing slash":     {URL: "https://mcp.example.com/"},
		"explicit port":      {URL: "https://mcp.example.com:8443/sse"},
		"plain http local":   {URL: "http://localhost:3000"},
		"untrimmed url":      {URL: "  https://mcp.example.com/sse  "},
		"bare hostname":      {Hostname: "gitmcp.io"},
		"untrimmed hostname": {Hostname: "\tgitmcp.io "},
	} {
		t.Run(name, func(t *testing.T) {
			env := newMDMEnforcementTestEnv(t)
			ctx, rec := env.create(t, types.MDMConfiguration{
				EnforcementEnabled:   true,
				EnforcementAllowlist: types.EnforcementAllowlist{Servers: []types.AllowlistServer{server}},
			})
			require.NoError(t, env.handler.Create(ctx))
			require.Len(t, decodeMDMConfiguration(t, rec).EnforcementAllowlist.Servers, 1)
		})
	}
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

func TestMDMConfigurationEnforcementFoldsEntriesThatNormalizeAlike(t *testing.T) {
	for name, tt := range map[string]struct {
		in   []types.AllowlistServer
		want []types.AllowlistServer
	}{
		"npm alias spellings": {
			in: []types.AllowlistServer{
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "mcp-server-time"}},
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "MCP-Server-Time"}},
			},
			want: []types.AllowlistServer{
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "mcp-server-time"}},
			},
		},
		"pypi alias spellings": {
			in: []types.AllowlistServer{
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: "mcp-server-git"}},
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: "Mcp_Server.Git"}},
			},
			want: []types.AllowlistServer{
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: "mcp-server-git"}},
			},
		},
		"hostname case": {
			in:   []types.AllowlistServer{{Hostname: "gitmcp.io"}, {Hostname: "GitMCP.IO"}},
			want: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
		},
		"connector case": {
			in:   []types.AllowlistServer{{Connector: "claude.ai Linear"}, {Connector: "claude.ai LINEAR"}},
			want: []types.AllowlistServer{{Connector: "claude.ai Linear"}},
		},
		// An entry allowing every tool is broader than one naming specific tools, so
		// it absorbs it rather than the reverse — matching the evaluator, where an
		// empty Tools list allows everything on the server.
		"all-tools entry absorbs a tool-scoped one": {
			in:   []types.AllowlistServer{{Hostname: "gitmcp.io"}, {Hostname: "gitmcp.io", Tools: []string{"fetch"}}},
			want: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
		},
		"tool-scoped entry widened by an all-tools one": {
			in:   []types.AllowlistServer{{Hostname: "gitmcp.io", Tools: []string{"fetch"}}, {Hostname: "gitmcp.io"}},
			want: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
		},
		"tool lists are unioned": {
			in: []types.AllowlistServer{
				{Hostname: "gitmcp.io", Tools: []string{"fetch"}},
				{Hostname: "gitmcp.io", Tools: []string{"fetch", "search"}},
			},
			want: []types.AllowlistServer{{Hostname: "gitmcp.io", Tools: []string{"fetch", "search"}}},
		},
		// A pinned version and an unpinned one are different rules: the evaluator
		// compares versions as exact strings and treats "" as any version.
		"a pinned version is a distinct entry": {
			in: []types.AllowlistServer{
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "pkg"}},
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "pkg", Version: "1.0.0"}},
			},
			want: []types.AllowlistServer{
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "pkg"}},
				{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "pkg", Version: "1.0.0"}},
			},
		},
		// Different dimensions never fold, even for the same underlying server.
		"url and hostname stay separate": {
			in:   []types.AllowlistServer{{URL: "https://gitmcp.io/docs"}, {Hostname: "gitmcp.io"}},
			want: []types.AllowlistServer{{URL: "https://gitmcp.io/docs"}, {Hostname: "gitmcp.io"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := newMDMEnforcementTestEnv(t)
			ctx, rec := env.create(t, types.MDMConfiguration{
				EnforcementEnabled:   true,
				EnforcementAllowlist: types.EnforcementAllowlist{Servers: tt.in},
			})
			require.NoError(t, env.handler.Create(ctx))
			require.Equal(t, http.StatusCreated, rec.Code)

			created := decodeMDMConfiguration(t, rec)
			assert.Equal(t, tt.want, created.EnforcementAllowlist.Servers)

			stored := env.stored(t, created.ID)
			assert.Equal(t, tt.want, stored.EnforcementAllowlist.Servers)
		})
	}
}
