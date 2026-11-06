package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type subscriptionHandoffFixture struct {
	directFixture
	admitted bool
	auth     string
}

func (p *subscriptionHandoffFixture) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	p.admitted, _ = llm.SubscriptionAttempt(ctx)
	extra, _ := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
	p.auth = strings.Join(extra["authorization"], "")
	return p.directFixture.PreLLMHook(ctx, req)
}

func TestDirectHostedSubscriptionHandoff(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[allowed], func(t *testing.T) {
			proxy, req := directProxy(t, &directFixture{})
			store, err := llmcoreclient.New(req.Storage, directCodec{})
			require.NoError(t, err)
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{}))
			var model v1.Model
			require.NoError(t, req.Storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "m1-test"}, &model))
			model.Spec.Manifest.TargetModel = "anthropic/test"
			require.NoError(t, req.Storage.Update(t.Context(), &model))
			require.NoError(t, req.GatewayClient.SetLLMGatewaySettings(t.Context(), gatewaytypes.LLMGatewaySettings{SubscriptionPassthrough: allowed}))

			fixture := &subscriptionHandoffFixture{}
			input, _ := guardrails.New(req.GatewayClient, nil).Plugins()
			engine := llmengine.New()
			require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: t.TempDir(), FirstPlugin: input, LastPlugin: fixture}))
			t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
			proxy.backend = &bifrostProviderBackend{engine: engine}

			// Hosted egress supplies the Obot key separately, preserving Claude's login.
			const bearer = "Bearer sk-ant-oat-hosted-fixture"
			req.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/messages", strings.NewReader(`{"model":"m1-test","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
			req.SetPathValue("path", "v1/messages")
			req.Request.Header.Set("User-Agent", "claude-cli/2.1.168")
			req.Request.Header.Set("Authorization", bearer)
			req.Request.Header.Set(HeaderObotAPIKey, "obot-egress-fixture")
			response := httptest.NewRecorder()
			req.ResponseWriter = response
			err = proxy.proxy(req)
			if !allowed {
				require.Error(t, err)
				require.False(t, fixture.admitted)
				require.Empty(t, fixture.auth)
				return
			}
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.True(t, fixture.admitted, "core must classify the subscription before access/budget hooks")
			require.Equal(t, bearer, fixture.auth)
			require.Empty(t, fixture.headers["authorization"], "routing headers must not contain the token")
			require.Empty(t, fixture.headers[strings.ToLower(HeaderObotAPIKey)])
			require.NotContains(t, response.Body.String(), "sk-ant-oat-hosted-fixture")
		})
	}
}
