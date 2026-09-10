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
	gserver "github.com/obot-platform/obot/pkg/gateway/server"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcptester"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mcpTesterProviderErrorLimit = 64 * 1024
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
	fallback       MCPTesterFallbackOptions
	fallbackClient mcpTesterHTTPClient
}

func NewMCPTesterHandler(storage kclient.Client, serverResolver mcpTesterServerActionResolver, accessHelper *accesscontrolrule.Helper, modelResolver mcptester.ModelAccessResolver, serverURL string, httpClient *http.Client, fallback ...MCPTesterFallbackOptions) *MCPTesterHandler {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	h := &MCPTesterHandler{
		storage:        storage,
		serverResolver: serverResolver,
		accessHelper:   accessHelper,
		modelResolver:  modelResolver,
		serverURL:      serverURL,
		httpClient:     httpClient,
		fallbackClient: mcptester.NewFallbackHTTPClient(),
	}

	if len(fallback) > 0 {
		h.fallback = fallback[0]
	}

	return h
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

	fallback, err := h.fallbackEnabled(req.Context())
	if err != nil {
		return writeMCPTesterError(req, http.StatusServiceUnavailable, types.MCPTesterErrorModelUnavailable, "model configuration is unavailable or changing", false)
	}

	model := mcptester.ResolvedModel{Dialect: llmtypes.DialectOpenAIResponses}
	if !fallback {
		model, err = mcptester.ResolveDefaultModel(req.Context(), h.storage, h.modelResolver, req.User)
		if err != nil {
			status := http.StatusServiceUnavailable
			if mcptester.IsModelResolutionError(err, mcptester.ModelResolutionErrorInaccessible) {
				status = http.StatusForbidden
			}

			return writeMCPTesterError(req, status, types.MCPTesterErrorModelUnavailable, err.Error(), false)
		}
	}

	chatRequest, err := readMCPTesterChatRequest(&req, fallback)
	if err != nil {
		if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
			return writeMCPTesterError(req, httpErr.Code, types.MCPTesterErrorInvalidRequest, httpErr.Message, false)
		}

		return writeMCPTesterError(req, http.StatusBadRequest, types.MCPTesterErrorInvalidRequest, err.Error(), false)
	}

	ctx := req.Context()
	var (
		proxyRequest *http.Request
		body         []byte
		audit        *gserver.TesterAudit
		outcome      error
	)

	outboundClient := h.httpClient
	if fallback {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, mcptester.FallbackTimeout)
		defer cancel()

		proxyRequest, body, err = h.fallbackRequest(ctx, chatRequest, server)
		if err != nil {
			status, code, message, retryable := http.StatusServiceUnavailable, types.MCPTesterErrorProvider, "The installation license could not be read. Try again later.", true
			if errors.Is(err, errMCPTesterLicenseRequired) {
				code, message, retryable = types.MCPTesterErrorLicenseRequired, errMCPTesterLicenseRequired.Error(), false
			} else if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok && httpErr.Code == http.StatusBadRequest {
				status, code, message, retryable = http.StatusBadRequest, types.MCPTesterErrorInvalidRequest, httpErr.Message, false
			}

			return writeMCPTesterError(req, status, code, message, retryable)
		}

		outboundClient = h.fallbackClient
		audit = gserver.NewTesterAudit(h.fallback.GatewayClient, req.Request, req.User, body)
		defer func() { audit.Finish(ctx, outcome) }()
	} else {
		body, err = mcptester.BuildModelRequest(chatRequest, model, testerSystemInstruction(server))
		if err != nil {
			return writeMCPTesterError(req, http.StatusBadRequest, types.MCPTesterErrorInvalidRequest, err.Error(), false)
		}

		proxyRequest, err = mcptester.NewLLMProxyRequest(ctx, h.serverURL, model, body, req.Request.Header)
		if err != nil {
			return writeMCPTesterError(req, http.StatusInternalServerError, types.MCPTesterErrorProvider, "failed to prepare model request", true)
		}
	}

	response, err := outboundClient.Do(proxyRequest)
	if err != nil {
		outcome = err
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return writeMCPTesterError(req, http.StatusRequestTimeout, types.MCPTesterErrorCancelled, "request cancelled", false)
		}

		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return writeMCPTesterError(req, http.StatusGatewayTimeout, types.MCPTesterErrorProvider, "The model service timed out. Try again later.", true)
		}

		return writeMCPTesterError(req, http.StatusBadGateway, types.MCPTesterErrorProvider, "failed to contact the model provider", true)
	}

	defer response.Body.Close()

	input := io.Reader(response.Body)
	if fallback {
		input = audit.Response(response)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if fallback {
			outcome = errors.New("model proxy rejected request")
			return writeMCPTesterFallbackError(req, response.StatusCode, input)
		}

		return h.writeProxyError(req, response)
	}

	req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	req.ResponseWriter.Header().Set("X-Accel-Buffering", "no")
	req.WriteHeader(http.StatusOK)
	req.Flush()

	streamErr := mcptester.NormalizeStream(ctx, model.Dialect, chatRequest.Tools, input, func(event types.MCPTesterStreamEvent) error {
		if fallback && event.Error != nil {
			outcome = errors.New("model proxy stream failed")
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				event.Error.Message = "The model service timed out. Try again later."
			} else if event.Error.Code != types.MCPTesterErrorCancelled {
				event.Error.Message = "The MCP Tester model service could not complete the response. Try again later."
			}
		}

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

	if streamErr != nil {
		outcome = streamErr
	}

	return nil
}

func readMCPTesterChatRequest(req *api.Context, fallback bool) (types.MCPTesterChatRequest, error) {
	var bodyOptions api.BodyOptions
	if fallback {
		bodyOptions.MaxBytes = mcptester.FallbackMaxBodyBytes
		req.Request.Body = http.MaxBytesReader(req.ResponseWriter, req.Request.Body, mcptester.FallbackMaxBodyBytes)
	}

	body, err := req.Body(bodyOptions)
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
		// The model provider rejected its credential.
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
