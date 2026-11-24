package mcpgateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"golang.org/x/oauth2"
)

const (
	mcpToolsCallMethod = "tools/call"
)

// mcpCall is the JSON-RPC call a request carries.
type mcpCall struct {
	id     jsonrpc.ID
	method string
}

// usesPairToken reports whether serverConfig, the connection of a component hop, authorizes with the upstream token
// that a user connected through an agent, for the agent acting on behalf of that user: the connection is the pair's
// (principal.DelegatedOwner), and the vMCP holds no preconfigured OAuth for the component, which would be used in its
// place.
func usesPairToken(serverConfig mcp.ServerConfig) bool {
	owner, _, ok := serverConfig.OAuthTokenKey()
	return ok && serverConfig.OAuthGrant == nil && principal.IsDelegatedOwner(owner)
}

// refreshFailure describes a failed refresh of an upstream token without the provider's response body, which is the
// provider's and may carry anything.
func refreshFailure(err error) string {
	if retrieveErr, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		if retrieveErr.Response != nil {
			return fmt.Sprintf("token endpoint answered %d %s", retrieveErr.Response.StatusCode, retrieveErr.ErrorCode)
		}
		return "token endpoint refused the refresh " + retrieveErr.ErrorCode
	}
	return err.Error()
}

// peekMCPCall returns the JSON-RPC call that r carries, if it is one, and leaves r's body to be read again. It is nil
// for anything else, and for a body too large to read here.
func peekMCPCall(r *http.Request) *mcpCall {
	if r.Method != http.MethodPost || r.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxJSONRPCErrorRequestBody+1))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), r.Body))
	if err != nil || len(raw) > maxJSONRPCErrorRequestBody {
		return nil
	}
	decoded, _, err := decodeMCPHookBody(io.NopCloser(bytes.NewReader(raw)), r.Header.Get("Content-Encoding"))
	if err != nil {
		return nil
	}
	defer decoded.Close()
	body, err := io.ReadAll(io.LimitReader(decoded, maxJSONRPCErrorRequestBody+1))
	if err != nil || len(body) > maxJSONRPCErrorRequestBody {
		return nil
	}
	msg, err := jsonrpc.DecodeMessage(body)
	if err != nil {
		return nil
	}
	call, ok := msg.(*jsonrpc.Request)
	if !ok || call == nil || !call.IsCall() {
		return nil
	}
	return &mcpCall{id: call.ID, method: call.Method}
}

// pairComponent returns the vMCP and the component that serverConfig, the configuration of a component hop, connects
// to, from the connection or server it names. Either is empty when it cannot be read.
func pairComponent(req api.Context, serverConfig mcp.ServerConfig) (vmcpID, componentID string) {
	var instanceID string
	if connectionID := serverConfig.MCPServerInstanceID; connectionID != "" {
		var connection v1.MCPServerInstance
		if err := req.Get(&connection, connectionID); err != nil {
			return "", ""
		}
		instanceID, componentID = connection.Spec.VMCPInstanceID, connection.Spec.VMCPComponentID
	} else if system.IsMCPServerID(serverConfig.MCPServerName) {
		var server v1.MCPServer
		if err := req.Get(&server, serverConfig.MCPServerName); err != nil {
			return "", ""
		}
		instanceID, componentID = server.Spec.VMCPInstanceID, server.Spec.VMCPComponentID
	}
	var instance v1.VMCPInstance
	if instanceID == "" || req.Get(&instance, instanceID) != nil {
		return "", componentID
	}
	return instance.Spec.Manifest.VMCPID, componentID
}

// delegatedSetupBrokenResponse is the response to an agent acting on behalf of a user whose request to a vMCP
// component cannot authorize with the upstream account the user connected through the agent: the provider refused to
// refresh it (invalid_grant), or the upstream refused it, so the user revoked it or it expired. Only the user can fix
// it, by approving the agent again and connecting the account. The failure is agent_user_approval_required, naming
// the vMCP and the component:
//
//   - For a tool call, it is the call's result, an MCP tool error, so that the agent's session goes on and the agent
//     learns why, mid-session, as it asks the user for approval.
//   - Anything else at a component hop is a bare 401, which the composite reports as the component's authorization
//     failure, and answers the agent with a 409 that names it (agentOAuthRequiredResponse).
//   - Anywhere else, it is that 409.
//
// None carries WWW-Authenticate, and no challenge is created.
func delegatedSetupBrokenResponse(req api.Context, call *mcpCall, vmcpID, componentID string) *mcpResponse {
	failure := vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		VMCPID:  vmcpID,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: componentID, Code: types.AgentReadinessOAuthRequired}},
	}
	if call != nil && call.method == mcpToolsCallMethod {
		if response := mcpToolErrorResponse(call.id, failure); response != nil {
			return response
		}
	}
	if principal.IsComponentHop(req.User) {
		return bareUnauthorizedResponse()
	}
	return agentReadinessResponse(failure)
}

// mcpToolErrorResponse is the JSON-RPC response to the tool call with id: a tool error whose content, and structured
// content, is failure.
func mcpToolErrorResponse(id jsonrpc.ID, failure vmcpaccess.AgentReadinessFailure) *mcpResponse {
	text, err := json.Marshal(failure)
	if err != nil {
		return nil
	}
	result, err := json.Marshal(&gomcp.CallToolResult{
		IsError:           true,
		Content:           []gomcp.Content{&gomcp.TextContent{Text: string(text)}},
		StructuredContent: failure,
	})
	if err != nil {
		return nil
	}
	body, err := jsonrpc.EncodeMessage(&jsonrpc.Response{ID: id, Result: result})
	if err != nil {
		return nil
	}
	return mcpJSONResponse(body)
}

// forgetRefusedPairToken removes the pair's upstream token that the provider refused to refresh
// (mcp.RefusedRefresh), so that the user's setup through the agent shows the account as to connect again, and the
// agent's next connection on behalf of the user is refused until it is. Only a token that is still expired is
// removed, by its grant ID: one the user connected again in the meantime is valid, and kept.
func forgetRefusedPairToken(req api.Context, store mcp.TokenStorage, owner, mcpID string) {
	_, current, err := store.GetTokenConfig(req.Context())
	if err != nil || current == nil || current.Valid() {
		return
	}
	grantID, _ := current.Extra("obot_grant_id").(string)
	if grantID == "" {
		return
	}
	if err := req.GatewayClient.DeleteMCPOAuthGrants(req.Context(), owner, mcpID, []string{grantID}); err != nil {
		slog.WarnContext(req.Context(), "Failed to remove an upstream account that its provider refused to refresh", "mcpID", mcpID, "error", err)
	}
}
