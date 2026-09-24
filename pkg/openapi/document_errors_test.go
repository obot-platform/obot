package openapi

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestSchemaTypeErrorDetails(t *testing.T) {
	// Reduced from Redoc's Petstore demo: tuple-style items are not an
	// OpenAPI Schema Object. JSON and YAML must report the same useful error.
	for _, content := range []string{
		`{"openapi":"3.0.0","info":{"title":"Example","version":"1"},"servers":[{"url":"https://example.com"}],"paths":{},"components":{"schemas":{"User":{"type":"array","items":[{"type":"object"}]}}}}`,
		`openapi: 3.0.0
info:
  title: Example
  version: "1"
servers:
  - url: https://example.com
paths: {}
components:
  schemas:
    User:
      type: array
      items:
        - type: object
`,
	} {
		_, err := Parse([]byte(content), types.OpenAPIRuntimeConfig{})
		require.EqualError(t, err, "invalid OpenAPI document: expected object for Schema, got array")
	}
}

func TestSchemaTypeErrorDoesNotExposeValues(t *testing.T) {
	document := documentWith(t, func(doc map[string]any) {
		doc["info"].(map[string]any)["title"] = map[string]any{"secret-marker": "secret-value"}
	})
	_, err := Parse(document, types.OpenAPIRuntimeConfig{})
	require.ErrorContains(t, err, "cannot parse OpenAPI document")
	require.NotContains(t, err.Error(), "secret-marker")
	require.NotContains(t, err.Error(), "secret-value")
	for _, value := range []any{"secret-marker", 987654321} {
		document := documentWith(t, func(doc map[string]any) {
			doc["components"] = map[string]any{"schemas": map[string]any{"secret-schema-name": value}}
		})
		_, err := Parse(document, types.OpenAPIRuntimeConfig{})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-marker")
		require.NotContains(t, err.Error(), "secret-schema-name")
		require.NotContains(t, err.Error(), "987654321")
	}
}
