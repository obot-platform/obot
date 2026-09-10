package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	ssservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVMCPConsentTargetUsesSelectedInstance(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: system.DefaultNamespace}
	oldest := &v1.VMCPInstance{
		Name:              "vmcpi1oldest",
		Namespace:         system.DefaultNamespace,
		CreationTimestamp: metav1.Unix(1, 0),
		Spec:              v1.VMCPInstanceSpec{UserID: "user", LegacySlug: "ms1oldest", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
	}
	selected := &v1.VMCPInstance{
		Name:              "vmcpi1selected",
		Namespace:         system.DefaultNamespace,
		CreationTimestamp: metav1.Unix(2, 0),
		Spec:              v1.VMCPInstanceSpec{UserID: "user", LegacySlug: "ms1selected", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
	}
	storage := vmcpConsentStorage(vmcp, oldest, selected)
	req := vmcpConsentRequest(storage, nil)

	for _, connectID := range []string{selected.Name, selected.Spec.LegacySlug} {
		_, instance, err := vmcpConsentTarget(req, connectID)
		require.NoError(t, err)
		require.NotNil(t, instance)
		require.Equal(t, selected.Name, instance.Name)
	}
}

func TestPrepareOAuthConsentRequiresVMCPConfigurationBeforeOAuth(t *testing.T) {
	vmcp, instance := vmcpConsentConfigurationTarget()
	authRequest := &v1.OAuthAuthRequest{
		Name:      "oar1request",
		Namespace: system.DefaultNamespace,
		Spec:      v1.OAuthAuthRequestSpec{MCPID: instance.Name},
	}
	storage := vmcpConsentStorage(vmcp, instance, authRequest)
	req := vmcpConsentRequest(storage, vmcpConsentGateway(t))

	// A nil oauthChecker panics if prepareOAuthConsent reaches an OAuth probe.
	require.NoError(t, (&handler{}).prepareOAuthConsent(req, authRequest))
	require.True(t, authRequest.Spec.ConsentPrepared)
	require.True(t, authRequest.Spec.ConsentMCPConfigRequired)
	require.False(t, authRequest.Spec.ConsentMCPAuthRequired)

	var stored v1.OAuthAuthRequest
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(authRequest), &stored))
	require.True(t, stored.Spec.ConsentMCPConfigRequired)
}

func TestVMCPConsentMissingConfigurationAllowsSavedRequiredAndOptionalEmpty(t *testing.T) {
	vmcp, instance := vmcpConsentConfigurationTarget()
	gatewayClient := vmcpConsentGateway(t)
	req := vmcpConsentRequest(vmcpConsentStorage(vmcp, instance), gatewayClient)

	missing, err := vmcpConsentMissingConfiguration(req, *vmcp, *instance)
	require.NoError(t, err)
	require.Equal(t, []string{vmcpconfig.ConfigurationKey("component", "REQUIRED")}, missing)

	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey("component", "REQUIRED"): "saved"},
	}))
	missing, err = vmcpConsentMissingConfiguration(req, *vmcp, *instance)
	require.NoError(t, err)
	require.Empty(t, missing)
}

func vmcpConsentConfigurationTarget() (*v1.VMCP, *v1.VMCPInstance) {
	return &v1.VMCP{
		Name:      "vmcp1shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
			ID: "component",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{
				{Key: "REQUIRED", Required: true},
				{Key: "OPTIONAL"},
			}}},
			Configuration: []types.VMCPConfigurationPolicy{
				{Key: "REQUIRED", Policy: types.VMCPConfigurationPolicyUserAllowed},
				{Key: "OPTIONAL", Policy: types.VMCPConfigurationPolicyUserAllowed},
			},
		}}}},
	}, &v1.VMCPInstance{
		Name:      "vmcpi1selected",
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPInstanceSpec{UserID: "user", Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1shared"}},
	}
}

func vmcpConsentStorage(objects ...kclient.Object) storage.Client {
	return clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug} }).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		Build()
}

func vmcpConsentRequest(storage storage.Client, gatewayClient *gatewayclient.Client) api.Context {
	return api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodGet, "/oauth/consent", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           &user.DefaultInfo{Name: "user", UID: "user", Groups: []string{types.GroupAuthenticated}},
	}
}

func vmcpConsentGateway(t *testing.T) *gatewayclient.Client {
	t.Helper()
	services, err := ssservices.New(ssservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}
