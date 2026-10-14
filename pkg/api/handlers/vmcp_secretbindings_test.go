package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVMCPHandlerSecretBindings(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, tc := range []struct {
			name       string
			backend    string
			personal   bool
			nonAdmin   bool
			missing    bool
			unlabeled  bool
			empty      bool
			missingKey bool
			value      string
			options    []types.MCPConfigurationOption
			policy     types.VMCPConfigurationPolicyType
			usage      types.Usage
			runtime    types.Runtime
			wantError  string
		}{
			{
				name:    "allowed header binding",
				backend: mcp.RuntimeBackendKubernetes,
			},
			{
				name:    "allowed environment binding",
				backend: mcp.RuntimeBackendKubernetes,
				runtime: types.RuntimeNPX,
				usage:   types.Env,
			},
			{
				name:      "header binding with options",
				backend:   mcp.RuntimeBackendKubernetes,
				options:   []types.MCPConfigurationOption{{Value: "old-literal"}},
				wantError: "secretBinding and options are mutually exclusive",
			},
			{
				name:      "environment binding with options",
				backend:   mcp.RuntimeBackendKubernetes,
				runtime:   types.RuntimeNPX,
				usage:     types.Env,
				options:   []types.MCPConfigurationOption{{Value: "old-literal"}},
				wantError: "secretBinding and options are mutually exclusive",
			},
			{
				name:      "docker backend",
				backend:   "docker",
				wantError: "requires the kubernetes MCP runtime backend",
			},
			{
				name:      "personal admin",
				backend:   mcp.RuntimeBackendKubernetes,
				personal:  true,
				wantError: "administrator-managed shared vMCPs",
			},
			{
				name:      "personal power user",
				backend:   mcp.RuntimeBackendKubernetes,
				personal:  true,
				nonAdmin:  true,
				wantError: "administrator-managed shared vMCPs",
			},
			{
				name:      "missing secret",
				backend:   mcp.RuntimeBackendKubernetes,
				missing:   true,
				wantError: "must reference an allowed Kubernetes Secret",
			},
			{
				name:      "unlabeled secret",
				backend:   mcp.RuntimeBackendKubernetes,
				unlabeled: true,
				wantError: "must reference an allowed Kubernetes Secret",
			},
			{
				name:      "empty key value",
				backend:   mcp.RuntimeBackendKubernetes,
				empty:     true,
				wantError: "must reference an allowed Kubernetes Secret",
			},
			{
				name:       "missing key",
				backend:    mcp.RuntimeBackendKubernetes,
				missingKey: true,
				wantError:  "must reference an allowed Kubernetes Secret",
			},
			{
				name:      "literal and binding",
				backend:   mcp.RuntimeBackendKubernetes,
				value:     "literal",
				wantError: "mutually exclusive",
			},
			{
				name:      "user allowed binding",
				backend:   mcp.RuntimeBackendKubernetes,
				policy:    types.VMCPConfigurationPolicyUserAllowed,
				wantError: "only set secretBinding with fixed policy",
			},
			{
				name:      "remote environment binding",
				backend:   mcp.RuntimeBackendKubernetes,
				usage:     types.Env,
				wantError: "only supported for headers on remote runtime",
			},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				entry := vmcpCatalogEntryForTest("entry")
				if tc.runtime != "" {
					entry.Spec.Manifest.Runtime = tc.runtime
				}
				usage := tc.usage
				if usage == "" {
					usage = types.Header
				}
				entry.Spec.Manifest.Config = []types.MCPConfig{{
					Key:      "TOKEN",
					Usage:    usage,
					Required: true,
					Options:  tc.options,
				}}
				storage := newVMCPTestStorage(entry)
				handler := vmcpHandlerForTest(t, storage)
				handler.mcpBackend = tc.backend
				handler.secretBindingAllowedLabel = testSecretBindingAllowedLabel
				gatewayClient := newHandlerTestGateway(t)
				u := &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupAdmin}}
				if tc.nonAdmin {
					u = &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupPowerUser}}
				}
				manifest := testVMCPManifest()
				manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "old-literal"}}
				var id string
				query := ""
				if tc.personal {
					query = "?scope=personal"
				}
				if operation == "update" {
					created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, u, query)
					manifest = created.VMCPManifest
					id = created.ID
				}
				binding := &types.MCPSecretBinding{Name: "tokens", Key: "token"}
				policy := types.VMCPConfigurationPolicyFixed
				if tc.policy != "" {
					policy = tc.policy
				}
				manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: policy, SecretBinding: binding, Value: tc.value}}
				secret := &corev1.Secret{
					Name:      "tokens",
					Namespace: system.DefaultNamespace,
					Labels:    map[string]string{testSecretBindingAllowedLabel: "true"},
					Data:      map[string][]byte{"token": []byte("secret-value")},
				}
				if tc.unlabeled {
					secret.Labels = nil
				}
				if tc.empty {
					secret.Data["token"] = nil
				}
				if tc.missingKey {
					secret.Data = nil
				}
				if tc.missing {
					secret.Name = "other"
				}
				body, err := json.Marshal(manifest)
				require.NoError(t, err)
				request := httptest.NewRequest(http.MethodPost, "/api/vmcps"+query, bytes.NewReader(body))
				request.SetPathValue("vmcp_id", id)
				recorder := httptest.NewRecorder()
				ctx := api.Context{
					Request:        request,
					ResponseWriter: recorder,
					Storage:        storage,
					GatewayClient:  gatewayClient,
					User:           u,
					LocalK8sClient: newCreateServerSecretBindingK8sClient(t, secret),
					ObotNamespace:  system.DefaultNamespace,
				}
				if tc.wantError != "" {
					storage.onCreate = func(kclient.Object) { t.Fatal("invalid binding created a resource") }
					storage.onUpdate = func(kclient.Object) { t.Fatal("invalid binding updated a resource") }
				}
				if operation == "create" {
					err = handler.Create(ctx)
				} else {
					err = handler.Update(ctx)
				}
				if tc.wantError != "" {
					var httpErr *types.ErrHTTP
					require.ErrorAs(t, err, &httpErr)
					require.Equal(t, http.StatusBadRequest, httpErr.Code)
					require.Contains(t, httpErr.Message, tc.wantError)
					require.NotContains(t, httpErr.Message, "secret-value")
					return
				}
				require.NoError(t, err)
				require.NotContains(t, recorder.Body.String(), "secret-value")
				var response types.VMCP
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
				require.Equal(t, binding, response.Components[0].Configuration[0].SecretBinding)
				var stored v1.VMCP
				require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: response.ID}, &stored))
				require.Equal(t, binding, stored.Spec.Manifest.Components[0].Configuration[0].SecretBinding)
				credential, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.StaticConfigurationCredentialContext(response.ID)}, vmcpconfig.StaticConfigurationCredentialName(&stored))
				require.NoError(t, err)
				require.Empty(t, credential.Secrets, "binding must remove any previously saved literal")
				config := vmcpconfig.ComponentConfig(response.Components[0])
				require.Equal(t, binding, config[0].SecretBinding)
			})
		}
	}
}

func TestConvertSyncedVMCPPreservesSecretBinding(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "gitlab-secret", Key: "gitlab_key"}
	vmcp := v1.VMCP{Spec: v1.VMCPSpec{
		SourceURL: "https://example.com/catalog.yaml",
		Manifest:  testVMCPManifest(),
	}}
	vmcp.Spec.Manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{
		Key:           "TOKEN",
		Policy:        types.VMCPConfigurationPolicyFixed,
		SecretBinding: binding,
	}}
	require.Equal(t, binding, convertVMCP(vmcp).Components[0].Configuration[0].SecretBinding)
}
