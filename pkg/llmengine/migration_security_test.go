package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMigrationDestinationErrorsKeepCredentialsSafe(t *testing.T) {
	user := "synthetic user/@"
	password := "synthetic password/@"
	token := "synthetic query/@"
	fragment := "synthetic fragment/@"
	sensitive := []string{user, password, token, fragment}
	for _, value := range []string{user, password, token, fragment} {
		sensitive = append(sensitive, url.QueryEscape(value), url.PathEscape(value))
	}
	for _, provider := range []string{system.GenericResponsesModelProvider, system.AzureModelProvider} {
		for _, destination := range []struct {
			name   string
			host   string
			path   string
			reason string
		}{
			{
				name:   "link local",
				host:   "169.254.169.254",
				path:   "/v1",
				reason: "link-local address, which can't be used",
			},
			{
				name:   "loopback",
				host:   "127.0.0.1",
				path:   "/v1",
				reason: "loopback network address, which needs Allow private network addresses",
			},
			{
				name:   "DNS failure",
				host:   "security-endpoint.invalid",
				path:   "/v1",
				reason: "could not resolve destination",
			},
			{
				name:   "DNS timeout",
				host:   "security-endpoint.invalid",
				path:   "/v1",
				reason: "destination lookup timeout",
			},
			{
				name:   "invalid port",
				host:   "169.254.169.254:bad",
				path:   "/v1",
				reason: "invalid URL",
			},
			{
				name:   "invalid escape",
				host:   "169.254.169.254",
				path:   "/v1/%zz",
				reason: "invalid URL",
			},
		} {
			t.Run(provider+"/"+destination.name, func(t *testing.T) {
				e, ctx := startTestEngine(t)
				g := migrationCredentials(t)
				if strings.HasPrefix(destination.name, "DNS") {
					previous := net.DefaultResolver
					net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
						return nil, &net.DNSError{Err: "synthetic DNS failure", IsTimeout: destination.name == "DNS timeout"}
					}}
					t.Cleanup(func() { net.DefaultResolver = previous })
				}
				t.Setenv(system.OpenAIAPIKeyEnvVar, "")
				t.Setenv(system.AnthropicAPIKeyEnvVar, "")
				address := "http://" + url.UserPassword(user, password).String() + "@" + destination.host + destination.path + "?" + url.Values{"token": {token}}.Encode() + "#" + url.PathEscape(fragment)
				secrets := azureSecrets(address)
				field := "azure_key_config.endpoint"
				config := map[string]any{}
				keys := []map[string]any{{"azure_key_config": map[string]any{"endpoint": address}}}
				if provider == system.GenericResponsesModelProvider {
					secrets = map[string]string{
						"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY":  "synthetic-api-key",
						"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": address,
					}
					field = "base URL"
					config = map[string]any{"network_config": map[string]any{"base_url": address}}
					keys = nil
				}
				putLegacy(t, g, provider, secrets)
				putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "independent-key"})

				_, importErr := e.ImportLegacyModelProviders(ctx, nil, g)
				require.Error(t, importErr)
				require.Contains(t, importErr.Error(), provider)
				require.Contains(t, importErr.Error(), field)
				require.Contains(t, importErr.Error(), destination.reason)
				_, err := e.store.GetProvider(ctx, "openai")
				require.NoError(t, err, "a rejected destination must not block independent imports")

				var logs bytes.Buffer
				previousLogger := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
				t.Cleanup(func() { slog.SetDefault(previousLogger) })
				mp := ModelProviderResource()
				c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(mp).Build()
				e.startupChanges(ctx, c, g, mp)()
				require.Contains(t, logs.String(), provider)
				require.Contains(t, logs.String(), field)
				require.Contains(t, logs.String(), destination.reason)

				status, err := e.MigrationStatus(ctx)
				require.NoError(t, err)
				raw, err := json.Marshal(status)
				require.NoError(t, err)
				// The controller returns ImportLegacyModelProviders' error unchanged; admin
				// provider and key writes reach the same destination validator below.
				body, err := json.Marshal(config)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &fields))
				adminErr := checkProviderAddresses(ctx, fields, keys)
				require.Error(t, adminErr)
				require.Contains(t, adminErr.Error(), destination.reason)
				path := "providers"
				if provider == system.AzureModelProvider {
					require.NoError(t, e.store.CreateProvider(ctx, "azure", llmcoreclient.Provider{}))
					path = "providers/azure/keys"
					body, err = json.Marshal(keys[0])
					require.NoError(t, err)
				}
				_, guardErr := e.GuardProviderChange(ctx, http.MethodPost, path, body, false)
				require.Error(t, guardErr)
				require.Contains(t, guardErr.Error(), field)
				require.Contains(t, guardErr.Error(), destination.reason)
				for _, output := range []string{importErr.Error(), adminErr.Error(), guardErr.Error(), logs.String(), string(raw)} {
					for _, secret := range sensitive {
						require.NotContains(t, output, secret)
					}
					require.NotContains(t, output, "synthetic", "encoded credentials must not survive sanitization")
				}
				if strings.Contains(destination.reason, "address") {
					require.Contains(t, importErr.Error(), destination.host)
				}
			})
		}
	}
}
