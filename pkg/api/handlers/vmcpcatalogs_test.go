package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	mcpcataloghandler "github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	"github.com/obot-platform/obot/pkg/gitcredential"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVMCPCatalogCredentialsAreIsolated(t *testing.T) {
	const source = "https://github.com/example/catalog"
	storage := newVMCPTestStorage(&v1.VMCPCatalog{
		Name:      system.DefaultCatalog,
		Namespace: system.DefaultNamespace,
	})
	gateway := newHandlerTestGateway(t)
	request := httptest.NewRequest(http.MethodPut, "/api/vmcp-catalogs/default", nil)
	request.SetPathValue("catalog_id", system.DefaultCatalog)
	ctx := api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), Storage: storage, GatewayClient: gateway}
	mcpTokens := map[string]string{source: "mcp-secret"}
	require.NoError(t, storeCatalogTokens(ctx, system.DefaultCatalog, mcpTokens, nil))
	handler := NewVMCPCatalogHandler("")

	for _, tc := range []struct {
		name  string
		body  string
		token string
	}{
		{
			name:  "store",
			body:  `{"sourceURLs":["https://github.com/example/catalog"],"sourceURLCredentials":{"https://github.com/example/catalog":"vmcp-secret"}}`,
			token: "vmcp-secret",
		},
		{
			name:  "preserve",
			body:  `{"sourceURLs":["https://github.com/example/catalog"]}`,
			token: "vmcp-secret",
		},
		{
			name: "clear",
			body: `{"sourceURLs":[]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/vmcp-catalogs/default", strings.NewReader(tc.body))
			ctx.SetPathValue("catalog_id", system.DefaultCatalog)
			require.NoError(t, handler.Update(ctx))
			tokens, err := revealCatalogTokens(ctx, system.DefaultCatalog)
			require.NoError(t, err)
			assert.Equal(t, mcpTokens, tokens)
			token, err := gitcredential.ResolveOrReveal(t.Context(), storage, gateway, system.DefaultNamespace, "", source, mcpcataloghandler.VMCPCatalogCredentialContext(system.DefaultCatalog), mcpcataloghandler.CatalogCredentialToolName)
			require.NoError(t, err)
			assert.Equal(t, tc.token, token)
		})
	}
}
