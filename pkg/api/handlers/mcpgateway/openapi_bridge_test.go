package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/openapiworker"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestOpenAPIBridgeResponsesAreJSONOnly(t *testing.T) {
	executor := &fakeOpenAPIExecutor{}
	handler := &Handler{openAPIWorkerExecutor: executor}

	initialize, initializeResponse := bridgeRequest(t, http.MethodPost, "1", "initialize", `{}`)
	handled, err := handler.handleOpenAPIWorkerBridge(initialize, bridgeServer(), bridgeConfig())
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, http.StatusOK, initializeResponse.Code)
	require.Equal(t, "application/json", initializeResponse.Header().Get("Content-Type"))
	require.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"Obot OpenAPI Worker Bridge","version":"1.0"}}}`, initializeResponse.Body.String())

	list, listResponse := bridgeRequest(t, http.MethodPost, "2", "tools/list", `{}`)
	_, err = handler.handleOpenAPIWorkerBridge(list, bridgeServer(), bridgeConfig())
	require.NoError(t, err)
	require.Equal(t, "application/json", listResponse.Header().Get("Content-Type"))
	require.Contains(t, listResponse.Body.String(), `"legacyToolName":"getItem"`)
	require.Equal(t, "user-1", executor.describePrincipal.UserID())

	sseOnly := httptest.NewRequest(http.MethodPost, "/mcp-connect/connection", bytes.NewBufferString(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`))
	sseOnly.Header.Set("Accept", "text/event-stream")
	sseOnly.Header.Set("Content-Type", "application/json")
	sseResponse := httptest.NewRecorder()
	sseContext := api.Context{Request: sseOnly, ResponseWriter: sseResponse, User: bridgeUser()}
	handled, err = handler.handleOpenAPIWorkerBridge(sseContext, bridgeServer(), bridgeConfig())
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "application/json", sseResponse.Header().Get("Content-Type"))
	require.NotContains(t, sseResponse.Body.String(), "event:")

	get, getResponse := bridgeRequest(t, http.MethodGet, "4", "tools/list", ``)
	get.Request.Header.Set("Accept", "application/json")
	handled, err = handler.handleOpenAPIWorkerBridge(get, bridgeServer(), bridgeConfig())
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, http.StatusOK, getResponse.Code)
	require.Equal(t, "application/json", getResponse.Header().Get("Content-Type"))
}

func TestOpenAPIBridgeToolsCallUsesExecutor(t *testing.T) {
	executor := &fakeOpenAPIExecutor{}
	handler := &Handler{openAPIWorkerExecutor: executor}
	request, response := bridgeRequest(t, http.MethodPost, "7", "tools/call", `{"name":"getItem","arguments":{"id":"42"}}`)

	handled, err := handler.handleOpenAPIWorkerBridge(request, bridgeServer(), bridgeConfig())
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"status":418`)
	require.Equal(t, "user-1", executor.executePrincipal.UserID())
	require.Equal(t, "GET /items/{id}", executor.invocation.OperationKey)
	require.JSONEq(t, `{"id":"42"}`, string(executor.invocation.ToolInput))
}

func bridgeRequest(t *testing.T, method, id, rpcMethod, params string) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload := map[string]any{"jsonrpc": "2.0", "method": rpcMethod}
	if id != "" {
		payload["id"] = json.RawMessage(id)
	}
	if params != "" {
		payload["params"] = json.RawMessage(params)
	}
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(method, "/mcp-connect/connection", bytes.NewReader(encoded))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	return api.Context{Request: request, ResponseWriter: recorder, User: bridgeUser()}, recorder
}

func bridgeServer() v1.MCPServer {
	return v1.MCPServer{Spec: v1.MCPServerSpec{
		Manifest:        types.MCPServerManifest{Runtime: types.RuntimeOpenAPI},
		VMCPID:          "vmcp",
		VMCPComponentID: "component",
	}}
}

func bridgeConfig() mcp.ServerConfig {
	return mcp.ServerConfig{Runtime: types.RuntimeOpenAPI, MCPServerInstanceID: "connection"}
}

func bridgeUser() kuser.Info {
	return &kuser.DefaultInfo{
		Name: "user-1", UID: "user-1", Groups: []string{types.GroupAuthenticated},
		Extra: map[string][]string{"obot_groups": {}, "auth_provider_groups": {}},
	}
}

type fakeOpenAPIExecutor struct {
	describePrincipal openapiworker.PrincipalContext
	executePrincipal  openapiworker.PrincipalContext
	invocation        openapiworker.Invocation
}

func (e *fakeOpenAPIExecutor) Describe(_ context.Context, principal openapiworker.PrincipalContext, _ openapiworker.Reference) (openapiworker.ResolvedDescription, error) {
	e.describePrincipal = principal
	return openapiworker.ResolvedDescription{
		Operations: []openapiworker.OperationDescription{{
			OperationKey: "GET /items/{id}", LegacyToolName: "getItem", DisplayName: "Get item",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}, nil
}

func (e *fakeOpenAPIExecutor) Execute(_ context.Context, principal openapiworker.PrincipalContext, invocation openapiworker.Invocation) (openapiworker.ExecutionResult, error) {
	e.executePrincipal = principal
	e.invocation = invocation
	return openapiworker.ExecutionResult{
		Kind: openapiworker.ResultKindBusiness, RequestID: "request", Status: http.StatusTeapot,
		BodyBase64: "e30=",
	}, nil
}
