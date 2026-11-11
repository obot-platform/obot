package apiproxy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewOpenAPIDocumentUsesProxyContract(t *testing.T) {
	document := newOpenAPIDocument(ComponentDocumentation{
		ID:              "component",
		DisplayName:     "FastAPI Items API",
		BasePath:        "https://obot.example.test/api-proxy/vmcp1abc/component",
		ExecutionDigest: "digest",
		Operations: []OperationDocumentation{{
			OperationKey:         "GET /items/{item_id}",
			LegacyToolName:       "get_item",
			DisplayName:          "Get Item",
			Method:               "GET",
			PathTemplate:         "/items/{item_id}",
			BusinessAuthRequired: true,
			Parameters: []ParameterDocumentation{{
				Name:     "item_id",
				In:       "path",
				Required: true,
				Schema:   json.RawMessage(`{"type":"string"}`),
			}},
		}},
	})

	require.Equal(t, "3.1.0", document.OpenAPI)
	require.Equal(t, "https://obot.example.test/api-proxy/vmcp1abc/component", document.Servers[0].URL)
	require.NotNil(t, document.Paths["/items/{item_id}"]["get"])

	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "x-obot")
	require.NotContains(t, string(encoded), "businessCredential")
	require.NotContains(t, string(encoded), "executionDigest")
	require.JSONEq(t, `{
		"openapi":"3.1.0",
		"info":{"title":"FastAPI Items API","version":"1.0.0","description":"Machine-readable contract for this API."},
		"servers":[{"url":"https://obot.example.test/api-proxy/vmcp1abc/component"}],
		"paths":{
			"/items/{item_id}":{
				"get":{
					"operationId":"get_item",
					"summary":"Get Item",
					"parameters":[{"name":"item_id","in":"path","required":true,"schema":{"type":"string"}}],
					"responses":{"default":{"description":"Business API response returned through Obot"}},
					"security":[{"X-API-Key":[]}]
				}
			}
		},
		"components":{"securitySchemes":{"X-API-Key":{"type":"apiKey","name":"X-API-Key","in":"header"}}}
	}`, string(encoded))
}
