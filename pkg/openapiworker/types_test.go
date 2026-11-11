package openapiworker_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapiworker"
	"github.com/stretchr/testify/require"
)

func TestBuildManifestAndExecutionDigest(t *testing.T) {
	schema := []byte(`{"openapi":"3.0.3","info":{"title":"Test","version":"1"},"servers":[{"url":"https://api.example.test/v1"}],"paths":{"/items/{id}":{"get":{"operationId":"getItem","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}},{"name":"X-Business","in":"header","required":false,"schema":{"type":"string"}}]}}}}`)
	config := types.OpenAPIRuntimeConfig{BaseURL: "https://api.example.test/v1"}
	headers := []types.MCPConfig{{
		Name: "Authorization", Key: "Authorization", Required: true, Sensitive: true, Prefix: "Bearer ", Usage: types.Header, UserAllowed: true,
	}}

	manifest, manifestBytes, policyBytes, digest, err := openapiworker.BuildManifest(schema, config, headers)
	require.NoError(t, err)
	require.Equal(t, "GET /items/{id}", manifest.Operations[0].OperationKey)
	require.Equal(t, "getItem", manifest.Operations[0].LegacyToolName)
	require.Len(t, manifest.Operations, 1)
	require.NotEmpty(t, manifest.SchemaDigest)
	require.NotEmpty(t, digest)
	require.JSONEq(t, `{"forwardable":[{"name":"X-Business"}],"credentials":[{"name":"Authorization","required":true,"prefix":"Bearer ","sensitive":true}]}`, string(policyBytes))

	_, repeatBytes, _, repeatDigest, err := openapiworker.BuildManifest(schema, config, headers)
	require.NoError(t, err)
	require.Equal(t, string(manifestBytes), string(repeatBytes))
	require.Equal(t, digest, repeatDigest)

	config.BaseURL = "https://other.example.test"
	_, _, _, changedDigest, err := openapiworker.BuildManifest(schema, config, headers)
	require.NoError(t, err)
	require.NotEqual(t, digest, changedDigest)
}

func TestStrictPathMatcher(t *testing.T) {
	operations := []openapiworker.Operation{{
		Method:         http.MethodGet,
		PathTemplate:   "/items/{id}",
		OperationKey:   "GET /items/{id}",
		LegacyToolName: "getItem",
	}}

	match, err := openapiworker.MatchOperation(operations, http.MethodGet, "/it%65ms/a%2Fb")
	require.NoError(t, err)
	require.Equal(t, "a%2Fb", match.PathBindings["id"])

	_, err = openapiworker.MatchOperation(operations, http.MethodGet, "/items/a/b")
	requireErrorCode(t, err, openapiworker.ErrorCodeOperationNotFound)
	_, err = openapiworker.MatchOperation(operations, http.MethodGet, "/items/%ZZ")
	require.Error(t, err)
	_, err = openapiworker.MatchOperation(operations, http.MethodGet, "//items/x")
	require.Error(t, err)
}

func requireErrorCode(t *testing.T, err error, code openapiworker.ErrorCode) {
	t.Helper()
	workerError, ok := openapiworker.AsError(err)
	require.True(t, ok)
	require.Equal(t, code, workerError.Code)
}

func TestHeaderClasses(t *testing.T) {
	policies := []openapiworker.HeaderPolicy{{
		Name: "X-Business", In: "header", Forwardable: true,
	}}
	input := http.Header{
		"Authorization":   {"caller"},
		"X-Obot-Trace":    {"caller"},
		"X-Forwarded-For": {"caller"},
		"X-Business":      {"one", "two"},
	}

	filtered := openapiworker.SanitizeInputHeaders(input, policies)
	require.Equal(t, []string{"one", "two"}, filtered.Values("X-Business"))
	require.Empty(t, filtered.Get("Authorization"))
	require.Empty(t, filtered.Get("X-Obot-Trace"))
	require.Empty(t, filtered.Get("X-Forwarded-For"))
}

func TestSnapshotEnvelopeDoesNotEncodeTwice(t *testing.T) {
	manifest := json.RawMessage(`{"manifestVersion":1}`)
	manifestBase64 := base64.StdEncoding.EncodeToString(manifest)
	policy := json.RawMessage(`{"forwardable":[],"credentials":[]}`)
	policyBase64 := base64.StdEncoding.EncodeToString(policy)
	envelope := openapiworker.SnapshotEnvelope{
		ProtocolVersion: openapiworker.ProtocolVersion, ExecutionDigest: "execution",
		ManifestBase64: manifestBase64, PolicyBase64: policyBase64, SchemaBase64: "c2NoZW1h",
		SchemaDigest: "digest", Manifest: manifest,
		Policy: openapiworker.HeaderPolicyDocument{
			Forwardable: []openapiworker.HeaderPolicyDefinition{}, Credentials: []openapiworker.HeaderPolicyDefinition{},
		},
	}
	clone := envelope.Clone()
	require.Equal(t, envelope, clone)
	encoded, err := json.Marshal(envelope)
	require.NoError(t, err)
	require.JSONEq(t, `{"protocolVersion":1,"executionDigest":"execution","manifestBase64":"`+manifestBase64+`","policyBase64":"`+policyBase64+`","schemaBase64":"c2NoZW1h"}`, string(encoded))
}
