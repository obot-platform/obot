package adminmcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	catalogcontroller "github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	parityEntry = `name: Search
entryKey: search
runtime: npx
npxConfig:
  package: search
toolPreview:
  - name: read
  - name: write
`

	parityVMCP = `type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - id: search
    mcpServerCatalogEntryKey: search
`
)

type adminValidationResult struct {
	Valid       bool         `json:"valid"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Access      []VMCPAccess `json:"access"`
}

// Exercise the public MCP tool and actual catalog sync against the same storage
// and credential store, rather than deriving expected results from the validator.
type adminSyncTest struct {
	t       *testing.T
	handler *catalogcontroller.Handler
	gateway *gatewayclient.Client
	client  kclient.WithWatch
	catalog *v1.MCPCatalog
	dir     string
	session *gomcp.ClientSession
}

func newAdminSyncTest(t *testing.T) *adminSyncTest {
	t.Helper()
	dir := t.TempDir()
	catalog := &v1.MCPCatalog{
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       "MCPCatalog",
		Name:       "default",
		Namespace:  "default",
		UID:        ktypes.UID("catalog-test"),
		Spec:       v1.MCPCatalogSpec{SourceURLs: []string{dir}},
	}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion})
	for _, kind := range []string{"MCPCatalog", "MCPServerCatalogEntry", "VMCP"} {
		mapper.Add(v1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
	}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithRESTMapper(mapper).
		WithStatusSubresource(&v1.MCPCatalog{}).
		WithIndex(&v1.MCPServerCatalogEntry{}, "spec.mcpCatalogName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerCatalogEntry).Spec.MCPCatalogName}
		}).WithObjects(catalog).Build()
	gateway := newTestGatewayClient(t)
	handler := catalogcontroller.New("", "", gateway, nil, &mcp.SessionManager{}, 0)
	admin := NewHandler("https://obot.example.com", "", "", nil, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin.Builtin(api.Context{
			Request:        r,
			ResponseWriter: w,
			Storage:        client,
			GatewayClient:  gateway,
			User:           &kuser.DefaultInfo{UID: "42", Groups: types.RoleAdmin.Groups()},
		}).ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "catalog-test", Version: "1"}, nil).
		Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: srv.URL}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return &adminSyncTest{t: t, handler: handler, gateway: gateway, client: client, catalog: catalog, dir: dir, session: session}
}

func (s *adminSyncTest) validate(files ...CatalogFile) adminValidationResult {
	s.t.Helper()
	var beforeEntries, afterEntries v1.MCPServerCatalogEntryList
	var beforeVMCPs, afterVMCPs v1.VMCPList
	require.NoError(s.t, s.client.List(s.t.Context(), &beforeEntries))
	require.NoError(s.t, s.client.List(s.t.Context(), &beforeVMCPs))
	beforeCredentials, err := s.gateway.ListCredentials(s.t.Context(), gatewayclient.ListCredentialsOptions{AllContexts: true})
	require.NoError(s.t, err)
	result, err := s.session.CallTool(s.t.Context(), &gomcp.CallToolParams{
		Name: "validate_catalog_files",
		Arguments: map[string]any{
			"sourceURL": s.dir,
			"files":     files,
		},
	})
	require.NoError(s.t, err)
	require.False(s.t, result.IsError, "%+v", result.Content)
	data, err := json.Marshal(result.StructuredContent)
	require.NoError(s.t, err)
	var out adminValidationResult
	require.NoError(s.t, json.Unmarshal(data, &out))
	require.NoError(s.t, s.client.List(s.t.Context(), &afterEntries))
	require.NoError(s.t, s.client.List(s.t.Context(), &afterVMCPs))
	require.Equal(s.t, beforeEntries, afterEntries, "validation must not save entries")
	require.Equal(s.t, beforeVMCPs, afterVMCPs, "validation must not save vMCPs")
	afterCredentials, err := s.gateway.ListCredentials(s.t.Context(), gatewayclient.ListCredentialsOptions{AllContexts: true})
	require.NoError(s.t, err)
	require.Equal(s.t, beforeCredentials, afterCredentials, "validation must not save credentials")
	return out
}

func (s *adminSyncTest) sync(files ...CatalogFile) *v1.MCPCatalog {
	s.t.Helper()
	for _, file := range files {
		require.NoError(s.t, os.WriteFile(filepath.Join(s.dir, file.Path), []byte(file.Content), 0o600))
	}
	require.NoError(s.t, s.handler.SyncNow(s.t.Context(), s.client, kclient.ObjectKeyFromObject(s.catalog)))
	current := new(v1.MCPCatalog)
	require.NoError(s.t, s.client.Get(s.t.Context(), kclient.ObjectKeyFromObject(s.catalog), current))
	return current
}

func TestAdminValidationMatchesOpenAPISync(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		snapshot  bool
		wantValid bool
	}{
		{
			name:      "inline source",
			content:   validationOpenAPISchema,
			wantValid: true,
		},
		{
			name:    "malformed source",
			content: "not an OpenAPI document",
		},
		{
			name:     "supplied snapshot",
			content:  validationOpenAPISchema,
			snapshot: true,
		},
		{
			name:    "blocked API destination",
			content: strings.Replace(validationOpenAPISchema, "8.8.8.8", "127.0.0.1", 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newAdminSyncTest(t)
			entry := types.MCPServerCatalogEntryManifest{
				Name:     "Example",
				EntryKey: "example",
				Runtime:  types.RuntimeOpenAPI,
				OpenAPIConfig: &types.OpenAPIRuntimeConfig{
					Source: types.OpenAPISource{Content: tt.content},
				},
			}
			if tt.snapshot {
				entry.OpenAPIConfig.Schema = &types.OpenAPISchema{Raw: json.RawMessage(validationOpenAPISchema)}
			}
			data, err := json.Marshal(entry)
			require.NoError(t, err)
			file := CatalogFile{Path: "api.json", Content: string(data)}
			result := s.validate(file)
			require.Equal(t, tt.wantValid, result.Valid, "%+v", result.Diagnostics)
			current := s.sync(file)
			require.Equal(t, tt.wantValid, len(current.Status.SyncErrors) == 0, "%+v", current.Status.SyncErrors)
			var entries v1.MCPServerCatalogEntryList
			require.NoError(t, s.client.List(t.Context(), &entries))
			if tt.wantValid {
				require.Len(t, entries.Items, 1)
				require.JSONEq(t, validationOpenAPISchema, string(entries.Items[0].Spec.Manifest.OpenAPIConfig.Schema.Raw))
			} else {
				require.Empty(t, entries.Items)
			}
		})
	}
}

func TestAdminAccessReportMatchesSyncedPermissions(t *testing.T) {
	tests := []struct {
		name         string
		profiles     string
		component    string
		wantProfiles int
		wantTools    []string
		wantAll      bool
	}{
		{
			name:         "omitted profiles apply the admin default",
			wantProfiles: 1,
			wantTools:    []string{"read", "write"},
			wantAll:      true,
		},
		{
			name:         "null profiles apply the admin default",
			profiles:     "profiles: null\n",
			wantProfiles: 1,
			wantTools:    []string{"read", "write"},
			wantAll:      true,
		},
		{
			name:     "empty profiles grant nothing",
			profiles: "profiles: []\n",
		},
		{
			name:         "wildcard respects component overrides",
			profiles:     "profiles:\n  - name: operators\n    subjects: [{type: obotGroup, id: admin}]\n    vmcpPermissions:\n      allowedComponents:\n        search:\n          allowedTools: ['*']\n",
			component:    "    toolOverrides:\n      - name: read\n        enabled: true\n      - name: write\n        enabled: false\n",
			wantProfiles: 1,
			wantTools:    []string{"read"},
			wantAll:      true,
		},
		{
			name:         "omitted tool list grants all enabled tools",
			profiles:     "profiles:\n  - name: operators\n    subjects: [{type: obotGroup, id: admin}]\n    vmcpPermissions:\n      allowedComponents:\n        search: {}\n",
			wantProfiles: 1,
			wantTools:    []string{"read", "write"},
			wantAll:      true,
		},
		{
			name:         "explicit tool grant",
			profiles:     "profiles:\n  - name: operators\n    subjects: [{type: obotGroup, id: admin}]\n    vmcpPermissions:\n      allowedComponents:\n        search:\n          allowedTools: [read]\n",
			wantProfiles: 1,
			wantTools:    []string{"read"},
		},
		{
			name:         "empty tool grant",
			profiles:     "profiles:\n  - name: operators\n    subjects: [{type: obotGroup, id: admin}]\n    vmcpPermissions:\n      allowedComponents:\n        search:\n          allowedTools: []\n",
			wantProfiles: 1,
			wantTools:    []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newAdminSyncTest(t)
			files := []CatalogFile{
				{Path: "entry.yaml", Content: parityEntry},
				{Path: "vmcp.yaml", Content: parityVMCP + tt.component + tt.profiles},
			}
			result := s.validate(files...)
			require.True(t, result.Valid, "%+v", result.Diagnostics)
			require.Empty(t, s.sync(files...).Status.SyncErrors)
			var vmcps v1.VMCPList
			require.NoError(t, s.client.List(t.Context(), &vmcps))
			require.Len(t, vmcps.Items, 1)
			manifest := vmcps.Items[0].Spec.Manifest
			require.Len(t, result.Access, 1)
			require.Len(t, result.Access[0].Profiles, tt.wantProfiles)
			require.Len(t, manifest.Profiles, tt.wantProfiles)
			if tt.wantProfiles == 0 {
				return
			}
			component := result.Access[0].Profiles[0].Components[0]
			require.ElementsMatch(t, tt.wantTools, component.Tools)
			require.Equal(t, tt.wantAll, component.AllTools)
			grant := vmcpconfig.AllowedTools(&kuser.DefaultInfo{Extra: map[string][]string{"obot_groups": types.RoleAdmin.Groups()}}, manifest.Profiles, nil)
			for _, name := range []string{"read", "write"} {
				ref := types.VMCPToolReference{ComponentID: "search", Name: name}
				enabled := vmcpconfig.ToolGranted(grant, ref) && manifest.ValidateToolReference(ref) == nil
				require.Equal(t, enabled, slices.Contains(component.Tools, name), "synced grant for %s", name)
			}
		})
	}
}

func TestAdminValidationMatchesRetainedCredentials(t *testing.T) {
	s := newAdminSyncTest(t)
	entry := CatalogFile{Path: "entry.yaml", Content: parityEntry + "config:\n  - key: TOKEN\n    usage: env\n    required: true\n    sensitive: true\n"}
	vmcp := CatalogFile{Path: "vmcp.yaml", Content: parityVMCP + "    configuration:\n      - key: TOKEN\n        policy: fixed\nprofiles: []\n"}
	require.False(t, s.validate(entry, vmcp).Valid, "a new vMCP cannot rely on a retained credential")
	require.NotEmpty(t, s.sync(entry, vmcp).Status.SyncErrors)

	seed := vmcp
	seed.Content = strings.Replace(vmcp.Content, "        policy: fixed\n", "        policy: fixed\n        value: credential-sentinel\n", 1)
	result := s.validate(entry, seed)
	require.True(t, result.Valid, "%+v", result.Diagnostics)
	require.Empty(t, s.sync(entry, seed).Status.SyncErrors)
	result = s.validate(entry, vmcp)
	require.True(t, result.Valid, "%+v", result.Diagnostics)
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(data), "credential-sentinel")
	require.Empty(t, s.sync(entry, vmcp).Status.SyncErrors)

	changed := vmcp
	changed.Content = strings.Replace(vmcp.Content, "id: search", "id: replacement", 1)
	require.False(t, s.validate(entry, changed).Valid, "credentials are scoped to the stable component ID")
	require.NotEmpty(t, s.sync(entry, changed).Status.SyncErrors)

	userAllowed := vmcp
	userAllowed.Content = strings.Replace(vmcp.Content, "policy: fixed", "policy: userAllowed", 1)
	require.True(t, s.validate(entry, userAllowed).Valid)
	require.Empty(t, s.sync(entry, userAllowed).Status.SyncErrors)
}

func TestAdminValidationMatchesEntryStaticCredentials(t *testing.T) {
	s := newAdminSyncTest(t)
	entry := CatalogFile{
		Path: "entry.yaml",
		Content: parityEntry + `config:
  - key: TOKEN
    usage: env
    required: true
    sensitive: true
    value: catalog-credential-sentinel
`,
	}
	vmcp := CatalogFile{Path: "vmcp.yaml", Content: parityVMCP + "profiles: []\n"}
	result := s.validate(entry, vmcp)
	require.True(t, result.Valid, "%+v", result.Diagnostics)
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, "warning", result.Diagnostics[0].Severity)
	require.Contains(t, result.Diagnostics[0].Message, "Kubernetes secretBinding")
	data, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(data), "catalog-credential-sentinel")
	require.Empty(t, s.sync(entry, vmcp).Status.SyncErrors)
	var vmcps v1.VMCPList
	require.NoError(t, s.client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	config := vmcps.Items[0].Spec.Manifest.Components[0].CatalogEntry.Manifest.Config[0]
	require.True(t, config.Static)
	require.Empty(t, config.Value)
}
