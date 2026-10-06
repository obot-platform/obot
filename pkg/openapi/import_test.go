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
			_, err := blocked.Import(t.Context(), config)
			require.ErrorContains(t, err, "API destination is blocked")

			allowed := NewImporter(safehttp.Options{BlockPrivateIP: true, BlockLinkLocal: true}, true)
			result, err := allowed.Import(t.Context(), config)
			require.NoError(t, err)
			require.Equal(t, baseURL+"/", result.BaseURL)
		})
	}
}

func TestDestinationHTTPSRequirement(t *testing.T) {
	config := types.OpenAPIRuntimeConfig{
		BaseURL: "http://api.example.com",
		Source:  types.OpenAPISource{Content: string(usersSchema(t))},
	}

	_, err := NewImporter(safehttp.Options{}, false).Import(t.Context(), config)
	require.ErrorContains(t, err, "API destination must use HTTPS")

	result, err := NewImporter(safehttp.Options{}, true).Import(t.Context(), config)
	require.NoError(t, err)
	require.Equal(t, "http://api.example.com/", result.BaseURL)
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
			name:    "relative servers only",
			change:  func(d map[string]any) { d["servers"] = []any{map[string]any{"url": "/v1"}} },
			message: "configure baseURL",
		},
		{
			name:    "webhooks",
			change:  func(d map[string]any) { d["webhooks"] = map[string]any{"hook": map[string]any{}} },
			message: "webhooks",
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
