package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	// toolsBody is what the entry's Tools tab posts once the user listed the tools.
	toolsBody = `{"tools":[{"id":"list_catalogs","name":"list_catalogs"}]}`
)

type toolListTest struct {
	t        *testing.T
	handler  *MCPCatalogHandler
	storage  storage.Client
	gateway  *gclient.Client
	recorder *httptest.ResponseRecorder
}

func newToolListTest(t *testing.T) *toolListTest {
	t.Helper()
	entry := &v1.MCPServerCatalogEntry{
		Name: "default-obot-admin", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:          "Obot Admin",
				Runtime:       types.RuntimeBuiltin,
				BuiltinConfig: &types.BuiltinRuntimeConfig{Name: "obot-admin"},
				ToolPreview:   []types.MCPServerTool{{ID: "list_catalogs", Name: "list_catalogs"}},
			},
		},
	}
	gateway := newHandlerTestGateway(t)
	return &toolListTest{
		t:       t,
		handler: NewMCPCatalogHandler("", "https://obot.example.com", "", nil, nil, gateway, nil, "", nil),
		storage: storage.Client(fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry).Build()),
		gateway: gateway,
	}
}

// call calls handle as user with body, and decodes the response into out.
func (tt *toolListTest) call(handle func(api.Context) error, user kuser.Info, requestID, body string, out any) error {
	tt.t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/tool-list-requests", strings.NewReader(body))
	request.SetPathValue("request_id", requestID)
	tt.recorder = httptest.NewRecorder()
	err := handle(api.Context{
		Request:        request,
		ResponseWriter: tt.recorder,
		Storage:        tt.storage,
		GatewayClient:  tt.gateway,
		User:           user,
	})
	if err == nil && out != nil {
		require.NoError(tt.t, json.Unmarshal(tt.recorder.Body.Bytes(), out))
	}
	return err
}

func toolListUser(uid string, role types.Role, apiKey bool) kuser.Info {
	extra := map[string][]string{"obot_groups": role.Groups()}
	groups := role.Groups()
	if apiKey {
		// An API key with the CLI's default scopes: no admin groups, the user's role in obot_groups.
		extra[principal.APIKeyIDExtra] = []string{"5"}
		groups = []string{types.GroupLLM, types.GroupSkills, types.GroupAuthenticated}
	}
	return &kuser.DefaultInfo{Name: "user-" + uid, UID: uid, Groups: groups, Extra: extra}
}

func httpCode(err error) int {
	if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
		return httpErr.Code
	}
	return 0
}

func TestToolListRequestFlow(t *testing.T) {
	tt := newToolListTest(t)
	cli := toolListUser("42", types.RoleAdmin, true)
	browser := toolListUser("42", types.RoleAdmin, false)

	var created types.ToolListRequest
	require.NoError(t, tt.call(tt.handler.CreateToolListRequest, cli, "", `{"entryID":"default-obot-admin"}`, &created))
	assert.Equal(t, http.StatusCreated, tt.recorder.Code)
	assert.Equal(t, types.ToolListRequestPending, created.Status)
	assert.Equal(t, "Obot Admin", created.EntryName)
	// The CLI opens the entry's Tools tab, where the user confirms.
	assert.Equal(t, "https://obot.example.com/mcp-servers/c/default-obot-admin?view=tools&toolListRequest="+created.ID, created.URL)

	// The CLI's API key can poll, but can't answer the request; only the user's browser session can.
	assert.Equal(t, http.StatusForbidden, httpCode(tt.call(tt.handler.CompleteToolListRequest, cli, created.ID, toolsBody, nil)))
	assert.Equal(t, http.StatusForbidden, httpCode(tt.call(tt.handler.CancelToolListRequest, cli, created.ID, "", nil)))

	var polled types.ToolListRequest
	require.NoError(t, tt.call(tt.handler.GetToolListRequest, cli, created.ID, "", &polled))
	assert.Equal(t, types.ToolListRequestPending, polled.Status)

	require.NoError(t, tt.call(tt.handler.CompleteToolListRequest, browser, created.ID, toolsBody, nil))

	require.NoError(t, tt.call(tt.handler.GetToolListRequest, cli, created.ID, "", &polled))
	assert.Equal(t, types.ToolListRequestCompleted, polled.Status)
	require.Len(t, polled.Tools, 1)
	assert.Equal(t, "list_catalogs", polled.Tools[0].Name)

	// A request is answered once.
	assert.Equal(t, http.StatusConflict, httpCode(tt.call(tt.handler.CompleteToolListRequest, browser, created.ID, toolsBody, nil)))
}

// A launch that fails on the Tools tab reaches the CLI at once, rather than when the request expires.
func TestToolListRequestFailure(t *testing.T) {
	tt := newToolListTest(t)
	var created types.ToolListRequest
	require.NoError(t, tt.call(tt.handler.CreateToolListRequest, toolListUser("42", types.RoleAdmin, true), "", `{"entryID":"default-obot-admin"}`, &created))

	require.NoError(t, tt.call(tt.handler.CompleteToolListRequest, toolListUser("42", types.RoleAdmin, false), created.ID, `{"error":"readiness check failed"}`, nil))
	var polled types.ToolListRequest
	require.NoError(t, tt.call(tt.handler.GetToolListRequest, toolListUser("42", types.RoleAdmin, true), created.ID, "", &polled))
	assert.Equal(t, types.ToolListRequestFailed, polled.Status)
	assert.Equal(t, "readiness check failed", polled.Error)
	assert.Empty(t, polled.Tools)
}

func TestToolListRequestCancel(t *testing.T) {
	tt := newToolListTest(t)
	var created types.ToolListRequest
	require.NoError(t, tt.call(tt.handler.CreateToolListRequest, toolListUser("42", types.RoleAdmin, true), "", `{"entryID":"default-obot-admin"}`, &created))

	require.NoError(t, tt.call(tt.handler.CancelToolListRequest, toolListUser("42", types.RoleAdmin, false), created.ID, "", nil))
	var polled types.ToolListRequest
	require.NoError(t, tt.call(tt.handler.GetToolListRequest, toolListUser("42", types.RoleAdmin, true), created.ID, "", &polled))
	assert.Equal(t, types.ToolListRequestFailed, polled.Status)
	assert.Contains(t, polled.Error, "declined")
}

func TestToolListRequestAccess(t *testing.T) {
	tt := newToolListTest(t)
	var created types.ToolListRequest
	require.NoError(t, tt.call(tt.handler.CreateToolListRequest, toolListUser("42", types.RoleAdmin, true), "", `{"entryID":"default-obot-admin"}`, &created))

	tests := []struct {
		name     string
		handle   func(api.Context) error
		user     kuser.Info
		body     string
		id       string
		wantCode int
	}{
		{
			name:     "non-admin can't start a request",
			handle:   tt.handler.CreateToolListRequest,
			user:     toolListUser("7", types.RoleBasic, true),
			body:     `{"entryID":"default-obot-admin"}`,
			wantCode: http.StatusForbidden,
		},
		{
			name:     "unknown entry",
			handle:   tt.handler.CreateToolListRequest,
			user:     toolListUser("42", types.RoleAdmin, true),
			body:     `{"entryID":"default-missing"}`,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "another admin can't poll the request",
			handle:   tt.handler.GetToolListRequest,
			user:     toolListUser("7", types.RoleAdmin, true),
			id:       created.ID,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "another admin can't answer the request",
			handle:   tt.handler.CompleteToolListRequest,
			user:     toolListUser("7", types.RoleAdmin, false),
			body:     toolsBody,
			id:       created.ID,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "a demoted user can't answer their request",
			handle:   tt.handler.CompleteToolListRequest,
			user:     toolListUser("42", types.RoleBasic, false),
			body:     toolsBody,
			id:       created.ID,
			wantCode: http.StatusForbidden,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantCode, httpCode(tt.call(tc.handle, tc.user, tc.id, tc.body, nil)))
		})
	}
}
