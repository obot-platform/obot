package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func usersSchema(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/users.yaml")
	require.NoError(t, err)
	return data
}

func TestSchemaParseErrorDetails(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty",
			content: "  ",
			want:    "schema content is empty",
		},
		{
			name:    "syntax",
			content: "info:\n  title: [secret-marker\nversion: 1",
			want:    "did not find expected",
		},
		{
			name:    "unknown anchor",
			content: "info: *secret-marker",
			want:    "unknown YAML anchor",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.content), types.OpenAPIRuntimeConfig{})
			require.ErrorContains(t, err, test.want)
			require.NotContains(t, err.Error(), "secret-marker")
		})
	}
}

func documentWith(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	document, _, err := normalize(usersSchema(t))
	require.NoError(t, err)
	change(document)
	data, err := json.Marshal(document)
	require.NoError(t, err)
	return data
}

func TestNormalize(t *testing.T) {
	result, err := Parse(usersSchema(t), types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
	require.Equal(t, "https://api.example.com/v1/", result.BaseURL)
	var pretty strings.Builder
	encoder := json.NewEncoder(&pretty)
	encoder.SetIndent("", "  ")
	var document any
	require.NoError(t, json.Unmarshal([]byte(result.Schema), &document))
	require.NoError(t, encoder.Encode(document))
	jsonResult, err := Parse([]byte(pretty.String()), types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
	require.Equal(t, result, jsonResult)
	require.Contains(t, string(result.Schema), `"openapi":"3.1.0"`)

	override, err := Parse(usersSchema(t), types.OpenAPIRuntimeConfig{
		BaseURL: "https://override.example.com/",
	})
	require.NoError(t, err)
	require.Equal(t, "https://override.example.com/", override.BaseURL)
	require.Equal(t, result.Schema, override.Schema, "overrides must not rewrite the stored API document")
}

func TestWrapperVersions(t *testing.T) {
	for _, version := range []string{"3.0.0", "3.0.4", "3.1.0", "3.1.2", "3.0.5", "3.1.3", "3.2.0"} {
		t.Run(version, func(t *testing.T) {
			data := documentWith(t, func(d map[string]any) { d["openapi"] = version })
			_, err := Parse(data, types.OpenAPIRuntimeConfig{})
			switch version {
			case "3.0.5", "3.1.3", "3.2.0":
				require.ErrorContains(t, err, "supported OpenAPI versions are")
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestInlineImport(t *testing.T) {
	data := usersSchema(t)
	want, err := Parse(data, types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
	got, err := NewImporter(safehttp.Options{}, false).Import(context.Background(), types.OpenAPIRuntimeConfig{
		Source: types.OpenAPISource{Content: string(data)},
	})
	require.NoError(t, err)
	require.True(t, got.DestinationValidation.Valid)
	want.DestinationValidation = types.OpenAPIDestinationValidation{Valid: true}
	require.Equal(t, want, got)
}

func TestDestinationNetworkPolicy(t *testing.T) {
	for _, baseURL := range []string{"http://localhost:9999", "http://127.0.0.1:9999", "http://0.0.0.0:9999"} {
		t.Run(baseURL, func(t *testing.T) {
			config := types.OpenAPIRuntimeConfig{
				BaseURL: baseURL,
				Source:  types.OpenAPISource{Content: string(usersSchema(t))},
			}
			blocked := NewImporter(safehttp.Options{BlockLoopback: true}, true)
			result, err := blocked.Import(t.Context(), config)
			require.NoError(t, err)
			require.False(t, result.DestinationValidation.Valid)
			require.Contains(t, result.DestinationValidation.Error, "API destination is blocked")

			allowed := NewImporter(safehttp.Options{BlockPrivateIP: true, BlockLinkLocal: true}, true)
			result, err = allowed.Import(t.Context(), config)
			require.NoError(t, err)
			require.Equal(t, baseURL+"/", result.BaseURL)
			require.True(t, result.DestinationValidation.Valid)
		})
	}
}

func TestDestinationHTTPSRequirement(t *testing.T) {
	config := types.OpenAPIRuntimeConfig{
		BaseURL: "http://api.example.com",
		Source:  types.OpenAPISource{Content: string(usersSchema(t))},
	}

	result, err := NewImporter(safehttp.Options{}, false).Import(t.Context(), config)
	require.NoError(t, err)
	require.False(t, result.DestinationValidation.Valid)
	require.Contains(t, result.DestinationValidation.Error, "API destination must use HTTPS")

	result, err = NewImporter(safehttp.Options{}, true).Import(t.Context(), config)
	require.NoError(t, err)
	require.Equal(t, "http://api.example.com/", result.BaseURL)
	require.True(t, result.DestinationValidation.Valid)
}

func TestTypedParsingPreservesSnapshot(t *testing.T) {
	data := documentWith(t, func(d map[string]any) {
		d["x-custom"] = map[string]any{"preserved": true}
		d["components"] = map[string]any{
			"schemas": map[string]any{
				"Nullable": map[string]any{"type": []string{"string", "null"}},
				"Anything": true,
			},
		}
	})
	result, err := Parse(data, types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(result.Schema))

	malformed := documentWith(t, func(d map[string]any) {
		d["paths"].(map[string]any)["/users"].(map[string]any)["get"].(map[string]any)["operationId"] = 123
	})
	_, err = Parse(malformed, types.OpenAPIRuntimeConfig{})
	require.ErrorContains(t, err, "cannot parse OpenAPI document")
}

func TestUnsupportedDocuments(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(map[string]any)
		message string
	}{
		{
			name:    "v2",
			change:  func(d map[string]any) { d["openapi"] = "2.0" },
			message: "supported OpenAPI versions",
		},
		{
			name:    "external ref",
			change:  func(d map[string]any) { d["x-test"] = map[string]any{"$ref": "https://example.com/schema"} },
			message: "only local",
		},
		{
			name:    "unresolved ref",
			change:  func(d map[string]any) { d["x-test"] = map[string]any{"$ref": "#/missing"} },
			message: "unresolved",
		},
		{
			name:    "dynamic ref",
			change:  func(d map[string]any) { d["x-test"] = map[string]any{"$dynamicRef": "#node"} },
			message: "dynamic",
		},
		{
			name: "referenced path",
			change: func(d map[string]any) {
				d["paths"].(map[string]any)["/ref"] = map[string]any{"$ref": "#/paths/~1users"}
			},
			message: "referenced path",
		},
		{
			name: "callback",
			change: func(d map[string]any) {
				d["paths"].(map[string]any)["/users"].(map[string]any)["get"].(map[string]any)["callbacks"] = map[string]any{"event": map[string]any{}}
			},
			message: "callbacks",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(documentWith(t, test.change), types.OpenAPIRuntimeConfig{})
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestDocumentBounds(t *testing.T) {
	for _, data := range []string{
		"{}", "[]", "null", "openapi: [", "a: 1\na: 2", `{"a":1,"a":2}`,
		"a: &a [*a]", string(usersSchema(t)) + "\n---\n{}",
		strings.Repeat(" ", MaxSchemaBytes+1), "a: " + strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130),
		string([]byte{0xff}),
	} {
		_, err := Parse([]byte(data), types.OpenAPIRuntimeConfig{})
		require.Error(t, err)
	}
	// Common YAML numeric response codes are normalized into JSON object keys.
	_, err := Parse([]byte(strings.ReplaceAll(string(usersSchema(t)), "'200'", "200")), types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
}

func TestURLImport(t *testing.T) {
	var data atomic.Value
	data.Store(usersSchema(t))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("schema fetch received credentials")
		}
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/schema", http.StatusFound)
		case "/error":
			http.Error(w, "private upstream details", http.StatusBadGateway)
		case "/large":
			_, _ = fmt.Fprint(w, strings.Repeat(" ", MaxSchemaBytes+1))
		default:
			_, _ = w.Write(data.Load().([]byte))
		}
	}))
	defer server.Close()
	config := types.OpenAPIRuntimeConfig{
		Source:  types.OpenAPISource{URL: server.URL + "/schema"},
		Schema:  &types.OpenAPISchema{Raw: json.RawMessage(`{"old":"snapshot"}`)},
		BaseURL: "http://127.0.0.1",
	}
	_, err := NewImporter(safehttp.Options{}, false).Import(context.Background(), config)
	require.ErrorContains(t, err, "HTTPS")
	require.Zero(t, requests.Load(), "production mode must reject HTTP before fetching")
	_, err = NewImporter(safehttp.Options{
		BlockLoopback:  true,
		BlockPrivateIP: true,
		BlockLinkLocal: true,
	}, true).Import(context.Background(), config)
	require.Error(t, err)
	require.Zero(t, requests.Load())
	importer := NewImporter(safehttp.Options{BlockPrivateIP: true, BlockLinkLocal: true}, true)
	first, err := importer.Import(context.Background(), config)
	require.NoError(t, err)
	second, err := importer.Import(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, first.Schema, second.Schema)
	require.EqualValues(t, 2, requests.Load(), "each sync must fetch again")
	data.Store([]byte(strings.ReplaceAll(string(data.Load().([]byte)), "List users", "List all users")))
	changed, err := importer.Import(context.Background(), config)
	require.NoError(t, err)
	require.NotEqual(t, first.Schema, changed.Schema)
	for _, path := range []string{"/redirect", "/error", "/large"} {
		config.Source.URL = server.URL + path
		result, err := importer.Import(context.Background(), config)
		require.Error(t, err)
		if path == "/large" {
			require.ErrorContains(t, err, "schema exceeds 1 MiB")
		}
		require.Nil(t, result)
		require.NotContains(t, err.Error(), "private upstream details")
	}
	require.EqualValues(t, 6, requests.Load(), "redirect must not be followed")
	require.JSONEq(t, `{"old":"snapshot"}`, string(config.Schema.Raw))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = importer.Import(ctx, config)
	require.Error(t, err)
	for _, source := range []types.OpenAPISource{
		{}, {
			URL:     server.URL,
			Content: "{}",
		}, {URL: "file:///etc/passwd"},
		{URL: "https://user:secret@example.com/schema"},
	} {
		_, err := importer.Import(context.Background(), types.OpenAPIRuntimeConfig{Source: source})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestURLImportDoesNotForwardConfiguredCredentials(t *testing.T) {
	schema := usersSchema(t)
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		_, _ = w.Write(schema)
	}))
	defer server.Close()

	importer := NewImporter(safehttp.Options{
		Headers: http.Header{
			"Authorization": {"Bearer configured-header"},
			"X-API-Key":     {"configured-key"},
		},
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "configured-token"}),
	}, true)
	_, err := importer.Import(t.Context(), types.OpenAPIRuntimeConfig{
		Source: types.OpenAPISource{URL: server.URL},
	})
	require.NoError(t, err)
	header := <-received
	require.Empty(t, header.Get("Authorization"))
	require.Empty(t, header.Get("X-API-Key"))
}

func TestImportSchemaSizeLimit(t *testing.T) {
	data := documentWith(t, func(doc map[string]any) { doc["x-padding"] = "" })
	data = documentWith(t, func(doc map[string]any) {
		doc["x-padding"] = strings.Repeat("x", MaxSchemaBytes-len(data))
	})
	require.Len(t, data, MaxSchemaBytes)

	var response atomic.Value
	response.Store(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(response.Load().([]byte))
	}))
	defer server.Close()
	importer := NewImporter(safehttp.Options{}, true)

	for _, source := range []types.OpenAPISource{
		{Content: string(data)},
		{URL: server.URL},
	} {
		result, err := importer.Import(t.Context(), types.OpenAPIRuntimeConfig{
			Source:  source,
			BaseURL: "https://8.8.8.8",
		})
		require.NoError(t, err)
		require.Len(t, result.Schema, MaxSchemaBytes)
	}

	data = append(data, ' ')
	response.Store(data)
	for _, source := range []types.OpenAPISource{
		{Content: string(data)},
		{URL: server.URL},
	} {
		result, err := importer.Import(t.Context(), types.OpenAPIRuntimeConfig{Source: source})
		require.EqualError(t, err, "schema exceeds 1 MiB")
		require.Nil(t, result)
	}
}

func TestJSONSurrogatePair(t *testing.T) {
	data := documentWith(t, func(doc map[string]any) {
		doc["info"].(map[string]any)["description"] = "👍"
	})
	data = []byte(strings.ReplaceAll(string(data), "👍", `\ud83d\udc4d`))
	require.True(t, json.Valid(data))

	result, err := Parse(data, types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
	require.Equal(t, "👍", result.SuggestedMetadata.Description)
	require.Contains(t, string(result.Schema), "👍")
}

func TestJSONStructuralBounds(t *testing.T) {
	for _, data := range []string{
		`{"a":1,"\u0061":2}`,
		`{"nested":{"a":1,"a":2}}`,
	} {
		_, _, err := normalize([]byte(data))
		require.EqualError(t, err, "schema object keys must be unique strings")
	}

	data := `{"nested":` + strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128) + "}"
	_, _, err := normalize([]byte(data))
	require.EqualError(t, err, "schema nesting exceeds 128 levels")
}

func TestInvalidDestinationOverridePreservesSchema(t *testing.T) {
	data := usersSchema(t)
	result, err := NewImporter(safehttp.Options{}, false).Import(t.Context(), types.OpenAPIRuntimeConfig{
		Source:  types.OpenAPISource{Content: string(data)},
		BaseURL: "not-a-url",
	})
	require.NoError(t, err)
	require.False(t, result.DestinationValidation.Valid)
	require.Contains(t, result.DestinationValidation.Error, "baseURL must be absolute HTTP(S)")
	require.Equal(t, "not-a-url", result.BaseURL)
	require.NotEmpty(t, result.Schema)
}

func TestPreviewSourceRestrictionsRemainFatal(t *testing.T) {
	importer := NewImporter(safehttp.Options{
		BlockLoopback:  true,
		BlockPrivateIP: true,
		BlockLinkLocal: true,
	}, false)
	for _, test := range []struct {
		name      string
		sourceURL string
		want      string
	}{
		{
			name:      "HTTP source",
			sourceURL: "http://8.8.8.8/schema",
			want:      "schema source must use HTTPS",
		},
		{
			name:      "blocked source",
			sourceURL: "https://127.0.0.1/schema",
			want:      "schema source URL is blocked",
		},
		{
			name:      "source credentials",
			sourceURL: "https://user:secret@8.8.8.8/schema",
			want:      "without credentials or fragment",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := importer.Import(t.Context(), types.OpenAPIRuntimeConfig{
				Source:  types.OpenAPISource{URL: test.sourceURL},
				BaseURL: "https://8.8.4.4",
			})
			require.ErrorContains(t, err, test.want)
			require.NotContains(t, err.Error(), "secret")
			require.Nil(t, result)
		})
	}
}
