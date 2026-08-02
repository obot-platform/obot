package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testStaticOAuthCredentialGeneration = "test-generation"

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic spaces",
			input:    "My App Config",
			expected: "my-app-config",
		},
		{
			name:     "single quotes and spaces",
			input:    "My App's Config",
			expected: "my-app-s-config",
		},
		{
			name:     "special characters",
			input:    "Test_Server@1.0!",
			expected: "test-server-1-0",
		},
		{
			name:     "mixed case with symbols",
			input:    "Special!@#$%Characters",
			expected: "special-characters",
		},
		{
			name:     "multiple consecutive spaces",
			input:    "App   With   Spaces",
			expected: "app-with-spaces",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  App Config  ",
			expected: "app-config",
		},
		{
			name:     "leading and trailing special chars",
			input:    "!!!App Config***",
			expected: "app-config",
		},
		{
			name:     "only special characters",
			input:    "!@#$%^&*()",
			expected: "",
		},
		{
			name:     "already valid name",
			input:    "my-valid-name",
			expected: "my-valid-name",
		},
		{
			name:     "numbers and hyphens",
			input:    "app-v1.2.3",
			expected: "app-v1-2-3",
		},
		{
			name:     "unicode characters",
			input:    "café-résumé",
			expected: "caf-r-sum",
		},
		{
			name:     "long name gets truncated",
			input:    "this-is-a-very-long-name-that-exceeds-the-kubernetes-limit-of-sixty-three-characters-and-should-be-truncated",
			expected: "this-is-a-very-long-name-that-exceeds-the-kubernetes-limit-of-s",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "uppercase letters",
			input:    "UPPERCASE-NAME",
			expected: "uppercase-name",
		},
		{
			name:     "mixed alphanumeric with symbols",
			input:    "App123@#$Test456",
			expected: "app123-test456",
		},
		{
			name:     "parentheses and brackets",
			input:    "App (v2.0) [Production]",
			expected: "app-v2-0-production",
		},
		{
			name:     "dots and underscores",
			input:    "my.app_name.config",
			expected: "my-app-name-config",
		},
		{
			name:     "consecutive special chars become single dash",
			input:    "app!!!@@@###config",
			expected: "app-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMCPCatalogEntryName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeNameKubernetesCompliance(t *testing.T) {
	testCases := []string{
		"My App's Config",
		"Test_Server@1.0!",
		"Special!@#$%Characters",
		"App   With   Spaces",
		"  App Config  ",
		"café-résumé",
		"UPPERCASE-NAME",
		"App (v2.0) [Production]",
	}

	for _, input := range testCases {
		t.Run(input, func(t *testing.T) {
			result := normalizeMCPCatalogEntryName(input)

			// Test length constraint
			if len(result) > 63 {
				t.Errorf("NormalizeName(%q) = %q has length %d, exceeds 63 characters", input, result, len(result))
			}

			// Test character constraints (only lowercase alphanumeric and hyphens)
			for i, r := range result {
				if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
					t.Errorf("NormalizeName(%q) = %q contains invalid character %q at position %d", input, result, r, i)
				}
			}

			// Test that it doesn't start or end with hyphen (unless empty)
			if len(result) > 0 {
				if result[0] == '-' {
					t.Errorf("NormalizeName(%q) = %q starts with hyphen", input, result)
				}
				if result[len(result)-1] == '-' {
					t.Errorf("NormalizeName(%q) = %q ends with hyphen", input, result)
				}
			}
		})
	}
}

func newEntry(catalogName, workspaceID string) v1.MCPServerCatalogEntry {
	return v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName:       catalogName,
			PowerUserWorkspaceID: workspaceID,
		},
	}
}

func TestValidateEntryScope(t *testing.T) {
	tests := []struct {
		name        string
		entry       v1.MCPServerCatalogEntry
		catalogName string
		workspaceID string
		expectError bool
	}{
		{
			name:        "catalog entry matches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "default",
			expectError: false,
		},
		{
			name:        "catalog entry mismatches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "other",
			expectError: true,
		},
		{
			name:        "workspace entry matches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws1",
			expectError: false,
		},
		{
			name:        "workspace entry mismatches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws2",
			expectError: true,
		},
		{
			name:        "global catalog entry rejected by strict workspace check",
			entry:       newEntry("default", ""),
			workspaceID: "ws1",
			expectError: true,
		},
		{
			name:        "unscoped request for unscoped entry passes",
			entry:       newEntry("", ""),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEntryScope(tt.entry, tt.catalogName, tt.workspaceID)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateEntryVisibleFromScope(t *testing.T) {
	tests := []struct {
		name        string
		entry       v1.MCPServerCatalogEntry
		catalogName string
		workspaceID string
		expectError bool
	}{
		{
			name:        "catalog entry matches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "default",
			expectError: false,
		},
		{
			name:        "catalog entry mismatches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "other",
			expectError: true,
		},
		{
			name:        "workspace entry matches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws1",
			expectError: false,
		},
		{
			name:        "workspace entry mismatches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws2",
			expectError: true,
		},
		{
			name:        "global catalog entry allowed via workspace scope (relaxed)",
			entry:       newEntry("default", ""),
			workspaceID: "ws1",
			expectError: false,
		},
		{
			name:        "entry with no scope rejected via workspace scope",
			entry:       newEntry("", ""),
			workspaceID: "ws1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEntryVisibleFromScope(tt.entry, tt.catalogName, tt.workspaceID)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPopulateComponentManifestsHydratesMCPServerID(t *testing.T) {
	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-server", Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "default",
			Manifest: types.MCPServerManifest{
				Name:            "Shared Server",
				Runtime:         types.RuntimeContainerized,
				MultiUserConfig: &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{Key: "API_KEY", Name: "API Key"}}},
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "example/shared:1.0.0",
					Port:  8080,
					Path:  "/mcp",
				},
			},
		},
	}
	manifest := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{MCPServerID: "shared-server"},
		}},
	}

	err := (&MCPCatalogHandler{}).populateComponentManifests(newPopulateComponentManifestsRequest(server), &manifest, "default", "")

	require.NoError(t, err)
	require.Len(t, manifest.CompositeConfig.ComponentServers, 1)
	component := manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "shared-server", component.MCPServerID)
	assert.Empty(t, component.CatalogEntryID)
	assert.Equal(t, "Shared Server", component.Manifest.Name)
	assert.Equal(t, types.RuntimeContainerized, component.Manifest.Runtime)
	require.NotNil(t, component.Manifest.ContainerizedConfig)
	assert.Equal(t, "example/shared:1.0.0", component.Manifest.ContainerizedConfig.Image)
	require.NotNil(t, component.Manifest.MultiUserConfig)
}

func TestPopulateComponentManifestsHydratesSameCatalogEntryID(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "component-entry", Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "custom",
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:           "Component Server",
				Runtime:        types.RuntimeNPX,
				ServerUserType: types.ServerUserTypeSingleUser,
				NPXConfig:      &types.NPXRuntimeConfig{Package: "@example/component"},
			},
		},
	}
	manifest := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "component-entry"},
		}},
	}

	err := (&MCPCatalogHandler{}).populateComponentManifests(newPopulateComponentManifestsRequest(entry), &manifest, "custom", "")

	require.NoError(t, err)
	require.Len(t, manifest.CompositeConfig.ComponentServers, 1)
	component := manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "component-entry", component.CatalogEntryID)
	assert.Empty(t, component.MCPServerID)
	assert.Equal(t, "Component Server", component.Manifest.Name)
	assert.Equal(t, types.RuntimeNPX, component.Manifest.Runtime)
	require.NotNil(t, component.Manifest.NPXConfig)
	assert.Equal(t, "@example/component", component.Manifest.NPXConfig.Package)
}

func newPopulateComponentManifestsRequest(objects ...client.Object) api.Context {
	return api.Context{
		Request:        httptest.NewRequest(http.MethodGet, "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Storage: storage.Client(fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(objects...).
			Build()),
	}
}

func TestSetOAuthCredentialsRejectsProoflessSaveWithoutPersistingCredentials(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	recorder := httptest.NewRecorder()
	req := newStaticOAuthTestRequest(t, http.MethodPost, "/", `{"clientID":"candidate-client","clientSecret":"candidate-secret"}`, recorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)

	handler := &MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}
	if err := handler.SetOAuthCredentials(req); err == nil {
		t.Fatal("proofless credential Save succeeded")
	}
	if _, err := gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth"); err == nil {
		t.Fatal("proofless credential Save persisted credentials")
	}
}

func TestSetOAuthCredentialsRequiresExactSuccessfulOneUseProof(t *testing.T) {
	t.Run("successful exact proof is consumed after credential persistence", func(t *testing.T) {
		gateway := newOAuthCredentialTestGatewayClient(t)
		entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
		proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
		req, recorder := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)

		if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).SetOAuthCredentials(req); err != nil {
			t.Fatalf("save verified credentials: %v", err)
		}
		credential, err := gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth")
		if err != nil {
			t.Fatalf("reveal stored credential: %v", err)
		}
		if credential.Secrets["CLIENT_ID"] != "candidate-client" || credential.Secrets["CLIENT_SECRET"] != "candidate-secret" {
			t.Fatalf("stored credential = %#v", credential.Secrets)
		}
		if err := commitMCPStaticOAuthCredential(t.Context(), gateway, proof, "user-1", entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "candidate-client", "candidate-secret", true); !errors.Is(err, gatewayclient.ErrMCPStaticOAuthTestInvalid) {
			t.Fatalf("saved proof was reusable: %v", err)
		}
		if strings.Contains(recorder.Body.String(), "candidate-secret") {
			t.Fatalf("Save response exposed secret: %s", recorder.Body.String())
		}
		var updated v1.MCPServerCatalogEntry
		if err := req.Get(&updated, entry.Name); err != nil {
			t.Fatalf("get reconciled entry: %v", err)
		}
		if updated.Annotations[v1.MCPServerCatalogEntrySyncAnnotation] != "true" {
			t.Fatalf("entry was not marked for reconciliation: %#v", updated.Annotations)
		}
	})

	for _, tt := range []struct {
		name          string
		userID        string
		requestClient string
		requestSecret string
		prepare       func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string
	}{
		{name: "wrong proof", userID: "user-1", requestClient: "candidate-client", requestSecret: "candidate-secret", prepare: func(*testing.T, *gatewayclient.Client, *v1.MCPServerCatalogEntry) string { return "wrong-proof" }},
		{name: "proof with surrounding whitespace", userID: "user-1", requestClient: "candidate-client", requestSecret: "candidate-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			return " " + successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1") + " "
		}},
		{name: "wrong caller", userID: "user-2", requestClient: "candidate-client", requestSecret: "candidate-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			return successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
		}},
		{name: "changed client ID", userID: "user-1", requestClient: "changed-client", requestSecret: "candidate-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			return successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
		}},
		{name: "changed client secret", userID: "user-1", requestClient: "candidate-client", requestSecret: "changed-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			return successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
		}},
		{name: "pending proof", userID: "user-1", requestClient: "candidate-client", requestSecret: "candidate-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			return pendingStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1").TestState
		}},
		{name: "failed proof", userID: "user-1", requestClient: "candidate-client", requestSecret: "candidate-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			started := pendingStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
			if err := gateway.CompleteMCPStaticOAuthTest(t.Context(), started.CallbackState, types.MCPStaticOAuthTestStatusFailed, types.MCPStaticOAuthTestFailureTokenExchange); err != nil {
				t.Fatalf("fail static OAuth test: %v", err)
			}
			return started.TestState
		}},
		{name: "consumed proof", userID: "user-1", requestClient: "candidate-client", requestSecret: "candidate-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
			if err := commitMCPStaticOAuthCredential(t.Context(), gateway, proof, "user-1", entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "candidate-client", "candidate-secret", false); err != nil {
				t.Fatalf("consume proof through commit: %v", err)
			}
			if _, err := gateway.DeleteMCPStaticOAuthCredential(t.Context(), entry.Name); err != nil {
				t.Fatalf("remove committed test credential: %v", err)
			}
			return proof
		}},
		{name: "stale proof", userID: "user-1", requestClient: "candidate-client", requestSecret: "candidate-secret", prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
			t.Helper()
			proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
			if err := gateway.CleanupExpiredMCPOAuthPendingStates(t.Context(), 0); err != nil {
				t.Fatalf("expire proof: %v", err)
			}
			return proof
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newOAuthCredentialTestGatewayClient(t)
			entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
			proof := tt.prepare(t, gateway, entry)
			req, _ := newSetOAuthCredentialRequest(t, gateway, entry, tt.userID, tt.requestClient, tt.requestSecret, proof)
			if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).SetOAuthCredentials(req); err == nil {
				t.Fatal("Save accepted invalid proof")
			}
			if _, err := gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth"); err == nil {
				t.Fatal("invalid proof persisted credentials")
			}
		})
	}
}

func TestSetOAuthCredentialsReplacesCredentialBoundToPreviousCatalogURL(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://new-mcp.example/api")
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     "old-client",
			"CLIENT_SECRET": "old-secret",
			"MCP_URL":       "https://old-mcp.example/api",
			"GENERATION":    "old-generation",
		},
	}); err != nil {
		t.Fatalf("seed old-provider credential: %v", err)
	}
	proof := successfulStaticOAuthCredentialProofFor(
		t,
		gateway,
		entry.Name,
		entry.Spec.Manifest.RemoteConfig.FixedURL,
		"user-1",
		"new-client",
		"new-secret",
	)
	req, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "new-client", "new-secret", proof)

	if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).SetOAuthCredentials(req); err != nil {
		t.Fatalf("save new-provider credential: %v", err)
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth")
	if err != nil {
		t.Fatalf("reveal new-provider credential: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "new-client" ||
		credential.Secrets["CLIENT_SECRET"] != "new-secret" ||
		credential.Secrets["MCP_URL"] != entry.Spec.Manifest.RemoteConfig.FixedURL {
		t.Fatalf("new-provider credential = %#v", credential.Secrets)
	}
}

func TestSetOAuthCredentialsReplacesCredentialWithoutProviderBinding(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     "legacy-client",
			"CLIENT_SECRET": "legacy-secret",
		},
	}))
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	req, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)

	require.NoError(t, (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).SetOAuthCredentials(req))
	credential, err := gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth")
	require.NoError(t, err)
	require.Equal(t, "candidate-client", credential.Secrets["CLIENT_ID"])
	require.Equal(t, entry.Spec.Manifest.RemoteConfig.FixedURL, credential.Secrets["MCP_URL"])
	require.NotEmpty(t, credential.Secrets["GENERATION"])
}

func TestSetOAuthCredentialsReplacesSameProviderCredentialWithoutGeneration(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     "legacy-client",
			"CLIENT_SECRET": "legacy-secret",
			"MCP_URL":       entry.Spec.Manifest.RemoteConfig.FixedURL,
		},
	}))
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	req, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)

	require.NoError(t, (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).SetOAuthCredentials(req))
	credential, err := gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth")
	require.NoError(t, err)
	require.Equal(t, "candidate-client", credential.Secrets["CLIENT_ID"])
	require.Equal(t, entry.Spec.Manifest.RemoteConfig.FixedURL, credential.Secrets["MCP_URL"])
	require.NotEmpty(t, credential.Secrets["GENERATION"])
}

func TestSetOAuthCredentialsProofConsumptionOnRejectedSave(t *testing.T) {
	t.Run("storage failure", func(t *testing.T) {
		transformer := &toggleCredentialWriteErrorTransformer{failWrite: true}
		gateway := newOAuthCredentialTestGatewayClientWithEncryption(t, &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{
			{Group: "obot.obot.ai", Resource: "credentials"}: transformer,
		}})
		entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
		proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
		req, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
		if err := (&MCPCatalogHandler{gatewayClient: gateway}).SetOAuthCredentials(req); err == nil {
			t.Fatal("Save succeeded despite credential storage failure")
		}
		transformer.failWrite = false
		retry, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
		if err := (&MCPCatalogHandler{gatewayClient: gateway}).SetOAuthCredentials(retry); err == nil || !strings.Contains(err.Error(), "invalid or expired OAuth credential test") {
			t.Fatalf("storage failure left proof reusable: %v", err)
		}
	})

	t.Run("existing credential", func(t *testing.T) {
		gateway := newOAuthCredentialTestGatewayClient(t)
		entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
		if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: system.MCPOAuthCredentialName(entry.Name), Name: "oauth", Secrets: map[string]string{
			"CLIENT_ID": "existing", "CLIENT_SECRET": "existing-secret",
			"MCP_URL": entry.Spec.Manifest.RemoteConfig.FixedURL, "GENERATION": "existing-generation",
		}}); err != nil {
			t.Fatalf("seed existing credential: %v", err)
		}
		proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
		req, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
		if err := (&MCPCatalogHandler{gatewayClient: gateway}).SetOAuthCredentials(req); err == nil {
			t.Fatal("Save overwrote existing credential")
		}
		retry := newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
		if err := (&MCPCatalogHandler{gatewayClient: gateway}).ReplaceOAuthCredentials(retry); err == nil || !strings.Contains(err.Error(), "invalid or expired OAuth credential test") {
			t.Fatalf("existing-credential rejection left proof reusable: %v", err)
		}
	})
}

func TestSetOAuthCredentialsSerializesConcurrentSameProofSaves(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	credName := system.MCPOAuthCredentialName(entry.Name)

	release, err := gateway.AcquireCredentialLock(t.Context(), credName)
	if err != nil {
		t.Fatalf("hold credential lock: %v", err)
	}
	defer release()

	handler := &MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		req, _ := newSetOAuthCredentialRequest(t, gateway, entry.DeepCopy(), "user-1", "candidate-client", "candidate-secret", proof)
		go func() {
			<-start
			results <- handler.SetOAuthCredentials(req)
		}()
	}
	close(start)

	select {
	case err := <-results:
		t.Fatalf("concurrent Save bypassed the held credential lock: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	release()

	var successes int
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				successes++
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for serialized Save")
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent Save successes = %d, want exactly 1", successes)
	}

	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if err != nil {
		t.Fatalf("verified credential did not remain configured: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "candidate-client" || credential.Secrets["CLIENT_SECRET"] != "candidate-secret" {
		t.Fatalf("configured credential = %#v", credential.Secrets)
	}
}

func TestStaticOAuthSaveAndProviderUpdateShareCatalogMutationFence(t *testing.T) {
	const catalogMutationLock = "mcp-static-oauth-catalog-mutation"

	t.Run("Save waits for catalog mutation", func(t *testing.T) {
		gateway := newOAuthCredentialTestGatewayClient(t)
		entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
		proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
		req, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
		release, err := gateway.AcquireCredentialLock(t.Context(), catalogMutationLock)
		require.NoError(t, err)

		result := make(chan error, 1)
		go func() {
			result <- (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).SetOAuthCredentials(req)
		}()
		select {
		case err := <-result:
			release()
			t.Fatalf("Save bypassed catalog mutation fence: %v", err)
		case <-time.After(250 * time.Millisecond):
		}
		release()
		select {
		case err := <-result:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for fenced Save")
		}
	})

	t.Run("provider update waits for credential mutation", func(t *testing.T) {
		gateway := newOAuthCredentialTestGatewayClient(t)
		entry := staticOAuthTestEntry("entry-1", "default", "https://example.com/old")
		entry.Spec.Editable = true
		manifest := entry.Spec.Manifest
		manifest.RemoteConfig.FixedURL = "https://example.com/new"
		body, err := json.Marshal(manifest)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		req := newStaticOAuthTestRequest(t, http.MethodPut, "/", string(body), recorder, gateway,
			&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
		req.SetPathValue("catalog_id", "default")
		req.SetPathValue("entry_id", entry.Name)

		release, err := gateway.AcquireCredentialLock(t.Context(), catalogMutationLock)
		require.NoError(t, err)
		result := make(chan error, 1)
		go func() {
			result <- (&MCPCatalogHandler{gatewayClient: gateway}).UpdateEntry(req)
		}()
		select {
		case err := <-result:
			release()
			t.Fatalf("provider update bypassed credential mutation fence: %v", err)
		case <-time.After(250 * time.Millisecond):
		}
		release()
		select {
		case err := <-result:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for fenced provider update")
		}
	})
}

func TestSetOAuthCredentialsRejectsProviderChangeWhileWaitingForCredentialLock(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://new-mcp.example/api")
	credName := system.MCPOAuthCredentialName(entry.Name)
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: credName,
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     "old-client",
			"CLIENT_SECRET": "old-secret",
			"MCP_URL":       "https://old-mcp.example/api",
			"GENERATION":    "old-generation",
		},
	}))
	proof := successfulStaticOAuthCredentialProofFor(
		t,
		gateway,
		entry.Name,
		entry.Spec.Manifest.RemoteConfig.FixedURL,
		"user-1",
		"new-client",
		"new-secret",
	)
	req, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "new-client", "new-secret", proof)

	release, err := gateway.AcquireCredentialLock(t.Context(), credName)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		result <- (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).SetOAuthCredentials(req)
	}()
	select {
	case err := <-result:
		t.Fatalf("Save bypassed held credential lock: %v", err)
	case <-time.After(250 * time.Millisecond):
	}

	var currentEntry v1.MCPServerCatalogEntry
	require.NoError(t, req.Storage.Get(t.Context(), client.ObjectKey{Namespace: entry.Namespace, Name: entry.Name}, &currentEntry))
	currentEntry.Spec.Manifest.RemoteConfig.FixedURL = "https://old-mcp.example/api"
	require.NoError(t, req.Storage.Update(t.Context(), &currentEntry))
	release()

	select {
	case err := <-result:
		require.Error(t, err)
		require.Contains(t, err.Error(), "provider changed")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for rejected Save")
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	require.NoError(t, err)
	require.Equal(t, "old-client", credential.Secrets["CLIENT_ID"])
	require.Equal(t, "old-generation", credential.Secrets["GENERATION"])

	retry, _ := newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "new-client", "new-secret", proof)
	err = (&MCPCatalogHandler{gatewayClient: gateway}).SetOAuthCredentials(retry)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid or expired OAuth credential test")
}

func TestReplaceOAuthCredentialsRejectsMismatchedProofWithoutMutatingActiveConfiguration(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	server := &v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "server-a", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name}}
	instance := &v1.MCPServerInstance{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: server.Name, MCPServerCatalogEntryName: entry.Name}}
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret"}}); err != nil {
		t.Fatalf("seed active OAuth credential: %v", err)
	}
	for _, mcpID := range []string{server.Name, instance.Name} {
		if err := gateway.ReplaceMCPOAuthToken(t.Context(), instance.Spec.UserID, mcpID, entry.Spec.Manifest.RemoteConfig.FixedURL, "", &oauth2.Config{}, &oauth2.Token{AccessToken: "active-token-" + mcpID}); err != nil {
			t.Fatalf("seed active user token for %s: %v", mcpID, err)
		}
	}
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, instance.Spec.UserID)
	req := newReplaceOAuthCredentialRequest(t, gateway, entry, instance.Spec.UserID, "changed-after-test", "candidate-secret", proof, server, instance)

	if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).ReplaceOAuthCredentials(req); err == nil {
		t.Fatal("Replace accepted credentials that did not match the successful proof")
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if err != nil {
		t.Fatalf("reveal active credential after rejected replacement: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "active-client" || credential.Secrets["CLIENT_SECRET"] != "active-secret" {
		t.Fatalf("active credential changed after rejected replacement: %#v", credential.Secrets)
	}
	for _, mcpID := range []string{server.Name, instance.Name} {
		if _, err := gateway.GetMCPOAuthToken(t.Context(), instance.Spec.UserID, mcpID, entry.Spec.Manifest.RemoteConfig.FixedURL); err != nil {
			t.Fatalf("active user token for %s was removed after rejected replacement: %v", mcpID, err)
		}
	}
	retry := newReplaceOAuthCredentialRequest(t, gateway, entry, instance.Spec.UserID, "candidate-client", "candidate-secret", proof, server, instance)
	if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).ReplaceOAuthCredentials(retry); err == nil || !strings.Contains(err.Error(), "invalid or expired OAuth credential test") {
		t.Fatalf("rejected replacement left proof reusable: %v", err)
	}
}

func TestReplaceOAuthCredentialsRotatesGenerationForSameValueReplacement(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: credName,
		Name:    "oauth",
		Secrets: map[string]string{"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret"},
	}); err != nil {
		t.Fatalf("seed active OAuth credential: %v", err)
	}
	proof := successfulStaticOAuthCredentialProofFor(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1", "active-client", "active-secret")
	req := newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "active-client", "active-secret", proof)

	if err := (&MCPCatalogHandler{gatewayClient: gateway}).ReplaceOAuthCredentials(req); err != nil {
		t.Fatalf("same-value replacement failed: %v", err)
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if err != nil {
		t.Fatalf("reveal replaced credential: %v", err)
	}
	if credential.Secrets["GENERATION"] == "" {
		t.Fatal("same-value replacement did not rotate the credential generation")
	}
}

func TestReplaceOAuthCredentialsListFailureLeavesActiveConfigurationAndConsumesProof(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret"}}); err != nil {
		t.Fatalf("seed active OAuth credential: %v", err)
	}
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	req := newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
	req.Storage = oauthInstanceListErrorStorage{Client: req.Storage}

	if err := (&MCPCatalogHandler{gatewayClient: gateway}).ReplaceOAuthCredentials(req); err == nil {
		t.Fatal("Replace reported success after instance list failure")
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if err != nil {
		t.Fatalf("reveal active credential after list failure: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "active-client" || credential.Secrets["CLIENT_SECRET"] != "active-secret" {
		t.Fatalf("active credential changed after list failure: %#v", credential.Secrets)
	}
	retry := newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
	if err := (&MCPCatalogHandler{gatewayClient: gateway}).ReplaceOAuthCredentials(retry); err == nil || !strings.Contains(err.Error(), "invalid or expired OAuth credential test") {
		t.Fatalf("list failure left proof reusable: %v", err)
	}
}

func TestReplaceOAuthCredentialsUpsertFailureLeavesActiveConfigurationAndConsumesProof(t *testing.T) {
	transformer := &toggleCredentialWriteErrorTransformer{}
	gateway := newOAuthCredentialTestGatewayClientWithEncryption(t, &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{
		{Group: "obot.obot.ai", Resource: "credentials"}: transformer,
	}})
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret"}}); err != nil {
		t.Fatalf("seed active OAuth credential: %v", err)
	}
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	transformer.failWrite = true
	req := newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)

	if err := (&MCPCatalogHandler{gatewayClient: gateway}).ReplaceOAuthCredentials(req); err == nil {
		t.Fatal("Replace reported success after credential upsert failure")
	}
	transformer.failWrite = false
	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if err != nil {
		t.Fatalf("reveal active credential after upsert failure: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "active-client" || credential.Secrets["CLIENT_SECRET"] != "active-secret" {
		t.Fatalf("active credential changed after upsert failure: %#v", credential.Secrets)
	}
	retry := newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
	if err := (&MCPCatalogHandler{gatewayClient: gateway}).ReplaceOAuthCredentials(retry); err == nil || !strings.Contains(err.Error(), "invalid or expired OAuth credential test") {
		t.Fatalf("upsert failure left proof reusable: %v", err)
	}
}

func TestReplaceOAuthCredentialsSerializesConcurrentSameProofRequests(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret"}}); err != nil {
		t.Fatalf("seed active OAuth credential: %v", err)
	}
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	handler := &MCPCatalogHandler{gatewayClient: gateway}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		req := newReplaceOAuthCredentialRequest(t, gateway, entry.DeepCopy(), "user-1", "candidate-client", "candidate-secret", proof)
		go func() {
			<-start
			results <- handler.ReplaceOAuthCredentials(req)
		}()
	}
	close(start)

	var successes int
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				successes++
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for concurrent replacements")
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent replacement successes = %d, want exactly 1", successes)
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if err != nil {
		t.Fatalf("reveal credential after concurrent replacements: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "candidate-client" || credential.Secrets["CLIENT_SECRET"] != "candidate-secret" {
		t.Fatalf("credential after concurrent replacements = %#v", credential.Secrets)
	}
}

func TestReplaceOAuthCredentialsCleanupFailureLeavesNewConfigurationAndRequiresFreshProofToRetry(t *testing.T) {
	failCleanup := false
	gateway := newOAuthCredentialTestGatewayClientWithTrigger(t, func(_ context.Context, mcpID string) error {
		if failCleanup && mcpID == "instance-a-user-1" {
			return errors.New("instance token reconciliation unavailable")
		}
		return nil
	})
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	server := &v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "server-a", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name}}
	instance := &v1.MCPServerInstance{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: server.Name, MCPServerCatalogEntryName: entry.Name}}
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret"}}); err != nil {
		t.Fatalf("seed active OAuth credential: %v", err)
	}
	if err := gateway.ReplaceMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "", &oauth2.Config{}, &oauth2.Token{AccessToken: "active-token"}); err != nil {
		t.Fatalf("seed active instance token: %v", err)
	}
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, instance.Spec.UserID)
	failCleanup = true
	handler := &MCPCatalogHandler{gatewayClient: gateway}
	req := newReplaceOAuthCredentialRequest(t, gateway, entry.DeepCopy(), instance.Spec.UserID, "candidate-client", "candidate-secret", proof, server, instance)

	err := handler.ReplaceOAuthCredentials(req)
	if err == nil || !strings.Contains(err.Error(), instance.Name) {
		t.Fatalf("Replace cleanup error = %v, want instance-scoped error", err)
	}
	credential, revealErr := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if revealErr != nil {
		t.Fatalf("reveal credential after cleanup failure: %v", revealErr)
	}
	if credential.Secrets["CLIENT_ID"] != "candidate-client" || credential.Secrets["CLIENT_SECRET"] != "candidate-secret" {
		t.Fatalf("replacement credential after cleanup failure = %#v", credential.Secrets)
	}
	if _, err := gateway.GetMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("instance token remained after cleanup trigger failure: %v", err)
	}

	retryReq := newReplaceOAuthCredentialRequest(t, gateway, entry.DeepCopy(), instance.Spec.UserID, "candidate-client", "candidate-secret", proof, server, instance)
	if err := handler.ReplaceOAuthCredentials(retryReq); err == nil {
		t.Fatal("Replace reused the proof consumed by the applied replacement")
	}
	failCleanup = false
	freshProof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, instance.Spec.UserID)
	freshReq := newReplaceOAuthCredentialRequest(t, gateway, entry.DeepCopy(), instance.Spec.UserID, "candidate-client", "candidate-secret", freshProof, server, instance)
	if err := handler.ReplaceOAuthCredentials(freshReq); err != nil {
		t.Fatalf("Replace with a fresh proof did not retry cleanup: %v", err)
	}
}

func TestReplaceOAuthCredentialsSupportsWorkspaceScope(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "", "https://mcp.example/api")
	entry.Spec.PowerUserWorkspaceID = "workspace-1"
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret"},
	}); err != nil {
		t.Fatalf("seed workspace OAuth credential: %v", err)
	}
	workspace := &v1.PowerUserWorkspace{ObjectMeta: metav1.ObjectMeta{Name: "workspace-1", Namespace: system.DefaultNamespace}, Spec: v1.PowerUserWorkspaceSpec{UserID: "user-1"}}
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	req := newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof, workspace)
	req.SetPathValue("catalog_id", "")
	req.SetPathValue("workspace_id", workspace.Name)

	if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).ReplaceOAuthCredentials(req); err != nil {
		t.Fatalf("replace workspace OAuth app: %v", err)
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth")
	if err != nil {
		t.Fatalf("reveal workspace OAuth credential: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "candidate-client" {
		t.Fatalf("workspace replacement credential = %#v", credential.Secrets)
	}
}

func TestReplaceOAuthCredentialsCannotRecreateApplicationAfterClear(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credentialName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: credentialName,
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID": "active-client", "CLIENT_SECRET": "active-secret",
			"GENERATION": testStaticOAuthCredentialGeneration,
		},
	}); err != nil {
		t.Fatalf("seed active OAuth credential: %v", err)
	}
	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	handler := &MCPCatalogHandler{gatewayClient: gateway}

	if err := handler.DeleteOAuthCredentials(newDeleteOAuthCredentialRequest(t, gateway, entry.DeepCopy())); err != nil {
		t.Fatalf("clear OAuth credential: %v", err)
	}
	replace := newReplaceOAuthCredentialRequest(t, gateway, entry.DeepCopy(), "user-1", "candidate-client", "candidate-secret", proof)
	if err := handler.ReplaceOAuthCredentials(replace); err == nil {
		t.Fatal("stale replacement proof recreated a cleared OAuth application")
	}
	if _, err := gateway.RevealCredential(t.Context(), []string{credentialName}, "oauth"); !errors.As(err, &gatewayclient.CredentialNotFoundError{}) {
		t.Fatalf("cleared credential was recreated: %v", err)
	}
}

func TestGetOAuthCredentialsReturnsCallbackAndNeverSecret(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{"CLIENT_ID": "saved-client", "CLIENT_SECRET": "saved-secret", "MCP_URL": entry.Spec.Manifest.RemoteConfig.FixedURL, "GENERATION": "safe-generation"},
	}); err != nil {
		t.Fatalf("seed OAuth credential: %v", err)
	}
	recorder := httptest.NewRecorder()
	req := newStaticOAuthTestRequest(t, http.MethodGet, "/", "", recorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)

	if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).GetOAuthCredentials(req); err != nil {
		t.Fatalf("get OAuth credential status: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode credential status: %v", err)
	}
	if len(got) != 4 || got["configured"] != true || got["clientID"] != "saved-client" || got["generation"] != "safe-generation" || got["callbackURL"] != "https://obot.example/oauth/mcp/callback" {
		t.Fatalf("credential status = %#v", got)
	}
	if strings.Contains(recorder.Body.String(), "saved-secret") {
		t.Fatalf("credential status exposed secret: %s", recorder.Body.String())
	}
}

func TestGetOAuthCredentialsFailsClosedWithoutProviderBinding(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{"CLIENT_ID": "legacy-client", "CLIENT_SECRET": "legacy-secret"},
	}))
	recorder := httptest.NewRecorder()
	req := newStaticOAuthTestRequest(t, http.MethodGet, "/", "", recorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)

	require.NoError(t, (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).GetOAuthCredentials(req))
	var got types.MCPServerOAuthCredentialStatus
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	require.False(t, got.Configured)
	require.Empty(t, got.ClientID)
	require.Empty(t, got.Generation)
}

func TestGetOAuthCredentialsFailsClosedWithoutGeneration(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID": "legacy-client", "CLIENT_SECRET": "legacy-secret",
			"MCP_URL": entry.Spec.Manifest.RemoteConfig.FixedURL,
		},
	}))
	recorder := httptest.NewRecorder()
	req := newStaticOAuthTestRequest(t, http.MethodGet, "/", "", recorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)

	require.NoError(t, (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).GetOAuthCredentials(req))
	var got types.MCPServerOAuthCredentialStatus
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	require.False(t, got.Configured)
	require.Empty(t, got.ClientID)
	require.Empty(t, got.Generation)
}

func TestGetOAuthCredentialsFailsClosedAfterCatalogURLChanges(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://new-mcp.example/api")
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     "saved-client",
			"CLIENT_SECRET": "saved-secret",
			"MCP_URL":       "https://old-mcp.example/api",
			"GENERATION":    "safe-generation",
		},
	}); err != nil {
		t.Fatalf("seed OAuth credential: %v", err)
	}
	recorder := httptest.NewRecorder()
	req := newStaticOAuthTestRequest(t, http.MethodGet, "/", "", recorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)

	if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).GetOAuthCredentials(req); err != nil {
		t.Fatalf("get OAuth credential status: %v", err)
	}
	var got types.MCPServerOAuthCredentialStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode credential status: %v", err)
	}
	if got.Configured || got.ClientID != "" || got.Generation != "" {
		t.Fatalf("changed provider reported stale credential configured: %#v", got)
	}
}

func TestOAuthCredentialsFailClosedOnCredentialReadErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		method string
		run    func(t *testing.T, handler *MCPCatalogHandler, req api.Context)
	}{
		{
			name:   "GET does not report an unreadable credential as unconfigured",
			method: http.MethodGet,
			run: func(t *testing.T, handler *MCPCatalogHandler, req api.Context) {
				t.Helper()
				recorder := req.ResponseWriter.(*httptest.ResponseRecorder)
				if err := handler.GetOAuthCredentials(req); err == nil {
					t.Fatal("GET reported unreadable OAuth credential status")
				}
				if strings.Contains(recorder.Body.String(), `"configured":false`) {
					t.Fatalf("GET failed open with response %s", recorder.Body.String())
				}
			},
		},
		{
			name:   "Save does not overwrite an unreadable credential",
			method: http.MethodPost,
			run: func(t *testing.T, handler *MCPCatalogHandler, req api.Context) {
				t.Helper()
				if err := handler.SetOAuthCredentials(req); err == nil {
					t.Fatal("Save overwrote an unreadable OAuth credential")
				}
			},
		},
		{
			name:   "Replace does not overwrite an unreadable credential",
			method: http.MethodPut,
			run: func(t *testing.T, handler *MCPCatalogHandler, req api.Context) {
				t.Helper()
				if err := handler.ReplaceOAuthCredentials(req); err == nil {
					t.Fatal("Replace overwrote an unreadable OAuth credential")
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			transformer := &toggleCredentialReadErrorTransformer{}
			gateway := newOAuthCredentialTestGatewayClientWithEncryption(t, &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{
				{Group: "obot.obot.ai", Resource: "credentials"}: transformer,
			}})
			entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
			credName := system.MCPOAuthCredentialName(entry.Name)
			if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: credName,
				Name:    "oauth",
				Secrets: map[string]string{"CLIENT_ID": "existing", "CLIENT_SECRET": "existing-secret"},
			}); err != nil {
				t.Fatalf("seed OAuth credential: %v", err)
			}

			var req api.Context
			switch tt.method {
			case http.MethodPost:
				proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
				req, _ = newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
			case http.MethodPut:
				proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
				req = newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
			default:
				recorder := httptest.NewRecorder()
				req = newStaticOAuthTestRequest(t, tt.method, "/", "", recorder, gateway,
					&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
				req.SetPathValue("catalog_id", "default")
				req.SetPathValue("entry_id", entry.Name)
			}

			transformer.failRead = true
			tt.run(t, &MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}, req)
			transformer.failRead = false

			credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
			if err != nil {
				t.Fatalf("reveal original credential: %v", err)
			}
			if credential.Secrets["CLIENT_ID"] != "existing" || credential.Secrets["CLIENT_SECRET"] != "existing-secret" {
				t.Fatalf("credential changed after read failure: %#v", credential.Secrets)
			}
		})
	}
}

func TestOAuthCredentialsFailClosedOnTransientDatabaseReadErrors(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			gateway, rawDB := newOAuthCredentialTestGatewayClientWithOptionsAndDB(t, nil, nil)
			entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
			credName := system.MCPOAuthCredentialName(entry.Name)
			if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: credName,
				Name:    "oauth",
				Secrets: map[string]string{"CLIENT_ID": "existing", "CLIENT_SECRET": "existing-secret"},
			}); err != nil {
				t.Fatalf("seed OAuth credential: %v", err)
			}

			var req api.Context
			switch method {
			case http.MethodPost:
				proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
				req, _ = newSetOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
			case http.MethodPut:
				proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
				req = newReplaceOAuthCredentialRequest(t, gateway, entry, "user-1", "candidate-client", "candidate-secret", proof)
			default:
				recorder := httptest.NewRecorder()
				req = newStaticOAuthTestRequest(t, method, "/", "", recorder, gateway,
					&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
				req.SetPathValue("catalog_id", "default")
				req.SetPathValue("entry_id", entry.Name)
			}

			failCredentialRead := true
			callbackName := "test:transient_oauth_credential_read_" + strings.ToLower(method)
			if err := rawDB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if failCredentialRead && tx.Statement.Table == "credentials" {
					_ = tx.AddError(errors.New("transient credential database failure"))
				}
			}); err != nil {
				t.Fatalf("register transient credential read failure: %v", err)
			}
			t.Cleanup(func() { _ = rawDB.Callback().Query().Remove(callbackName) })

			handler := &MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}
			var err error
			switch method {
			case http.MethodPost:
				err = handler.SetOAuthCredentials(req)
			case http.MethodPut:
				err = handler.ReplaceOAuthCredentials(req)
			default:
				err = handler.GetOAuthCredentials(req)
			}
			if err == nil {
				t.Fatalf("%s failed open on transient credential database error", method)
			}

			failCredentialRead = false
			credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
			if err != nil {
				t.Fatalf("reveal original credential: %v", err)
			}
			if credential.Secrets["CLIENT_ID"] != "existing" || credential.Secrets["CLIENT_SECRET"] != "existing-secret" {
				t.Fatalf("credential changed after transient read failure: %#v", credential.Secrets)
			}
		})
	}
}

func TestDeleteOAuthCredentialsRetainsSiblingDeploymentsAndClearsEveryUserToken(t *testing.T) {
	triggered := map[string]int{}
	gateway := newOAuthCredentialTestGatewayClientWithTrigger(t, func(_ context.Context, mcpID string) error {
		triggered[mcpID]++
		return nil
	})
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	servers := []*v1.MCPServer{
		{ObjectMeta: metav1.ObjectMeta{Name: "server-a", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name}},
		{ObjectMeta: metav1.ObjectMeta{Name: "server-b", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name}},
	}
	instances := []*v1.MCPServerInstance{
		{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: servers[0].Name, MCPServerCatalogEntryName: entry.Name}},
		{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-2", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-2", MCPServerName: servers[0].Name, MCPServerCatalogEntryName: entry.Name}},
		{ObjectMeta: metav1.ObjectMeta{Name: "instance-b-user-1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: servers[1].Name, MCPServerCatalogEntryName: entry.Name}},
	}
	unrelatedServer := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server-unrelated", Namespace: system.DefaultNamespace},
		Spec:       v1.MCPServerSpec{MCPServerCatalogEntryName: "entry-unrelated"},
	}
	unrelatedInstance := &v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "instance-unrelated", Namespace: system.DefaultNamespace},
		Spec:       v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: unrelatedServer.Name, MCPServerCatalogEntryName: "entry-unrelated"},
	}
	rules := []*v1.AccessControlRule{
		{ObjectMeta: metav1.ObjectMeta{Name: "acr-a", Namespace: system.DefaultNamespace}, Spec: v1.AccessControlRuleSpec{MCPCatalogID: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "acr-b", Namespace: system.DefaultNamespace}, Spec: v1.AccessControlRuleSpec{MCPCatalogID: "default"}},
	}
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: system.MCPOAuthCredentialName(entry.Name), Name: "oauth", Secrets: map[string]string{
		"CLIENT_ID": "saved-client", "CLIENT_SECRET": "saved-secret", "GENERATION": testStaticOAuthCredentialGeneration,
	}}); err != nil {
		t.Fatalf("seed OAuth credential: %v", err)
	}
	if err := gateway.DeleteMCPOAuthTokenForAllUsers(t.Context(), "trigger-probe"); err != nil || triggered["trigger-probe"] != 1 {
		t.Fatalf("token cleanup trigger fixture is not active: %#v, %v", triggered, err)
	}
	clear(triggered)
	conf := &oauth2.Config{ClientID: "saved-client", ClientSecret: "saved-secret"}
	for _, instance := range instances {
		if err := gateway.ReplaceMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "", conf, &oauth2.Token{AccessToken: "instance-token-" + instance.Name}); err != nil {
			t.Fatalf("seed user token for %s: %v", instance.Name, err)
		}
	}
	for _, server := range servers {
		if err := gateway.ReplaceMCPOAuthToken(t.Context(), "single-user", server.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "", conf, &oauth2.Token{AccessToken: "server-token-" + server.Name}); err != nil {
			t.Fatalf("seed server-name token for %s: %v", server.Name, err)
		}
	}
	if err := gateway.ReplaceMCPOAuthToken(t.Context(), unrelatedInstance.Spec.UserID, unrelatedInstance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "", conf, &oauth2.Token{AccessToken: "unrelated-instance-token"}); err != nil {
		t.Fatalf("seed unrelated instance token: %v", err)
	}
	if err := gateway.ReplaceMCPOAuthToken(t.Context(), "single-user", unrelatedServer.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "", conf, &oauth2.Token{AccessToken: "unrelated-server-token"}); err != nil {
		t.Fatalf("seed unrelated server token: %v", err)
	}
	clear(triggered)

	objects := []client.Object{
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}},
		entry, servers[0], servers[1], instances[0], instances[1], instances[2], unrelatedServer, unrelatedInstance, rules[0], rules[1],
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(object client.Object) []string {
			return []string{object.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerCatalogEntryName", func(object client.Object) []string {
			return []string{object.(*v1.MCPServerInstance).Spec.MCPServerCatalogEntryName}
		}).
		WithObjects(objects...).
		Build()
	recorder := httptest.NewRecorder()
	req := api.Context{
		Request: httptest.NewRequest(
			http.MethodDelete, "/", strings.NewReader(`{"expectedGeneration":"`+testStaticOAuthCredentialGeneration+`"}`),
		),
		ResponseWriter: recorder,
		Storage:        storage.Client(storageClient),
		GatewayClient:  gateway,
		User:           &user.DefaultInfo{Name: "owner", UID: "user-1"},
	}
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)
	var indexedServers v1.MCPServerList
	if err := req.List(&indexedServers, client.MatchingFields{"spec.mcpServerCatalogEntryName": entry.Name}); err != nil || len(indexedServers.Items) != 2 {
		t.Fatalf("test server index returned %d siblings: %v", len(indexedServers.Items), err)
	}

	if err := (&MCPCatalogHandler{gatewayClient: gateway}).DeleteOAuthCredentials(req); err != nil {
		t.Fatalf("delete OAuth credentials: %v", err)
	}
	for _, server := range servers {
		var retained v1.MCPServer
		if err := req.Get(&retained, server.Name); err != nil {
			t.Fatalf("sibling deployment %s was removed: %v", server.Name, err)
		}
	}
	for _, instance := range instances {
		if _, err := gateway.GetMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("user token for %s remains: %v", instance.Name, err)
		}
	}
	for _, server := range servers {
		if _, err := gateway.GetMCPOAuthToken(t.Context(), "single-user", server.Name, entry.Spec.Manifest.RemoteConfig.FixedURL); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("server-name token for %s remains: %v", server.Name, err)
		}
	}
	if _, err := gateway.GetMCPOAuthToken(t.Context(), unrelatedInstance.Spec.UserID, unrelatedInstance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL); err != nil {
		t.Fatalf("unrelated instance token was removed: %v", err)
	}
	if _, err := gateway.GetMCPOAuthToken(t.Context(), "single-user", unrelatedServer.Name, entry.Spec.Manifest.RemoteConfig.FixedURL); err != nil {
		t.Fatalf("unrelated server token was removed: %v", err)
	}
	for _, rule := range rules {
		var retained v1.AccessControlRule
		if err := req.Get(&retained, rule.Name); err != nil {
			t.Fatalf("access control rule %s was removed: %v", rule.Name, err)
		}
	}
	for _, id := range []string{"server-a", "server-b", "instance-a-user-1", "instance-a-user-2", "instance-b-user-1"} {
		if triggered[id] != 1 {
			t.Fatalf("token cleanup triggers = %#v; %s count = %d, want 1", triggered, id, triggered[id])
		}
	}
	if len(triggered) != 5 {
		t.Fatalf("token cleanup triggers = %#v, want only matching servers and instances", triggered)
	}
	var updated v1.MCPServerCatalogEntry
	if err := req.Get(&updated, entry.Name); err != nil {
		t.Fatalf("get reconciled entry: %v", err)
	}
	if updated.Annotations[v1.MCPServerCatalogEntrySyncAnnotation] != "true" {
		t.Fatalf("entry was not marked for reconciliation: %#v", updated.Annotations)
	}
}

func TestDeleteOAuthCredentialsRejectsStaleGenerationWithoutRemovingCurrentApp(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credentialName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: credentialName,
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     "current-client",
			"CLIENT_SECRET": "current-secret",
			"MCP_URL":       entry.Spec.Manifest.RemoteConfig.FixedURL,
			"GENERATION":    "generation-2",
		},
	}); err != nil {
		t.Fatalf("seed current OAuth credential: %v", err)
	}
	req := newDeleteOAuthCredentialRequest(t, gateway, entry)
	req.Request = httptest.NewRequest(
		http.MethodDelete,
		"/",
		strings.NewReader(`{"expectedGeneration":"generation-1"}`),
	)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)

	err := (&MCPCatalogHandler{gatewayClient: gateway}).DeleteOAuthCredentials(req)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale Clear error = %v, want generation conflict", err)
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{credentialName}, "oauth")
	if err != nil {
		t.Fatalf("stale Clear removed current OAuth credential: %v", err)
	}
	if credential.Secrets["GENERATION"] != "generation-2" {
		t.Fatalf("current OAuth credential changed: %#v", credential.Secrets)
	}
}

func TestDeleteOAuthCredentialsReturnsServerListFailure(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{
		"CLIENT_ID": "saved-client", "CLIENT_SECRET": "saved-secret", "GENERATION": testStaticOAuthCredentialGeneration,
	}}); err != nil {
		t.Fatalf("seed OAuth credential: %v", err)
	}
	req := newDeleteOAuthCredentialRequest(t, gateway, entry)
	req.Storage = oauthServerListErrorStorage{Client: req.Storage}

	if err := (&MCPCatalogHandler{gatewayClient: gateway}).DeleteOAuthCredentials(req); err == nil {
		t.Fatal("Clear reported success after server list failure")
	}
	if _, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth"); err != nil {
		t.Fatalf("server-list failure removed the shared credential: %v", err)
	}
}

func TestDeleteOAuthCredentialsReturnsInstanceListFailure(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{
		"CLIENT_ID": "saved-client", "CLIENT_SECRET": "saved-secret", "GENERATION": testStaticOAuthCredentialGeneration,
	}}); err != nil {
		t.Fatalf("seed OAuth credential: %v", err)
	}
	req := newDeleteOAuthCredentialRequest(t, gateway, entry)
	req.Storage = oauthInstanceListErrorStorage{Client: req.Storage}

	if err := (&MCPCatalogHandler{gatewayClient: gateway}).DeleteOAuthCredentials(req); err == nil {
		t.Fatal("Clear reported success after instance list failure")
	}
	if _, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth"); err != nil {
		t.Fatalf("instance-list failure removed the shared credential: %v", err)
	}
}

func TestDeleteOAuthCredentialsReturnsServerTokenPurgeFailure(t *testing.T) {
	failPurge := false
	triggered := map[string]int{}
	gateway := newOAuthCredentialTestGatewayClientWithTrigger(t, func(_ context.Context, mcpID string) error {
		triggered[mcpID]++
		if failPurge && mcpID == "instance-a-user-1" {
			return errors.New("token purge trigger unavailable")
		}
		return nil
	})
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	server := &v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "server-a", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name}}
	instances := []*v1.MCPServerInstance{
		{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: server.Name, MCPServerCatalogEntryName: entry.Name}},
		{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-2", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-2", MCPServerName: server.Name, MCPServerCatalogEntryName: entry.Name}},
	}
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{
		"CLIENT_ID": "saved-client", "CLIENT_SECRET": "saved-secret", "GENERATION": testStaticOAuthCredentialGeneration,
	}}); err != nil {
		t.Fatalf("seed OAuth credential: %v", err)
	}
	for _, instance := range instances {
		if err := gateway.ReplaceMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "", &oauth2.Config{}, &oauth2.Token{AccessToken: "token-" + instance.Name}); err != nil {
			t.Fatalf("seed user token for %s: %v", instance.Name, err)
		}
	}
	clear(triggered)
	failPurge = true
	req := newDeleteOAuthCredentialRequest(t, gateway, entry, server, instances[0], instances[1])

	if err := (&MCPCatalogHandler{gatewayClient: gateway}).DeleteOAuthCredentials(req); err == nil {
		t.Fatal("Clear reported success after per-instance token purge failure")
	}
	for _, instance := range instances {
		if triggered[instance.Name] != 1 {
			t.Fatalf("cleanup did not attempt every matching instance: %#v", triggered)
		}
		if _, err := gateway.GetMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("token for %s remained after partial cleanup failure: %v", instance.Name, err)
		}
	}
}

func TestDeleteOAuthCredentialsRetryAfterTriggerFailureRetriesNotification(t *testing.T) {
	purgeAttempts := 0
	gateway := newOAuthCredentialTestGatewayClientWithTrigger(t, func(_ context.Context, mcpID string) error {
		if mcpID != "instance-a-user-1" {
			return nil
		}
		purgeAttempts++
		if purgeAttempts == 2 {
			return errors.New("token purge trigger unavailable")
		}
		return nil
	})
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	server := &v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "server-a", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name}}
	instance := &v1.MCPServerInstance{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: server.Name, MCPServerCatalogEntryName: entry.Name}}
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{
		"CLIENT_ID": "saved-client", "CLIENT_SECRET": "saved-secret", "GENERATION": testStaticOAuthCredentialGeneration,
	}}); err != nil {
		t.Fatalf("seed OAuth credential: %v", err)
	}
	if err := gateway.ReplaceMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "", &oauth2.Config{}, &oauth2.Token{AccessToken: "token"}); err != nil {
		t.Fatalf("seed user token: %v", err)
	}
	req := newDeleteOAuthCredentialRequest(t, gateway, entry, server, instance)
	handler := &MCPCatalogHandler{gatewayClient: gateway}

	if err := handler.DeleteOAuthCredentials(req); err == nil {
		t.Fatal("first Clear reported success after token purge failure")
	}
	if _, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth"); !errors.As(err, &gatewayclient.CredentialNotFoundError{}) {
		t.Fatalf("shared credential remained after first Clear: %v", err)
	}
	if err := handler.DeleteOAuthCredentials(newDeleteOAuthCredentialRequest(t, gateway, entry, server, instance)); err != nil {
		t.Fatalf("retry Clear after credential removal: %v", err)
	}
	if purgeAttempts != 3 {
		t.Fatalf("token purge attempts = %d, want seed plus both Clear attempts", purgeAttempts)
	}
	if _, err := gateway.GetMCPOAuthToken(t.Context(), instance.Spec.UserID, instance.Name, entry.Spec.Manifest.RemoteConfig.FixedURL); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("user token remained after retry: %v", err)
	}
}

func TestReplaceOAuthCredentialsAtomicallySwapsAppAndClearsOnlyMatchingTokens(t *testing.T) {
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	server := &v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "server-a", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: entry.Name}}
	instance := &v1.MCPServerInstance{ObjectMeta: metav1.ObjectMeta{Name: "instance-a-user-1", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: server.Name, MCPServerCatalogEntryName: entry.Name}}
	unrelatedServer := &v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "server-unrelated", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: "entry-unrelated"}}
	unrelatedInstance := &v1.MCPServerInstance{ObjectMeta: metav1.ObjectMeta{Name: "instance-unrelated", Namespace: system.DefaultNamespace}, Spec: v1.MCPServerInstanceSpec{UserID: "user-1", MCPServerName: unrelatedServer.Name, MCPServerCatalogEntryName: "entry-unrelated"}}
	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credName, Name: "oauth", Secrets: map[string]string{"CLIENT_ID": "old-client", "CLIENT_SECRET": "old-secret"}}); err != nil {
		t.Fatalf("seed old OAuth credential: %v", err)
	}
	for _, mcpID := range []string{server.Name, instance.Name, unrelatedServer.Name, unrelatedInstance.Name} {
		if err := gateway.ReplaceMCPOAuthToken(t.Context(), instance.Spec.UserID, mcpID, entry.Spec.Manifest.RemoteConfig.FixedURL, "", &oauth2.Config{ClientID: "old-client", ClientSecret: "old-secret"}, &oauth2.Token{AccessToken: "old-token-" + mcpID}); err != nil {
			t.Fatalf("seed old user token for %s: %v", mcpID, err)
		}
	}

	proof := successfulStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, instance.Spec.UserID)
	req := newReplaceOAuthCredentialRequest(t, gateway, entry.DeepCopy(), instance.Spec.UserID, "candidate-client", "candidate-secret", proof, server, instance, unrelatedServer, unrelatedInstance)
	if err := (&MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}).ReplaceOAuthCredentials(req); err != nil {
		t.Fatalf("replace OAuth app: %v", err)
	}

	for _, mcpID := range []string{server.Name, instance.Name} {
		if _, err := gateway.GetMCPOAuthToken(t.Context(), instance.Spec.UserID, mcpID, entry.Spec.Manifest.RemoteConfig.FixedURL); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("old matching token for %s survived OAuth app replacement: %v", mcpID, err)
		}
	}
	for _, mcpID := range []string{unrelatedServer.Name, unrelatedInstance.Name} {
		if _, err := gateway.GetMCPOAuthToken(t.Context(), instance.Spec.UserID, mcpID, entry.Spec.Manifest.RemoteConfig.FixedURL); err != nil {
			t.Fatalf("unrelated token for %s was removed: %v", mcpID, err)
		}
	}
	credential, err := gateway.RevealCredential(t.Context(), []string{credName}, "oauth")
	if err != nil {
		t.Fatalf("reveal replacement OAuth credential: %v", err)
	}
	if credential.Secrets["CLIENT_ID"] != "candidate-client" || credential.Secrets["CLIENT_SECRET"] != "candidate-secret" {
		t.Fatalf("replacement credential = %#v", credential.Secrets)
	}
}

func newDeleteOAuthCredentialRequest(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry, objects ...client.Object) api.Context {
	t.Helper()
	allObjects := []client.Object{
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}},
		entry,
	}
	allObjects = append(allObjects, objects...)
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(object client.Object) []string {
			return []string{object.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerCatalogEntryName", func(object client.Object) []string {
			return []string{object.(*v1.MCPServerInstance).Spec.MCPServerCatalogEntryName}
		}).
		WithObjects(allObjects...).
		Build()
	req := api.Context{
		Request: httptest.NewRequest(
			http.MethodDelete, "/", strings.NewReader(`{"expectedGeneration":"`+testStaticOAuthCredentialGeneration+`"}`),
		),
		ResponseWriter: httptest.NewRecorder(),
		Storage:        storage.Client(storageClient),
		GatewayClient:  gateway,
		User:           &user.DefaultInfo{Name: "owner", UID: "user-1"},
	}
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)
	return req
}

type oauthServerListErrorStorage struct {
	storage.Client
}

type oauthInstanceListErrorStorage struct {
	storage.Client
}

func (s oauthInstanceListErrorStorage) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v1.MCPServerInstanceList); ok {
		return errors.New("server instance list unavailable")
	}
	return s.Client.List(ctx, list, opts...)
}

func (oauthServerListErrorStorage) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	if _, ok := list.(*v1.MCPServerList); ok {
		return errors.New("server list unavailable")
	}
	return errors.New("unexpected list type")
}

func newSetOAuthCredentialRequest(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry, userID, clientID, clientSecret, proof string) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	body := fmt.Sprintf(`{"clientID":%q,"clientSecret":%q,"proof":%q}`, clientID, clientSecret, proof)
	req := newDeleteOAuthCredentialRequest(t, gateway, entry)
	req.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.ResponseWriter = recorder
	req.User = &user.DefaultInfo{Name: "owner", UID: userID}
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)
	return req, recorder
}

func newReplaceOAuthCredentialRequest(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry, userID, clientID, clientSecret, proof string, objects ...client.Object) api.Context {
	t.Helper()
	req := newDeleteOAuthCredentialRequest(t, gateway, entry, objects...)
	req.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(fmt.Sprintf(`{"clientID":%q,"clientSecret":%q,"proof":%q}`, clientID, clientSecret, proof)))
	req.ResponseWriter = httptest.NewRecorder()
	req.User = &user.DefaultInfo{Name: "owner", UID: userID}
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)
	return req
}

func successfulStaticOAuthCredentialProof(t *testing.T, gateway *gatewayclient.Client, entryName, fixedURL, userID string) string {
	t.Helper()
	return successfulStaticOAuthCredentialProofFor(t, gateway, entryName, fixedURL, userID, "candidate-client", "candidate-secret")
}

func commitMCPStaticOAuthCredential(ctx context.Context, gateway *gatewayclient.Client, proof, userID, entryName, fixedURL, clientID, clientSecret string, replace bool, cleanupMCPIDs ...string) error {
	claim, err := gateway.ClaimMCPStaticOAuthCredentialProof(ctx, proof, userID, entryName, fixedURL, clientID, clientSecret)
	if err != nil {
		return err
	}
	return gateway.CommitClaimedMCPStaticOAuthCredential(ctx, claim, replace, cleanupMCPIDs...)
}

func successfulStaticOAuthCredentialProofFor(t *testing.T, gateway *gatewayclient.Client, entryName, fixedURL, userID, clientID, clientSecret string) string {
	t.Helper()
	started := pendingStaticOAuthCredentialProofFor(t, gateway, entryName, fixedURL, userID, clientID, clientSecret)
	if err := gateway.CompleteMCPStaticOAuthTest(t.Context(), started.CallbackState, types.MCPStaticOAuthTestStatusSucceeded, ""); err != nil {
		t.Fatalf("complete static OAuth test: %v", err)
	}
	result, err := gateway.GetMCPStaticOAuthTestStatus(t.Context(), started.TestState, userID, entryName)
	if err != nil {
		t.Fatalf("read successful static OAuth test: %v", err)
	}
	if result.Proof == "" {
		t.Fatal("successful static OAuth test did not mint a Save proof")
	}
	return result.Proof
}

func pendingStaticOAuthCredentialProof(t *testing.T, gateway *gatewayclient.Client, entryName, fixedURL, userID string) gatewayclient.MCPStaticOAuthTestStart {
	t.Helper()
	return pendingStaticOAuthCredentialProofFor(t, gateway, entryName, fixedURL, userID, "candidate-client", "candidate-secret")
}

func pendingStaticOAuthCredentialProofFor(t *testing.T, gateway *gatewayclient.Client, entryName, fixedURL, userID, clientID, clientSecret string) gatewayclient.MCPStaticOAuthTestStart {
	t.Helper()
	proof, err := gateway.CreateMCPStaticOAuthTest(t.Context(), userID, entryName, fixedURL, "verifier", &oauth2.Config{
		ClientID: clientID, ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{AuthURL: "https://provider.example/authorize", TokenURL: "https://provider.example/token"},
	})
	if err != nil {
		t.Fatalf("create static OAuth test: %v", err)
	}
	return proof
}

type toggleCredentialReadErrorTransformer struct {
	failRead bool
}

type toggleCredentialWriteErrorTransformer struct {
	failWrite bool
}

func (t *toggleCredentialWriteErrorTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	if t.failWrite {
		return nil, errors.New("credential encryption unavailable")
	}
	return append([]byte("encrypted:"), data...), nil
}

func (*toggleCredentialWriteErrorTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	return bytes.TrimPrefix(data, []byte("encrypted:")), false, nil
}

func (*toggleCredentialReadErrorTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	return append([]byte("encrypted:"), data...), nil
}

func (t *toggleCredentialReadErrorTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	if t.failRead {
		return nil, false, errors.New("transient credential decryption failure")
	}
	return bytes.TrimPrefix(data, []byte("encrypted:")), false, nil
}
