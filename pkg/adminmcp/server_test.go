package adminmcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testBaseURL     = "https://obot.example.com"
	otherSourceURL  = "https://github.com/acme/other"
	testAdminUserID = "42"
)

// fakeAuthProviders reports the auth provider Obot serves logins from.
type fakeAuthProviders struct {
	configured string
}

type testServer struct {
	url           string
	gatewayClient *gatewayclient.Client
}

func (f fakeAuthProviders) GetConfiguredAuthProvider(context.Context) (string, error) {
	return f.configured, nil
}

func (f fakeAuthProviders) URLForAuthProvider(_ context.Context, namespace, name string) (url.URL, error) {
	return url.URL{Scheme: "http", Host: name + "." + namespace + ".provider.test"}, nil
}

func newTestStorage() storage.Client {
	entry := func(name, sourceURL, entryKey string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
		manifest.EntryKey = entryKey
		return &v1.MCPServerCatalogEntry{
			Namespace: system.DefaultNamespace,
			Name:      name,
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: "default",
				SourceURL:      sourceURL,
				Manifest:       manifest,
			},
		}
	}
	return clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithIndex(&v1.MCPServerCatalogEntry{}, "spec.mcpCatalogName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerCatalogEntry).Spec.MCPCatalogName}
		}).
		WithObjects(
			&v1.MCPCatalog{
				Namespace: system.DefaultNamespace,
				Name:      "default",
				Spec: v1.MCPCatalogSpec{
					DisplayName: "Default",
					SourceURLs:  []string{testSourceURL, otherSourceURL},
				},
				Status: v1.MCPCatalogStatus{SyncErrors: map[string]string{otherSourceURL: "clone failed"}},
			},
			entry("default-github", testSourceURL, "github", types.MCPServerCatalogEntryManifest{
				Name:             "GitHub",
				ShortDescription: "GitHub issues and pull requests",
				Runtime:          types.RuntimeRemote,
				Config: []types.MCPConfig{
					{Key: "TOKEN", Name: "Token", Usage: types.Header, Required: true, Sensitive: true},
				},
				ToolPreview: []types.MCPServerTool{
					{Name: "search_issues", Description: "Search issues"},
					{Name: "create_issue", Description: "Create an issue"},
				},
			}),
			entry("default-slack", otherSourceURL, "slack", types.MCPServerCatalogEntryManifest{
				Name:    "Slack",
				Runtime: types.RuntimeRemote,
			}),
			entry("default-handmade", "", "", types.MCPServerCatalogEntryManifest{
				Name:    "Handmade",
				Runtime: types.RuntimeRemote,
			}),
		).
		Build()
}

func newTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	gatewayClient := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	require.NoError(t, db.WithContext(t.Context()).Create(&gatewaytypes.User{
		ID:             42,
		Username:       "alice",
		HashedUsername: hash.String("alice"),
		DisplayName:    "Alice",
		Email:          "alice@example.com",
		HashedEmail:    hash.String("alice@example.com"),
		Role:           types.RoleAdmin,
	}).Error)
	return gatewayClient
}

// newTestServer serves the Obot Admin server to the given user, as the gateway does for a vMCP,
// on an Obot with no auth provider configured.
func newTestServer(t *testing.T, caller user.Info) testServer {
	t.Helper()
	return newTestServerWithAuthProvider(t, caller, "")
}

// newTestServerWithAuthProvider serves the Obot Admin server to the given user, on an Obot that
// serves logins from the named auth provider.
func newTestServerWithAuthProvider(t *testing.T, caller user.Info, authProvider string) testServer {
	t.Helper()
	store := newTestStorage()
	gatewayClient := newTestGatewayClient(t)
	h := NewHandler(testBaseURL, "kubernetes", "", nil, fakeAuthProviders{configured: authProvider})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Builtin(api.Context{
			ResponseWriter: w,
			Request:        r,
			Storage:        store,
			GatewayClient:  gatewayClient,
			User:           caller,
		}).ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return testServer{url: srv.URL + "/" + BuiltinName, gatewayClient: gatewayClient}
}

// vmcpUser is the user of a vMCP's loopback request to its components: the user who connected to
// the vMCP, with their current role in obot_groups.
func vmcpUser(roleGroups ...string) *user.DefaultInfo {
	return &user.DefaultInfo{
		UID:    testAdminUserID,
		Name:   "alice",
		Groups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
		Extra: map[string][]string{
			"obot_groups":        roleGroups,
			"authorized_mcp_ids": {"vmcp1test"},
		},
	}
}

func connect(t *testing.T, url string, opts *gomcp.ClientOptions) *gomcp.ClientSession {
	t.Helper()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-agent", Version: "1.0.0"}, opts)
	session, err := client.Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: url}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callTool[T any](t *testing.T, session *gomcp.ClientSession, name string, arguments any) (T, *gomcp.CallToolResult) {
	t.Helper()
	result, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: name, Arguments: arguments})
	require.NoError(t, err)
	var out T
	if !result.IsError {
		data, err := json.Marshal(result.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &out))
	}
	return out, result
}

func resultText(result *gomcp.CallToolResult) string {
	var text []string
	for _, content := range result.Content {
		if content, ok := content.(*gomcp.TextContent); ok {
			text = append(text, content.Text)
		}
	}
	return strings.Join(text, "\n")
}

func TestAdminMCPTools(t *testing.T) {
	srv := newTestServer(t, vmcpUser(types.GroupAdmin, types.GroupAuthenticated))
	session := connect(t, srv.url, nil)
	scope := map[string]any{"sourceURL": testSourceURL}

	t.Run("lists the tools", func(t *testing.T) {
		result, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		var names []string
		for _, tool := range result.Tools {
			names = append(names, tool.Name)
		}
		assert.ElementsMatch(t, []string{
			toolListCatalogs, toolSearchCatalogEntries, toolGetCatalogEntry, toolListEntryTools, toolSearchGroups,
			toolFindUser, toolResolveSubjects, toolListSecretBindings, toolValidateCatalogFiles,
		}, names)
	})

	t.Run("serves the schema resource", func(t *testing.T) {
		result, err := session.ReadResource(t.Context(), &gomcp.ReadResourceParams{URI: mcpcatalog.CatalogItemSchemaURI})
		require.NoError(t, err)
		require.Len(t, result.Contents, 1)
		assert.Contains(t, result.Contents[0].Text, "vmcpPermissions")
	})

	t.Run("lists catalogs and their sources", func(t *testing.T) {
		out, _ := callTool[listCatalogsOutput](t, session, toolListCatalogs, map[string]any{})
		require.Len(t, out.Catalogs, 1)
		assert.Equal(t, []catalogSource{
			{URL: testSourceURL, SourceID: "github.com/acme/catalog"},
			{URL: otherSourceURL, SourceID: "github.com/acme/other", SyncError: "clone failed"},
		}, out.Catalogs[0].Sources)
	})

	t.Run("returns references relative to the edited source", func(t *testing.T) {
		out, _ := callTool[searchCatalogEntriesOutput](t, session, toolSearchCatalogEntries, scope)
		references := map[string]string{}
		for _, entry := range out.Entries {
			references[entry.Name] = entry.Reference + entry.ReferenceError
		}
		assert.Equal(t, "github", references["GitHub"])
		assert.Equal(t, "github.com/acme/other::slack", references["Slack"])
		assert.Contains(t, references["Handmade"], "created in Obot")

		out, _ = callTool[searchCatalogEntriesOutput](t, session, toolSearchCatalogEntries, map[string]any{"sourceURL": testSourceURL, "query": "pull requests"})
		require.Len(t, out.Entries, 1)
		assert.Equal(t, "GitHub", out.Entries[0].Name)
		assert.Equal(t, []configInfo{{Key: "TOKEN", Name: "Token", Usage: "header", Required: true, Sensitive: true}}, out.Entries[0].Configuration)
	})

	t.Run("rejects a source that isn't the catalog's", func(t *testing.T) {
		_, result := callTool[searchCatalogEntriesOutput](t, session, toolSearchCatalogEntries, map[string]any{"sourceURL": "https://github.com/acme/unknown"})
		assert.True(t, result.IsError)
		assert.Contains(t, resultText(result), "is not a source of catalog")
	})

	t.Run("returns stored tools without launching", func(t *testing.T) {
		out, _ := callTool[listEntryToolsOutput](t, session, toolListEntryTools, map[string]any{"sourceURL": testSourceURL, "reference": "github"})
		assert.Equal(t, []toolInfo{{Name: "search_issues", Description: "Search issues"}, {Name: "create_issue", Description: "Create an issue"}}, out.Tools)
	})

	t.Run("sends entries without a tool list to the obot CLI", func(t *testing.T) {
		// Listing launches the entry's server, which the user confirms in Obot through the CLI.
		_, result := callTool[listEntryToolsOutput](t, session, toolListEntryTools, map[string]any{"sourceURL": testSourceURL, "reference": "github.com/acme/other::slack"})
		assert.True(t, result.IsError)
		assert.Contains(t, resultText(result), "obot mcp list-tools default-slack --non-interactive")
		assert.Contains(t, resultText(result), "Don't guess the tool names")
	})

	t.Run("checks files", func(t *testing.T) {
		out, _ := callTool[validateCatalogFilesOutput](t, session, toolValidateCatalogFiles, map[string]any{
			"sourceURL": testSourceURL,
			"files": []CatalogFile{{Path: "support.yaml", Content: `
type: vmcp
entryKey: support
displayName: Support
components:
  - id: github
    mcpServerCatalogEntryKey: github
    configuration:
      - key: TOKEN
        policy: fixed
        value: ghp_secret
profiles:
  - name: support
    subjects:
      - type: user
        id: "42"
    vmcpPermissions:
      allowedComponents:
        github:
          allowedTools: [search_issues, delete_repo]
`}},
		})
		assert.False(t, out.Valid)
		var messages []string
		for _, diagnostic := range out.Diagnostics {
			messages = append(messages, diagnostic.Severity+": "+diagnostic.Message)
		}
		assert.Len(t, messages, 3, messages)
		assert.Contains(t, strings.Join(messages, "\n"), `error: component "github" configuration "TOKEN" is sensitive`)
		assert.Contains(t, strings.Join(messages, "\n"), `error: profile "support" names tool "delete_repo"`)
		assert.Contains(t, strings.Join(messages, "\n"), `warning: profile "support" grants user "42"`)
	})

	t.Run("identifies the files it checked and reports access", func(t *testing.T) {
		content := "type: vmcp\nentryKey: support\ndisplayName: Support\ncomponents:\n  - id: github\n    mcpServerCatalogEntryKey: github\n" +
			"    configuration:\n      - key: TOKEN\n        policy: fixed\n        secretBinding:\n          name: github\n          key: token\n" +
			"profiles:\n  - name: support\n    subjects:\n      - type: obotGroup\n        id: admin\n" +
			"    vmcpPermissions:\n      allowedComponents:\n        github:\n          allowedTools: [search_issues]\n"
		out, _ := callTool[validateCatalogFilesOutput](t, session, toolValidateCatalogFiles, map[string]any{
			"sourceURL": testSourceURL,
			"files":     []CatalogFile{{Path: "support.yaml", Content: content}},
		})
		assert.True(t, out.Valid, out.Diagnostics)
		assert.Equal(t, []checkedFile{{
			Path:   "support.yaml",
			Lines:  21,
			SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(content))),
		}}, out.Files)
		assert.Equal(t, []VMCPAccess{{
			File: "support.yaml",
			VMCP: "Support",
			Profiles: []ProfileAccess{{
				Profile:    "support",
				Components: []ComponentAccess{{Component: "github", Tools: []string{"search_issues"}}},
			}},
		}}, out.Access)
	})

	t.Run("finds users by exact username or email", func(t *testing.T) {
		for _, query := range []string{"alice", "alice@example.com"} {
			out, _ := callTool[findUserOutput](t, session, toolFindUser, map[string]any{"usernameOrEmail": query})
			assert.Equal(t, []userInfo{{ID: testAdminUserID, DisplayName: "Alice", Username: "alice"}}, out.Users, query)
		}
		out, _ := callTool[findUserOutput](t, session, toolFindUser, map[string]any{"usernameOrEmail": "ali"})
		assert.Empty(t, out.Users)
	})

	t.Run("resolves subjects", func(t *testing.T) {
		out, _ := callTool[resolveSubjectsOutput](t, session, toolResolveSubjects, map[string]any{"subjects": []types.Subject{
			{Type: types.SubjectTypeUser, ID: testAdminUserID},
			{Type: types.SubjectTypeUser, ID: "7"},
			{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin},
			{Type: types.SubjectTypeSelector, ID: "*"},
		}})
		assert.Equal(t, []subjectInfo{
			{Type: types.SubjectTypeUser, ID: testAdminUserID, Name: "Alice", Found: true},
			{Type: types.SubjectTypeUser, ID: "7"},
			{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin, Name: "Obot role admin", Found: true},
			{Type: types.SubjectTypeSelector, ID: "*", Name: "Every user", Found: true},
		}, out.Subjects)
	})
}

func TestSearchGroupsExplainsMissingGroups(t *testing.T) {
	tests := []struct {
		name         string
		authProvider string
		want         string
	}{
		{
			name:         "local accounts",
			authProvider: system.LocalAuthProvider,
			want:         "this Obot uses local accounts, which have no groups",
		},
		{
			name: "no auth provider",
			want: "this Obot has no auth provider configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The configured provider decides, whatever provider the caller's own session names.
			caller := vmcpUser(types.GroupAdmin)
			caller.Extra["auth_provider_name"] = []string{"okta-auth-provider"}
			caller.Extra["auth_provider_namespace"] = []string{system.DefaultNamespace}
			session := connect(t, newTestServerWithAuthProvider(t, caller, tt.authProvider).url, nil)

			_, result := callTool[searchGroupsOutput](t, session, toolSearchGroups, map[string]any{"query": "support"})
			assert.True(t, result.IsError)
			assert.Contains(t, resultText(result), tt.want)
			assert.Contains(t, resultText(result), "use obotGroup subjects")
		})
	}
}
