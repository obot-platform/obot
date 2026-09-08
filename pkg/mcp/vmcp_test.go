package mcp

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/watch"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	vmcpTestListenPort = 18080
)

type vmcpWatchSignalingStorage struct {
	storage.Client
	once     sync.Once
	watching chan struct{}
}

// vmcpInitialEventsStorage supplies the watch-list behavior that the Kubernetes
// API server provides but controller-runtime's fake watch does not emulate.
type vmcpInitialEventsStorage struct {
	storage.Client
}

func TestRestrictComponentTools(t *testing.T) {
	renamed := types.ToolOverride{
		Name:                "find",
		OverrideName:        "search",
		OverrideDescription: "Search documents",
		Enabled:             true,
	}
	for _, tt := range []struct {
		name        string
		component   ComponentServer
		allowed     []types.VMCPToolReference
		componentID string
		want        []types.ToolOverride
		disabled    bool
	}{
		{
			name:        "component wildcard preserves enabled and disabled overrides",
			component:   ComponentServer{Tools: []types.ToolOverride{renamed, {Name: "delete", Enabled: false}}},
			componentID: "docs",
			allowed:     []types.VMCPToolReference{{ComponentID: "docs", Name: "*"}},
			want:        []types.ToolOverride{renamed, {Name: "delete", Enabled: false}},
		},
		{
			name:        "component wildcard without overrides stays unrestricted",
			componentID: "docs",
			allowed:     []types.VMCPToolReference{{ComponentID: "docs", Name: "*"}},
		},
		{
			name:        "wildcard on another component grants nothing",
			componentID: "docs",
			allowed:     []types.VMCPToolReference{{ComponentID: "other", Name: "*"}},
			disabled:    true,
		},
		{
			name:      "all preserves overrides",
			component: ComponentServer{Tools: []types.ToolOverride{renamed}},
			want:      []types.ToolOverride{renamed},
		},
		{
			name:     "empty grant disables tools",
			allowed:  []types.VMCPToolReference{},
			disabled: true,
		},
		{
			name:        "grants original name despite prefix and rename",
			component:   ComponentServer{ToolPrefix: "docs", Tools: []types.ToolOverride{renamed, {Name: "delete", Enabled: false}}},
			componentID: "docs-component",
			allowed:     []types.VMCPToolReference{{ComponentID: "docs-component", Name: "find"}, {ComponentID: "docs-component", Name: "delete"}},
			want:        []types.ToolOverride{renamed},
		},
		{
			name:        "same named tool on another component does not grant access",
			component:   ComponentServer{Tools: []types.ToolOverride{renamed}},
			componentID: "docs-component",
			allowed:     []types.VMCPToolReference{{ComponentID: "other-component", Name: "find"}},
			disabled:    true,
		},
		{
			name:        "creates override for granted upstream tool",
			componentID: "docs-component",
			allowed:     []types.VMCPToolReference{{ComponentID: "docs-component", Name: "find"}},
			want:        []types.ToolOverride{{Name: "find", Enabled: true}},
		},
		{
			name:        "wrong component has no grant",
			componentID: "docs-component",
			allowed:     []types.VMCPToolReference{{ComponentID: "files-component", Name: "find"}},
			disabled:    true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := restrictComponentTools(&tt.component, tt.allowed, tt.componentID); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tt.component.Tools, tt.want) || tt.component.DisableTools != tt.disabled {
				t.Fatalf("got tools %#v, disabled %v; want %#v, disabled %v", tt.component.Tools, tt.component.DisableTools, tt.want, tt.disabled)
			}
		})
	}
}

func TestServerConfigForVMCPBuildsAggregateConfig(t *testing.T) {
	const (
		vmcpID = "vmcp1shared"
		userID = "user-1"
	)
	instanceID := "vmcpi1-shared-instance"
	otherInstanceID := "vmcpi1-other-user-instance"

	vmcp := &v1.VMCP{
		Name:      vmcpID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				DisplayName:     "Shared VMCP",
				ForceSingleUser: true,
				Profiles: []types.VMCPProfile{
					{
						Subjects:     []types.Subject{{Type: types.SubjectTypeUser, ID: userID}},
						AllowedTools: types.VMCPToolSet{"search-component": []string{"find"}},
					},
					{
						Subjects:     []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
						AllowedTools: types.VMCPToolSet{"files-component": []string{"read"}},
					},
				},
				Components: []types.VMCPComponent{
					{
						ID:   "search-component",
						Name: "search",
						CatalogEntry: types.MCPServerCatalogEntrySnapshot{
							Manifest: types.MCPServerCatalogEntryManifest{
								Name:    "Cached Search",
								Runtime: types.RuntimeRemote,
							},
						},
						ToolPrefix: "search",
						ToolOverrides: []types.ToolOverride{
							{
								Name:                "find",
								OverrideName:        "find_documents",
								Description:         "Find a document",
								OverrideDescription: "Search the document index",
								Enabled:             true,
							},
						},
					},
					{
						ID:   "files-component",
						Name: "files",
						CatalogEntry: types.MCPServerCatalogEntrySnapshot{
							Manifest: types.MCPServerCatalogEntryManifest{
								Name:    "Cached Files",
								Runtime: types.RuntimeRemote,
							},
						},
						ToolPrefix: "files",
					},
				},
			},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      instanceID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID:       vmcpID,
				EnabledTools: types.VMCPToolSet{"search-component": []string{"find", "not_granted"}},
			},
			UserID: userID,
		},
	}
	otherUserInstance := &v1.VMCPInstance{
		Name:      otherInstanceID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcpID},
			UserID:   "user-2",
		},
	}
	otherVMCPInstance := &v1.VMCPInstance{
		Name:      "vmcpi1-other-vmcp-instance",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1other"},
			UserID:   userID,
		},
	}

	searchServer := vmcpComponentServer(
		"search-server",
		instanceID,
		userID,
		"search-component",
		"Search service",
		"https://search.example.test/mcp",
	)
	filesServer := vmcpComponentServer(
		"files-server",
		instanceID,
		userID,
		"files-component",
		"Files service",
		"https://files.example.test/mcp",
	)
	otherUserServer := vmcpComponentServer(
		"aaa-search-server-other-user",
		otherInstanceID,
		"user-2",
		"search-component",
		"Wrong user service",
		"https://wrong-user.example.test/mcp",
	)

	storageClient := newVMCPTestStorage(vmcp, instance, otherUserInstance, otherVMCPInstance, searchServer, filesServer, otherUserServer)
	manager := &SessionManager{
		storageClient:  storageClient,
		httpListenPort: vmcpTestListenPort,
	}

	serverConfig, err := manager.ServerConfigForVMCP(t.Context(), vmcpID, userID)
	if err != nil {
		t.Fatalf("ServerConfigForVMCP() error = %v", err)
	}
	if serverConfig.Runtime != types.RuntimeVMCP {
		t.Fatalf("vMCP runtime = %q, want %q", serverConfig.Runtime, types.RuntimeVMCP)
	}
	if serverConfig.MCPServerName != vmcpID {
		t.Fatalf("vMCP MCP server name = %q, want %q", serverConfig.MCPServerName, vmcpID)
	}
	if serverConfig.MCPServerDisplayName != vmcp.Spec.Manifest.DisplayName {
		t.Fatalf("vMCP display name = %q, want %q", serverConfig.MCPServerDisplayName, vmcp.Spec.Manifest.DisplayName)
	}
	if serverConfig.AuditLogMetadata["mcpID"] != vmcpID || serverConfig.AuditLogMetadata["mcpServerDisplayName"] != vmcp.Spec.Manifest.DisplayName || serverConfig.AuditLogMetadata["userID"] != userID {
		t.Fatalf("missing vMCP audit attribution: %#v", serverConfig.AuditLogMetadata)
	}
	if len(serverConfig.Components) != 2 {
		t.Fatalf("component count = %d, want 2", len(serverConfig.Components))
	}

	components := make(map[string]ComponentServer, len(serverConfig.Components))
	for _, component := range serverConfig.Components {
		components[component.Name] = component
	}
	searchComponent, ok := components[searchServer.Name]
	if !ok {
		t.Fatalf("vMCP config omitted component %q: %#v", searchServer.Name, serverConfig.Components)
	}
	if searchComponent.DisplayName != "search" {
		t.Fatalf("search component display name = %q, want search", searchComponent.DisplayName)
	}
	wantSearchURL := system.LocalMCPConnectURL(searchServer.Name, vmcpTestListenPort)
	if searchComponent.URL != wantSearchURL {
		t.Fatalf("search component URL = %q, want %q", searchComponent.URL, wantSearchURL)
	}
	if searchComponent.ToolPrefix != "search" {
		t.Fatalf("search component tool prefix = %q, want search", searchComponent.ToolPrefix)
	}
	wantTools := []types.ToolOverride{
		{
			Name:                "find",
			OverrideName:        "find_documents",
			Description:         "Find a document",
			OverrideDescription: "Search the document index",
			Enabled:             true,
		},
	}
	if !reflect.DeepEqual(searchComponent.Tools, wantTools) {
		t.Fatalf("search component tool overrides = %#v, want %#v", searchComponent.Tools, wantTools)
	}

	filesComponent, ok := components[filesServer.Name]
	if !ok {
		t.Fatalf("vMCP config omitted component %q: %#v", filesServer.Name, serverConfig.Components)
	}
	wantFilesURL := system.LocalMCPConnectURL(filesServer.Name, vmcpTestListenPort)
	if filesComponent.DisplayName != "files" || filesComponent.URL != wantFilesURL {
		t.Fatalf("files component metadata = %#v, want display name and local URL", filesComponent)
	}
	if filesComponent.ToolPrefix != "files" {
		t.Fatalf("files component tool prefix = %q, want files", filesComponent.ToolPrefix)
	}
	if filesComponent.Tools != nil {
		t.Fatalf("files component tool overrides = %#v, want nil", filesComponent.Tools)
	}

	mmmcpConfig := MMMCPConfig(serverConfig, nil)
	if mmmcpConfig.Name != vmcp.Spec.Manifest.DisplayName {
		t.Fatalf("MMMCP name = %q, want %q", mmmcpConfig.Name, vmcp.Spec.Manifest.DisplayName)
	}
	if len(mmmcpConfig.Servers) != 2 {
		t.Fatalf("MMMCP server count = %d, want 2", len(mmmcpConfig.Servers))
	}
	var searchMMMCP *config.Server
	var filesMMMCP *config.Server
	for i := range mmmcpConfig.Servers {
		server := &mmmcpConfig.Servers[i]
		switch server.Name {
		case "search":
			searchMMMCP = server
		case "files":
			filesMMMCP = server
		}
	}
	if searchMMMCP == nil || filesMMMCP == nil {
		t.Fatalf("MMMCP config components = %#v, want search and files", mmmcpConfig.Servers)
	}
	if searchMMMCP.DisableTools || !filesMMMCP.DisableTools {
		t.Fatal("only the search component should expose tools")
	}
	if searchMMMCP.Prefix != "search" || searchMMMCP.URL != wantSearchURL {
		t.Fatalf("MMMCP search component = %#v, want prefix and URL", *searchMMMCP)
	}
	if len(searchMMMCP.Tools) != 1 {
		t.Fatalf("MMMCP search tool overrides = %#v, want one", searchMMMCP.Tools)
	}
	if searchMMMCP.Tools[0].Name != "find" || searchMMMCP.Tools[0].OverrideName != "find_documents" || searchMMMCP.Tools[0].Description != "Find a document" || searchMMMCP.Tools[0].OverrideDescription != "Search the document index" || !searchMMMCP.Tools[0].Enabled {
		t.Fatalf("MMMCP search tool override = %#v, want copied override", searchMMMCP.Tools[0])
	}
	if filesMMMCP.Prefix != "files" || filesMMMCP.URL != wantFilesURL || len(filesMMMCP.Tools) != 0 {
		t.Fatalf("MMMCP files component = %#v, want prefix, URL, and no overrides", *filesMMMCP)
	}

	var servers v1.MCPServerList
	if err := storageClient.List(t.Context(), &servers); err != nil {
		t.Fatalf("list component servers: %v", err)
	}
	if len(servers.Items) != 3 {
		t.Fatalf("MCPServer count = %d, want 3; resolving a vMCP must not deploy a wrapper", len(servers.Items))
	}
	for _, server := range servers.Items {
		if server.Name == vmcpID {
			t.Fatalf("vMCP resolution created wrapper MCPServer %q", vmcpID)
		}
	}
}

func TestServerConfigForVMCPRejectsEmptyBeforeCreatingInstance(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1empty",
		Namespace: system.DefaultNamespace,
	}
	storage := newVMCPTestStorage(vmcp)
	manager := &SessionManager{storageClient: storage}
	if _, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, "user"); err == nil || !strings.Contains(err.Error(), "without components") {
		t.Fatalf("expected empty VMCP connection error, got %v", err)
	}
	var instances v1.VMCPInstanceList
	if err := storage.List(t.Context(), &instances); err != nil {
		t.Fatal(err)
	}
	if len(instances.Items) != 0 {
		t.Fatal("rejected connection created an instance")
	}
}

func TestServerConfigForVMCPCreatesGeneratedInstance(t *testing.T) {
	const (
		vmcpID = "vmcp1personal"
		userID = "user-without-instance"
	)
	vmcp := &v1.VMCP{
		Name:      vmcpID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				DisplayName: "Personal VMCP",
				Components:  []types.VMCPComponent{{ID: "component"}},
			},
		},
	}
	storageClient := newVMCPTestStorage(vmcp, &v1.MCPServer{
		Name: "ms1shared", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{VMCPID: vmcpID, VMCPComponentID: "component"},
	})
	manager := &SessionManager{
		storageClient:  storageClient,
		httpListenPort: vmcpTestListenPort,
	}

	first, err := manager.ServerConfigForVMCP(t.Context(), vmcpID, userID)
	if err != nil {
		t.Fatalf("first ServerConfigForVMCP() error = %v", err)
	}
	second, err := manager.ServerConfigForVMCP(t.Context(), vmcpID, userID)
	if err != nil {
		t.Fatalf("second ServerConfigForVMCP() error = %v", err)
	}
	if first.Runtime != types.RuntimeVMCP || second.Runtime != types.RuntimeVMCP {
		t.Fatalf("vMCP runtimes = %q and %q, want %q", first.Runtime, second.Runtime, types.RuntimeVMCP)
	}
	if first.MCPServerName != vmcpID || first.MCPServerDisplayName != vmcp.Spec.Manifest.DisplayName {
		t.Fatalf("first vMCP identity = %#v, want vMCP identity", first)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated resolution changed vMCP config: first %#v, second %#v", first, second)
	}

	var instances v1.VMCPInstanceList
	if err := storageClient.List(t.Context(), &instances); err != nil {
		t.Fatalf("list VMCP instances: %v", err)
	}
	if len(instances.Items) != 1 {
		t.Fatalf("VMCP instance count = %d, want 1 after repeated resolution", len(instances.Items))
	}
	instance := instances.Items[0]
	if !slices.Equal(instance.Finalizers, []string{v1.VMCPInstanceFinalizer}) {
		t.Fatalf("missing instance credential cleanup finalizer: %v", instance.Finalizers)
	}
	if !strings.HasPrefix(instance.Name, system.VMCPInstancePrefix) {
		t.Fatalf("created VMCP instance name = %q, want prefix %q", instance.Name, system.VMCPInstancePrefix)
	}
	if instance.Spec.UserID != userID || instance.Spec.Manifest.VMCPID != vmcpID {
		t.Fatalf("created VMCP instance identity = %#v, want user %q and vMCP %q", instance.Spec, userID, vmcpID)
	}
}

func TestServerConfigForVMCPWaitsForComponentServer(t *testing.T) {
	const (
		vmcpID = "vmcp1notready"
		userID = "user-1"
	)
	instanceID := "vmcpi1-not-ready-instance"
	vmcp := &v1.VMCP{
		Name:      vmcpID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				DisplayName:     "Not Ready VMCP",
				ForceSingleUser: true,
				Components: []types.VMCPComponent{
					{
						ID:   "first-component",
						Name: "first",
					},
					{
						ID:   "second-component",
						Name: "second",
					},
				},
			},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      instanceID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcpID,
			},
			UserID: userID,
		},
	}
	storageClient := newVMCPTestStorage(vmcp, instance)
	watching := make(chan struct{})
	signalingStorage := &vmcpWatchSignalingStorage{
		Client:   storageClient,
		watching: watching,
	}
	createErr := make(chan error, 2)
	go func() {
		<-watching
		createErr <- storageClient.Create(t.Context(), vmcpComponentServer(
			"second-component-server",
			instanceID,
			userID,
			"second-component",
			"Second component",
			"https://second.example.test/mcp",
		))
		createErr <- storageClient.Create(t.Context(), vmcpComponentServer(
			"first-component-server",
			instanceID,
			userID,
			"first-component",
			"First component",
			"https://first.example.test/mcp",
		))
	}()
	manager := &SessionManager{storageClient: signalingStorage}

	serverConfig, err := manager.ServerConfigForVMCP(t.Context(), vmcpID, userID)
	if err != nil {
		t.Fatalf("ServerConfigForVMCP() error = %v", err)
	}
	for range 2 {
		if err := <-createErr; err != nil {
			t.Fatalf("create component server: %v", err)
		}
	}
	if len(serverConfig.Components) != 2 ||
		serverConfig.Components[0].Name != "first-component-server" ||
		serverConfig.Components[1].Name != "second-component-server" {
		t.Fatalf("components = %#v, want both servers in manifest order", serverConfig.Components)
	}
}

func (s *vmcpWatchSignalingStorage) Watch(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) (watch.Interface, error) {
	watcher, err := s.Client.Watch(ctx, list, opts...)
	if err == nil {
		s.once.Do(func() {
			close(s.watching)
		})
	}
	return watcher, err
}

func vmcpComponentServer(name, instanceID, userID, componentID, displayName, url string) *v1.MCPServer {
	return &v1.MCPServer{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:    displayName,
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: url,
				},
			},
			UserID:          userID,
			VMCPInstanceID:  instanceID,
			VMCPComponentID: componentID,
		},
	}
}

func newVMCPTestStorage(objects ...kclient.Object) storage.Client {
	return &vmcpInitialEventsStorage{Client: fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.VMCP{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCP).Spec.LegacySlug} }).
		WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.VMCPID}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.VMCPInstance).Spec.UserID}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.VMCPInstanceID}
		}).
		WithObjects(objects...).
		Build()}
}

func (s *vmcpInitialEventsStorage) Watch(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) (watch.Interface, error) {
	upstream, err := s.Client.Watch(ctx, list, opts...)
	if err != nil {
		return nil, err
	}

	initialList := list.DeepCopyObject().(kclient.ObjectList)
	listOptions := &kclient.ListOptions{}
	listOptions.ApplyOptions(opts)
	listOptions.Raw = nil
	if err := s.List(ctx, initialList, listOptions); err != nil {
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

func TestServerConfigForMultiUserVMCPUsesSharedServers(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1multi", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "one", Name: "one"}},
	}}}
	shared := vmcpComponentServer("ms1shared", "", "", "one", "one", "https://example.com/mcp")
	shared.Spec.VMCPID = vmcp.Name
	stale := vmcpComponentServer("ms1stale", "vmcpi1stale", "1", "one", "wrong", "https://wrong.example.com")
	storage := newVMCPTestStorage(vmcp, shared, stale)
	manager := &SessionManager{storageClient: storage}
	for _, userID := range []string{"1", "2"} {
		cfg, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Components) != 1 || cfg.Components[0].Name != shared.Name {
			t.Fatalf("user %s: wrong shared components: %#v", userID, cfg.Components)
		}
		if cfg.MCPServerName != vmcp.Name {
			t.Fatalf("wrong aggregate identity: %s", cfg.MCPServerName)
		}
		if cfg.AuditLogMetadata["mcpID"] != vmcp.Name || cfg.AuditLogMetadata["userID"] != userID {
			t.Fatalf("wrong shared vMCP audit attribution: %#v", cfg.AuditLogMetadata)
		}
	}
}
