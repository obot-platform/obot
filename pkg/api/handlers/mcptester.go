package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcptester"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mcpTesterProviderErrorLimit = 64 * 1024
	httpStatusClientClosed      = 499
)

type mcpTesterServerActionResolver interface {
	ServerForActionWithConnectID(context.Context, string, string) (string, v1.MCPServer, mcp.ServerConfig, error)
}

type mcpTesterHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type MCPTesterHandler struct {
	storage        kclient.Client
	serverResolver mcpTesterServerActionResolver
	accessHelper   *accesscontrolrule.Helper
	modelResolver  mcptester.ModelAccessResolver
	serverURL      string
	httpClient     mcpTesterHTTPClient
}

func NewMCPTesterHandler(storage kclient.Client, serverResolver mcpTesterServerActionResolver, accessHelper *accesscontrolrule.Helper, modelResolver mcptester.ModelAccessResolver, serverURL string, httpClient *http.Client) *MCPTesterHandler {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &MCPTesterHandler{
		storage:        storage,
		serverResolver: serverResolver,
		accessHelper:   accessHelper,
		modelResolver:  modelResolver,
		serverURL:      serverURL,
		httpClient:     httpClient,
	}
}

// Chat handles one stateless model continuation for the MCP Tester. It checks
// current MCP and model access on every call and never executes MCP tools.
func (h *MCPTesterHandler) Chat(req api.Context) error {
	mcpServerID := req.PathValue("mcp_server_id")
	authorized, err := authz.UserCanConnectToMCP(req.Context(), h.storage, h.accessHelper, req.User, mcpServerID)
	if err != nil || !authorized {
		return writeMCPTesterError(req, http.StatusForbidden, types.MCPTesterErrorAccessDenied, "you do not have permission to connect to this MCP server", false)
	}

	_, server, _, err := h.serverResolver.ServerForActionWithConnectID(req.Context(), mcpServerID, principal.ResourceOwnerID(req.User))
	if err != nil {
		return writeMCPTesterError(req, http.StatusForbidden, types.MCPTesterErrorAccessDenied, "the MCP server is not available to this user", false)
	}
	if server.Spec.Template || server.Spec.CompositeName != "" {
		return writeMCPTesterError(req, http.StatusForbidden, types.MCPTesterErrorAccessDenied, "the selected MCP server is not a connectable deployment", false)
	}

	model, err := mcptester.ResolveDefaultModel(req.Context(), h.storage, h.modelResolver, req.User)
	if err != nil {
		status := http.StatusServiceUnavailable
		if mcptester.IsModelResolutionError(err, mcptester.ModelResolutionErrorInaccessible) {
			status = http.StatusForbidden
		}
		return writeMCPTesterError(req, status, types.MCPTesterErrorModelUnavailable, err.Error(), false)
	}

	chatRequest, err := readMCPTesterChatRequest(&req)
	if err != nil {
		if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
			return writeMCPTesterError(req, httpErr.Code, types.MCPTesterErrorInvalidRequest, httpErr.Message, false)
		}
		return writeMCPTesterError(req, http.StatusBadRequest, types.MCPTesterErrorInvalidRequest, err.Error(), false)
	}

	body, err := mcptester.BuildModelRequest(chatRequest, model, testerSystemInstruction(server))
	if err != nil {
		return writeMCPTesterError(req, http.StatusBadRequest, types.MCPTesterErrorInvalidRequest, err.Error(), false)
	}

	proxyRequest, err := mcptester.NewLLMProxyRequest(req.Context(), h.serverURL, model, body, req.Request.Header)
	if err != nil {
		return writeMCPTesterError(req, http.StatusInternalServerError, types.MCPTesterErrorProvider, "failed to prepare model request", true)
	}
	response, err := h.httpClient.Do(proxyRequest)
	if err != nil {
		if req.Context().Err() != nil || errors.Is(err, context.Canceled) {
			return writeMCPTesterError(req, httpStatusClientClosed, types.MCPTesterErrorCancelled, "request cancelled", false)
		}
		return writeMCPTesterError(req, http.StatusBadGateway, types.MCPTesterErrorProvider, "failed to contact the model provider", true)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return h.writeProxyError(req, response)
	}

	req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	req.ResponseWriter.Header().Set("X-Accel-Buffering", "no")
	req.WriteHeader(http.StatusOK)
	req.Flush()

	_ = mcptester.NormalizeStream(req.Context(), model.Dialect, chatRequest.Tools, response.Body, func(event types.MCPTesterStreamEvent) error {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(req.ResponseWriter, "data: %s\n\n", data); err != nil {
			return err
		}
		req.Flush()
		return nil
	})
	return nil
}

func readMCPTesterChatRequest(req *api.Context) (types.MCPTesterChatRequest, error) {
	body, err := req.Body()
	if err != nil {
		return types.MCPTesterChatRequest{}, err
	}
	if len(body) == 0 {
		return types.MCPTesterChatRequest{}, fmt.Errorf("request body must not be empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request types.MCPTesterChatRequest
	if err := decoder.Decode(&request); err != nil {
		return types.MCPTesterChatRequest{}, fmt.Errorf("invalid request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return types.MCPTesterChatRequest{}, fmt.Errorf("request body must contain exactly one JSON value")
	}
	return request, nil
}

func testerSystemInstruction(server v1.MCPServer) string {
	name := strings.TrimSpace(server.Spec.Manifest.Name)
	if name == "" {
		name = server.Name
	}
	return fmt.Sprintf("You are testing the deployed MCP server %q in Obot's ephemeral MCP Tester. Use only the tools provided in this request. Do not claim access to any other tools, servers, files, or browser capabilities. Every tool call requires explicit user approval. Keep responses focused on testing this deployment.", name)
}

func (h *MCPTesterHandler) writeProxyError(req api.Context, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, mcpTesterProviderErrorLimit))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}

	switch response.StatusCode {
	case http.StatusUnauthorized:
		// The model provider rejected its credential.ß
		return writeMCPTesterError(req, http.StatusBadGateway, types.MCPTesterErrorProvider, message, false)
	case http.StatusForbidden:
		return writeMCPTesterError(req, http.StatusForbidden, types.MCPTesterErrorPolicyDenied, message, false)
	default:
		return writeMCPTesterError(req, http.StatusBadGateway, types.MCPTesterErrorProvider, message, response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError)
	}
}

func writeMCPTesterError(req api.Context, status int, code types.MCPTesterErrorCode, message string, retryable bool) error {
	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	return req.WriteCode(types.MCPTesterErrorResponse{
		Error: types.MCPTesterStreamError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}, status)
}
