package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
	"github.com/stretchr/testify/require"
)

func TestCallbackDeclarations(t *testing.T) {
	for _, version := range []string{"3.0.4", "3.1.2"} {
		t.Run(version, func(t *testing.T) {
			data := documentWith(t, func(document map[string]any) {
				document["openapi"] = version
				document["components"] = map[string]any{"schemas": map[string]any{
					"WebhookEvent": map[string]any{"type": "object"},
				}}
				document["paths"].(map[string]any)["/users"].(map[string]any)["post"].(map[string]any)["callbacks"] = map[string]any{
					"event": map[string]any{"{$request.body#/endpoint}": map[string]any{"post": map[string]any{
						"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/WebhookEvent"},
						}}},
						"responses": map[string]any{"200": map[string]any{"description": "Acknowledged"}},
					}}},
				}
			})
			result, err := Parse(data, types.OpenAPIRuntimeConfig{})
			require.NoError(t, err)
			require.JSONEq(t, string(data), string(result.Schema), "callback declarations remain in the snapshot")
			env, err := SnapshotEnvironment(types.OpenAPIRuntimeConfig{
				Schema: &types.OpenAPISchema{
					Raw: result.Schema,
				},
			}, nil, false)
			require.NoError(t, err)
			require.Contains(t, env, "OPENAPI_BASE_URL=https://api.example.com/v1/")
		})
	}
}

func TestCallbackReferences(t *testing.T) {
	for _, ref := range []string{"#/missing", "https://example.com/callback.json"} {
		data := documentWith(t, func(document map[string]any) {
			document["paths"].(map[string]any)["/users"].(map[string]any)["post"].(map[string]any)["callbacks"] = map[string]any{
				"event": map[string]any{"$ref": ref},
			}
		})
		_, err := Parse(data, types.OpenAPIRuntimeConfig{})
		if strings.HasPrefix(ref, "#/") {
			require.ErrorContains(t, err, "unresolved local reference")
		} else {
			require.ErrorContains(t, err, "only local JSON pointer references")
		}
	}
}

func TestCompatibleSchemaImports(t *testing.T) {
	for _, name := range []string{"typesense", "resend"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + name + ".yaml")
			require.NoError(t, err)
			parsed, err := Parse(data, types.OpenAPIRuntimeConfig{})
			require.NoError(t, err)
			require.Len(t, parsed.SuggestedHeaders, 1)
			var snapshot map[string]any
			require.NoError(t, json.Unmarshal(parsed.Schema, &snapshot))
			if name == "typesense" {
				require.Empty(t, parsed.BaseURL, "server variables require an explicit destination")
				parameters := snapshot["paths"].(map[string]any)["/analytics/events"].(map[string]any)["get"].(map[string]any)["parameters"].([]any)
				require.Equal(t, "n", parameters[0].(map[string]any)["name"])
				require.Equal(t, "X-TYPESENSE-API-KEY", parsed.SuggestedHeaders[0].Key)
			} else {
				require.Equal(t, "https://api.resend.com/", parsed.BaseURL)
				require.Contains(t, snapshot["webhooks"], "email.sent")
				require.Contains(t, snapshot["paths"], "/webhooks")
				require.Equal(t, "Authorization", parsed.SuggestedHeaders[0].Key)
				require.Equal(t, "Bearer ", parsed.SuggestedHeaders[0].Prefix)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(data)
			}))
			defer server.Close()
			importer := NewImporter(safehttp.Options{}, true)
			for _, source := range []types.OpenAPISource{
				{
					Content: string(data),
				},
				{
					URL: server.URL,
				},
			} {
				result, err := importer.Import(t.Context(), types.OpenAPIRuntimeConfig{
					Source:  source,
					BaseURL: "https://8.8.8.8",
				})
				require.NoError(t, err)
				require.Equal(t, parsed.Schema, result.Schema)
				require.Equal(t, "https://8.8.8.8/", result.BaseURL)
			}

			roundTrip, err := Parse(parsed.Schema, types.OpenAPIRuntimeConfig{})
			require.NoError(t, err)
			require.Equal(t, parsed, roundTrip)
		})
	}
}

func TestWebhookReferences(t *testing.T) {
	for _, ref := range []string{"#/missing", "https://example.com/schema.json"} {
		data := documentWith(t, func(document map[string]any) {
			document["webhooks"] = map[string]any{
				"event": map[string]any{"$ref": ref},
			}
		})
		_, err := Parse(data, types.OpenAPIRuntimeConfig{})
		if ref == "#/missing" {
			require.ErrorContains(t, err, "unresolved local reference")
		} else {
			require.ErrorContains(t, err, "only local JSON pointer references")
		}
	}
}
