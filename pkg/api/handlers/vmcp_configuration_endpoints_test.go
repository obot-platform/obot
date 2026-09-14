package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVMCPRevealReturnsOnlyFixedConfiguration(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-reveal",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{
			{ID: "fixed", Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}}},
			{ID: "allowed", Configuration: []types.VMCPConfigurationPolicy{{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed}}},
		}}},
	}
	tests := []struct {
		name    string
		secrets map[string]string
		want    types.VMCPConfiguration
	}{
		{
			name: "configured",
			secrets: map[string]string{
				vmcpconfig.ConfigurationKey("fixed", "TOKEN"):    "fixed-value",
				vmcpconfig.ConfigurationKey("allowed", "HEADER"): "user-value",
				vmcpconfig.ConfigurationKey("removed", "VALUE"):  "removed-value",
			},
			want: types.VMCPConfiguration{Components: map[string]map[string]string{
				"fixed": {"TOKEN": "fixed-value"},
			}},
		},
		{
			name: "missing credential",
			want: types.VMCPConfiguration{Components: map[string]map[string]string{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient := newHandlerTestGateway(t)
			if tt.secrets != nil {
				require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
					Name:    vmcpconfig.ConfigurationCredentialName(),
					Secrets: tt.secrets,
				}))
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/reveal", nil)
			request.SetPathValue("vmcp_id", vmcp.Name)
			require.NoError(t, NewVMCPHandler(nil).Reveal(api.Context{
				ResponseWriter: recorder,
				Request:        request,
				Storage:        newVMCPTestStorage(vmcp),
				GatewayClient:  gatewayClient,
			}))
			var got types.VMCPConfiguration
			require.NoError(t, json.NewDecoder(recorder.Body).Decode(&got))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVMCPDeconfigureDeletesConfigurationAndPublishesHashes(t *testing.T) {
	component := types.VMCPComponent{ID: "component", Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}}}
	vmcp := &v1.VMCP{Name: "vmcp-deconfigure", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}}}}
	secrets := map[string]string{vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): "secret"}
	vmcpconfig.SetStaticConfigurationHashes(vmcp, secrets)
	storage := newVMCPTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: secrets,
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/deconfigure", nil)
	request.SetPathValue("vmcp_id", vmcp.Name)
	require.NoError(t, NewVMCPHandler(nil).Deconfigure(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}))

	_, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)}, vmcpconfig.ConfigurationCredentialName())
	var notFound gateway.CredentialNotFoundError
	require.ErrorAs(t, err, &notFound)
	var updated v1.VMCP
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &updated))
	emptyHash := utils.Digest(map[string]string{})
	assert.Equal(t, emptyHash, updated.Spec.StaticConfigurationHash)
	assert.Equal(t, emptyHash, updated.Spec.ComponentStaticConfigurationHashes[component.ID])
}

func TestVMCPInstanceDeconfigureDeletesConfigurationAndTriggersSync(t *testing.T) {
	instance := &v1.VMCPInstance{
		Name:        "vmcpi-deconfigure",
		Namespace:   system.DefaultNamespace,
		Annotations: map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: "old"},
	}
	storage := newVMCPTestStorage(instance)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{"secret": "value"},
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+instance.Name+"/deconfigure", nil)
	request.SetPathValue("vmcp_instance_id", instance.Name)
	require.NoError(t, NewVMCPInstanceHandler().Deconfigure(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}))

	_, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	var notFound gateway.CredentialNotFoundError
	require.ErrorAs(t, err, &notFound)
	var updated v1.VMCPInstance
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &updated))
	assert.Equal(t, utils.Digest(map[string]string{}), updated.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation])
}

// TestVMCPInstanceConfigurationSurvivesConcurrentInstanceWrite covers the
// VMCPInstance controllers writing to an instance while its owner is
// configuring it, which used to surface as an intermittent 409.
func TestVMCPInstanceConfigurationSurvivesConcurrentInstanceWrite(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-conflict",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
			ID:            "component-id",
			Name:          "component",
			Configuration: []types.VMCPConfigurationPolicy{{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed}},
		}}}},
	}
	tests := []struct {
		name     string
		endpoint string
		body     string
		call     func(api.Context) error
		want     map[string]string
	}{
		{
			name:     "configure",
			endpoint: "configure",
			body:     `{"components":{"component-id":{"HEADER":"user-secret"}}}`,
			call:     NewVMCPInstanceHandler().Configure,
			want:     map[string]string{vmcpconfig.ConfigurationKey("component-id", "HEADER"): "user-secret"},
		},
		{
			name:     "deconfigure",
			endpoint: "deconfigure",
			body:     "",
			call:     NewVMCPInstanceHandler().Deconfigure,
			want:     map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &v1.VMCPInstance{
				Name:      "vmcpi-conflict",
				Namespace: system.DefaultNamespace,
				Spec: v1.VMCPInstanceSpec{
					UserID:   "user-1",
					Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
				},
			}
			storage := newVMCPTestStorage(vmcp.DeepCopy(), instance)
			gatewayClient := newHandlerTestGateway(t)
			require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
				Name:    vmcpconfig.ConfigurationCredentialName(),
				Secrets: map[string]string{"stale": "value"},
			}))

			// Stand in for a controller reconciling the instance between the
			// handler's read and its write, bumping the ResourceVersion.
			var raced bool
			storage.onUpdate = func(kclient.Object) {
				if raced {
					return
				}
				raced = true
				var current v1.VMCPInstance
				require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &current))
				current.Status.ConfigurationCheckHash = "reconciled"
				require.NoError(t, storage.WithWatch.Update(t.Context(), &current))
			}

			request := httptest.NewRequest(http.MethodPost,
				"/api/vmcp-instances/"+instance.Name+"/"+tt.endpoint, strings.NewReader(tt.body))
			request.SetPathValue("vmcp_instance_id", instance.Name)
			require.NoError(t, tt.call(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        request,
				Storage:        storage,
				GatewayClient:  gatewayClient,
				User:           &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}},
			}))
			require.True(t, raced, "concurrent instance write never happened")

			var updated v1.VMCPInstance
			require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &updated))
			assert.Equal(t, utils.Digest(tt.want), updated.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation])
			assert.Equal(t, "reconciled", updated.Status.ConfigurationCheckHash, "concurrent controller write was clobbered")
		})
	}
}

// TestSyncInstanceConfigurationHash covers the annotation landing on the
// credential that is actually stored when another request replaces it between
// this request's read and its write.
func TestSyncInstanceConfigurationHash(t *testing.T) {
	written := map[string]string{vmcpconfig.ConfigurationKey("component-id", "HEADER"): "written"}
	raced := map[string]string{vmcpconfig.ConfigurationKey("component-id", "HEADER"): "raced"}

	tests := []struct {
		name string
		// knownHash is the annotation as this request read it, before it stored
		// its own credential.
		knownHash string
		// storedHash and storedSecrets are what another request left behind. A
		// nil storedSecrets means the credential was deleted.
		storedHash    string
		storedSecrets map[string]string
		want          string
	}{
		{
			name:          "uncontended write records this request's credential",
			knownHash:     "before",
			storedHash:    "before",
			storedSecrets: written,
			want:          utils.Digest(written),
		},
		{
			name:          "concurrent write that already recorded its credential is left alone",
			knownHash:     "before",
			storedHash:    utils.Digest(raced),
			storedSecrets: raced,
			want:          utils.Digest(raced),
		},
		{
			name:          "stale annotation is corrected to the stored credential",
			knownHash:     "before",
			storedHash:    "stale",
			storedSecrets: raced,
			want:          utils.Digest(raced),
		},
		{
			name:          "concurrent deconfigure records the empty digest",
			knownHash:     "before",
			storedHash:    "stale",
			storedSecrets: nil,
			want:          utils.Digest(map[string]string{}),
		},
		{
			// The reported bug was hit on a retry, so a request landing on an
			// annotation that already describes what it stored is routine.
			name:          "resubmitting the same configuration converges",
			knownHash:     "before",
			storedHash:    utils.Digest(written),
			storedSecrets: written,
			want:          utils.Digest(written),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := &v1.VMCPInstance{
				Name:        "vmcpi-sync",
				Namespace:   system.DefaultNamespace,
				Annotations: map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: tt.storedHash},
			}
			storage := newVMCPTestStorage(stored)
			gatewayClient := newHandlerTestGateway(t)
			if tt.storedSecrets != nil {
				require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: vmcpconfig.InstanceConfigurationCredentialContext(stored.Name),
					Name:    vmcpconfig.ConfigurationCredentialName(),
					Secrets: tt.storedSecrets,
				}))
			}

			// The instance as this request read it, before the concurrent write.
			instance := v1.VMCPInstance{
				Name:        stored.Name,
				Namespace:   stored.Namespace,
				Annotations: map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: tt.knownHash},
			}
			require.NoError(t, syncInstanceConfigurationHash(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+stored.Name+"/configure", nil),
				Storage:        storage,
				GatewayClient:  gatewayClient,
			}, &instance, written))

			var updated v1.VMCPInstance
			require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(stored), &updated))
			assert.Equal(t, tt.want, updated.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation])
			assert.Equal(t, tt.want, instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation],
				"instance returned to the caller disagrees with storage")
		})
	}
}

// TestSyncInstanceConfigurationHashRetriesAfterCorrection covers a conflict
// landing after the annotation was already corrected to the credential another
// request stored, which carries state across retry passes.
func TestSyncInstanceConfigurationHashRetriesAfterCorrection(t *testing.T) {
	written := map[string]string{vmcpconfig.ConfigurationKey("component-id", "HEADER"): "written"}
	raced := map[string]string{vmcpconfig.ConfigurationKey("component-id", "HEADER"): "raced"}

	stored := &v1.VMCPInstance{
		Name:        "vmcpi-retry",
		Namespace:   system.DefaultNamespace,
		Annotations: map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: "stale"},
	}
	storage := newVMCPTestStorage(stored)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(stored.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: raced,
	}))

	// Conflict once, after the first pass has already resolved which credential
	// is stored, so the second pass runs with the corrected digest carried over.
	var conflicted bool
	storage.onUpdate = func(kclient.Object) {
		if conflicted {
			return
		}
		conflicted = true
		var current v1.VMCPInstance
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(stored), &current))
		current.Status.ConfigurationCheckHash = "reconciled"
		require.NoError(t, storage.WithWatch.Update(t.Context(), &current))
	}

	instance := v1.VMCPInstance{
		Name:        stored.Name,
		Namespace:   stored.Namespace,
		Annotations: map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: "before"},
	}
	require.NoError(t, syncInstanceConfigurationHash(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+stored.Name+"/configure", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}, &instance, written))
	require.True(t, conflicted, "the conflicting write never happened")

	var updated v1.VMCPInstance
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(stored), &updated))
	assert.Equal(t, utils.Digest(raced), updated.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation])
	assert.Equal(t, "reconciled", updated.Status.ConfigurationCheckHash, "concurrent write was clobbered")
}
