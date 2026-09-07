package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageServices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLaunchServerVMCPUsesDeclaredComponentsOnly(t *testing.T) {
	const (
		vmcpID      = "vmcp1launch"
		userID      = "user-launch"
		componentID = "component-a"
	)
	vmcp, instance, component, unrelated := vmcpActionObjects(vmcpID, userID, componentID, false)
	manager, storage, _ := newVMCPActionSessionManager(t, vmcp, instance, component, unrelated)

	req := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcpID+"/launch", nil)
	req.SetPathValue("vmcp_id", vmcpID)
	err := (&MCPHandler{mcpSessionManager: manager}).LaunchServer(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storage,
		User:           testUser(userID),
	})
	require.NoError(t, err)
}

func TestCheckOAuthVMCPChecksDeclaredRemoteComponents(t *testing.T) {
	const (
		vmcpID      = "vmcp1oauth"
		userID      = "user-oauth"
		componentID = "component-a"
	)
	vmcp, instance, component, unrelated := vmcpActionObjects(vmcpID, userID, componentID, true)
	manager, storage, _ := newVMCPActionSessionManager(t, vmcp, instance, component, unrelated)

	req := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcpID+"/check-oauth", nil)
	req.SetPathValue("vmcp_id", vmcpID)
	recorder := httptest.NewRecorder()
	err := (&MCPHandler{mcpSessionManager: manager}).CheckOAuth(api.Context{
		ResponseWriter: recorder,
		Request:        req,
		Storage:        storage,
		User:           testUser(userID),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusPreconditionFailed, recorder.Code)
}

func TestGetOAuthURLVMCPResolvesAggregateConfiguration(t *testing.T) {
	const (
		vmcpID      = "vmcp1oauth-url"
		userID      = "user-oauth-url"
		componentID = "component-a"
	)
	vmcp, instance, component, unrelated := vmcpActionObjects(vmcpID, userID, componentID, false)
	manager, storage, _ := newVMCPActionSessionManager(t, vmcp, instance, component, unrelated)
	checker := newRecordingMCPAuthChecker("https://oauth.example/authorize")

	req := httptest.NewRequest(http.MethodGet, "/api/vmcps/"+vmcpID+"/oauth-url", nil)
	req.SetPathValue("vmcp_id", vmcpID)
	recorder := httptest.NewRecorder()
	err := (&MCPHandler{
		mcpSessionManager: manager,
		mcpOAuthChecker:   checker,
	}).GetOAuthURL(api.Context{
		ResponseWriter: recorder,
		Request:        req,
		Storage:        storage,
		User:           testUser(userID),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, vmcpID, checker.server.Name)
	assert.Equal(t, types.RuntimeVMCP, checker.server.Spec.Manifest.Runtime)
	assert.Equal(t, types.RuntimeVMCP, checker.config.Runtime)
	assert.Len(t, checker.config.Components, 1)
	assert.Equal(t, component.Name, checker.config.Components[0].Name)

	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "https://oauth.example/authorize", response["oauthURL"])
}

func TestClearOAuthCredentialsVMCPUsesDeclaredRemoteComponents(t *testing.T) {
	const (
		vmcpID      = "vmcp1clear-oauth"
		userID      = "user-clear-oauth"
		componentID = "component-a"
	)
	vmcp, instance, component, unrelated := vmcpActionObjects(vmcpID, userID, componentID, false)
	manager, storage, gatewayClient := newVMCPActionSessionManager(t, vmcp, instance, component, unrelated)
	trigger := &recordingMCPServerTrigger{}

	req := httptest.NewRequest(http.MethodDelete, "/api/vmcps/"+vmcpID+"/oauth", nil)
	req.SetPathValue("vmcp_id", vmcpID)
	recorder := httptest.NewRecorder()
	err := (&MCPHandler{
		mcpSessionManager: manager,
		controllerBackend: trigger,
	}).ClearOAuthCredentials(api.Context{
		ResponseWriter: recorder,
		Request:        req,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           testUser(userID),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, []string{component.Name}, trigger.keys)
}

func TestAggregateComponentServersForActionVMCPUsesServerConfigComponents(t *testing.T) {
	declared := &v1.MCPServer{
		ObjectMeta: objectMetaForVMCPActionTest("ms1declared"),
	}
	unrelated := &v1.MCPServer{
		ObjectMeta: objectMetaForVMCPActionTest("ms1unrelated"),
	}
	storage := clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(declared, unrelated).
		Build()

	server := v1.MCPServer{
		ObjectMeta: objectMetaForVMCPActionTest("vmcp1aggregate"),
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{Runtime: types.RuntimeVMCP},
		},
	}
	serverConfig := mcp.ServerConfig{
		Runtime:    types.RuntimeVMCP,
		Components: []mcp.ComponentServer{{Name: declared.Name}},
	}

	components, err := (&MCPHandler{}).aggregateComponentServersForAction(
		api.Context{
			Request: httptest.NewRequest(http.MethodGet, "/", nil),
			Storage: storage,
		},
		server,
		serverConfig,
	)
	require.NoError(t, err)
	require.Len(t, components, 1)
	assert.Equal(t, declared.Name, components[0].Name)
}

type recordingMCPAuthChecker struct {
	server   v1.MCPServer
	config   mcp.ServerConfig
	oauthURL string
}

func newRecordingMCPAuthChecker(oauthURL string) *recordingMCPAuthChecker {
	return &recordingMCPAuthChecker{oauthURL: oauthURL}
}

func (c *recordingMCPAuthChecker) CheckForMCPAuth(_ api.Context, server v1.MCPServer, config mcp.ServerConfig, _, _, _ string) (string, error) {
	c.server = server
	c.config = config
	return c.oauthURL, nil
}

func vmcpActionObjects(vmcpID, userID, componentID string, staticOAuth bool) (*v1.VMCP, *v1.VMCPInstance, *v1.MCPServer, *v1.MCPServer) {
	vmcp := &v1.VMCP{
		ObjectMeta: objectMetaForVMCPActionTest(vmcpID),
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				ForceSingleUser: true,
				DisplayName:     "Test vMCP",
				Components: []types.VMCPComponent{{
					ID:                      componentID,
					Name:                    "component-a",
					MCPCatalogID:            system.DefaultCatalog,
					MCPServerCatalogEntryID: "entry-a",
				}},
			},
		},
	}
	instance := &v1.VMCPInstance{
		ObjectMeta: objectMetaForVMCPActionTest(system.VMCPInstancePrefix + "action-instance"),
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcpID},
			UserID:   userID,
		},
	}
	remoteConfig := &types.RemoteRuntimeConfig{URL: "https://component.example.com/mcp"}
	if staticOAuth {
		remoteConfig.StaticOAuthRequired = true
	}
	component := &v1.MCPServer{
		ObjectMeta: objectMetaForVMCPActionTest("ms1component-a"),
		Spec: v1.MCPServerSpec{
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: componentID,
			Manifest: types.MCPServerManifest{
				Name:         "component-a",
				Runtime:      types.RuntimeRemote,
				RemoteConfig: remoteConfig,
			},
		},
	}
	// This server shares the instance but is not declared by the vMCP. Its
	// invalid runtime configuration makes accidental list-all behavior fail
	// loudly.
	unrelated := &v1.MCPServer{
		ObjectMeta: objectMetaForVMCPActionTest("ms1unrelated"),
		Spec: v1.MCPServerSpec{
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: "unrelated",
			Manifest: types.MCPServerManifest{
				Name:      "unrelated",
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{},
			},
		},
	}
	return vmcp, instance, component, unrelated
}

type recordingMCPServerTrigger struct {
	keys []string
}

func (r *recordingMCPServerTrigger) Trigger(_ context.Context, _ schema.GroupVersionKind, key string, _ time.Duration) error {
	r.keys = append(r.keys, key)
	return nil
}

func newVMCPActionSessionManager(t *testing.T, objects ...kclient.Object) (*mcp.SessionManager, kclient.WithWatch, *gatewayclient.Client) {
	t.Helper()

	storageClient := &vmcpActionInitialEventsClient{WithWatch: clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.VMCPInstance).Spec.UserID}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string {
			server := obj.(*v1.MCPServer)
			if server.Spec.VMCPInstanceID == "" {
				return nil
			}
			return []string{server.Spec.VMCPInstanceID}
		}).
		WithObjects(objects...).
		Build()}

	services, err := storageServices.New(storageServices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	gatewayDatabase, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, gatewayDatabase.AutoMigrate())
	gatewayClient := gatewayclient.New(t.Context(), gatewayDatabase, storageClient, nil, nil, nil, nil, time.Hour, 100, 90, 90, 90, false)
	t.Cleanup(func() { require.NoError(t, gatewayClient.Close()) })

	manager, err := mcp.NewSessionManager(
		t.Context(),
		false,
		nil,
		nil,
		"https://obot.example.com",
		8080,
		mcp.Options{
			MCPNamespace:      "obot-mcp",
			MCPRuntimeBackend: mcp.RuntimeBackendKubernetes,
		},
		nil,
		&rest.Config{Host: "https://127.0.0.1"},
		storageClient,
		storageClient,
		storageClient,
		gatewayClient,
		system.DefaultNamespace,
		nil,
	)
	require.NoError(t, err)
	return manager, storageClient, gatewayClient
}

// vmcpActionInitialEventsClient supplies the watch-list behavior that the
// Kubernetes API server provides but controller-runtime's fake watch omits.
type vmcpActionInitialEventsClient struct {
	kclient.WithWatch
}

func (c *vmcpActionInitialEventsClient) Watch(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) (watch.Interface, error) {
	upstream, err := c.WithWatch.Watch(ctx, list, opts...)
	if err != nil {
		return nil, err
	}

	initialList := list.DeepCopyObject().(kclient.ObjectList)
	listOptions := &kclient.ListOptions{}
	listOptions.ApplyOptions(opts)
	listOptions.Raw = nil
	if err := c.WithWatch.List(ctx, initialList, listOptions); err != nil {
		upstream.Stop()
		return nil, err
	}
	objects, err := meta.ExtractList(initialList)
	if err != nil {
		upstream.Stop()
		return nil, err
	}

	result := make(chan watch.Event)
	proxy := watch.NewProxyWatcher(result)
	go func() {
		defer close(result)
		defer upstream.Stop()
		for _, object := range objects {
			select {
			case result <- watch.Event{Type: watch.Added, Object: object}:
			case <-proxy.StopChan():
				return
			}
		}
		for {
			select {
			case event, ok := <-upstream.ResultChan():
				if !ok {
					return
				}
				select {
				case result <- event:
				case <-proxy.StopChan():
					return
				}
			case <-proxy.StopChan():
				return
			}
		}
	}()
	return proxy, nil
}

func objectMetaForVMCPActionTest(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace}
}

func TestSharedVMCPServerUsesOneScopeAndPerUserHeaders(t *testing.T) {
	vmcp, first, server, _ := vmcpActionObjects("vmcp1shared", "1", "one", false)
	vmcp.Spec.Manifest.ForceSingleUser = false
	header := types.MCPHeader{Key: "TOKEN", Name: "X-Token", Required: true}
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{Headers: []types.MCPHeader{header}}
	vmcp.Spec.Manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyUserAllowed}}
	server.Spec.VMCPInstanceID = ""
	server.Spec.VMCPID = vmcp.Name
	server.Spec.Manifest.RemoteConfig.Headers = []types.MCPHeader{header}
	second := first.DeepCopy()
	second.Name = "vmcpi1second"
	second.Spec.UserID = "2"
	manager, storage, credentials := newVMCPActionSessionManager(t, vmcp, first, second, server)
	for _, instance := range []*v1.VMCPInstance{first, second} {
		require.NoError(t, credentials.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
			Name:    vmcpconfig.ConfigurationCredentialName(),
			Secrets: map[string]string{vmcpconfig.ConfigurationKey("one", "TOKEN"): "token-" + instance.Spec.UserID},
		}))
	}
	var previousScope string
	for _, userID := range []string{"1", "2", "1"} {
		id, resolved, cfg, err := manager.ServerForActionWithConnectID(t.Context(), server.Name, userID)
		require.NoError(t, err)
		require.Equal(t, server.Name, id)
		require.Equal(t, server.Name, resolved.Name)
		require.Contains(t, cfg.Headers, "TOKEN=token-"+userID)
		if previousScope != "" {
			require.Equal(t, previousScope, cfg.Scope)
		}
		previousScope = cfg.Scope
	}
	var legacyInstances v1.MCPServerInstanceList
	require.NoError(t, storage.List(t.Context(), &legacyInstances))
	require.Empty(t, legacyInstances.Items)
}
