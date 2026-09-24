package openapi

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestSuggestedMetadata(t *testing.T) {
	document := documentWith(t, func(doc map[string]any) {
		doc["info"] = map[string]any{
			"title":       " Example ",
			"version":     "1",
			"summary":     "**Short** [summary](https://example.com)",
			"description": "# Full description",
			"x-logo":      map[string]any{"url": "../logo.svg"},
		}
	})
	result, err := Parse(document, types.OpenAPIRuntimeConfig{Source: types.OpenAPISource{URL: "https://example.com/docs/openapi.json"}})
	require.NoError(t, err)
	require.Equal(t, types.OpenAPIMetadata{
		Name:             "Example",
		Description:      "# Full description",
		ShortDescription: "**Short** [summary](https://example.com)",
		Icon:             "https://example.com/logo.svg",
	}, result.SuggestedMetadata)
	require.JSONEq(t, string(document), string(result.Schema))
}

func TestMetadataOptionalFields(t *testing.T) {
	for _, logo := range []any{nil, "invalid", map[string]any{"url": 5}, map[string]any{"url": "javascript:alert(1)"}, map[string]any{"url": "data:image/png,example"}, map[string]any{"url": "http://example.com/logo"}, map[string]any{"url": "https://user:secret@example.com/logo"}, map[string]any{"url": "/logo.png"}} {
		document := documentWith(t, func(doc map[string]any) {
			doc["info"] = map[string]any{
				"title":       "Example",
				"version":     "1",
				"summary":     " ",
				"description": "**Hello** " + strings.Repeat("😀", 200),
				"x-logo":      logo,
			}
		})
		result, err := Parse(document, types.OpenAPIRuntimeConfig{})
		require.NoError(t, err)
		require.Empty(t, result.SuggestedMetadata.Icon)
		require.Empty(t, result.SuggestedMetadata.ShortDescription)
	}
}

func TestMetadataSummaryIsNotTruncated(t *testing.T) {
	summary := strings.Repeat("An API summary. ", 20)
	document := documentWith(t, func(doc map[string]any) {
		doc["info"].(map[string]any)["summary"] = summary
	})
	result, err := Parse(document, types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(summary), result.SuggestedMetadata.ShortDescription)
}
