package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gserver "github.com/obot-platform/obot/pkg/gateway/server"
	"github.com/obot-platform/obot/pkg/mcptester"
	"gorm.io/gorm"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	llmProviderTesterMaxBodyBytes  = 16 * 1024
	llmProviderTesterErrorLimit    = 64 * 1024
	llmProviderTesterTimeout       = 2 * time.Minute
	llmProviderTesterTimeoutReason = "the LLM proxy did not respond in time"
)

// LLMProviderTesterHandler sends a single test message to a gateway provider's
// model through Obot's own LLM proxy. Stored keys are bcrypt-hashed and cannot be
// read back, so the handler checks the key's status and LLM proxy scope itself
// then calls the proxy with the admin's own session. Going through the proxy, rather than
// calling the provider directly, also checks routing, model access, and the
// gateway's own handling. Upstream errors are returned as-is, because
// diagnosing them is the point of the test.
type LLMProviderTesterHandler struct {
	storage        kclient.Client
	localServerURL string
	httpClient     mcpTesterHTTPClient
}

// proxyDecidesModelAccess leaves the model access check to the LLM proxy, so a
// denial reaches the admin as the proxy's own error, like any other client's.
type proxyDecidesModelAccess struct{}

func NewLLMProviderTesterHandler(storage kclient.Client, localServerURL string, httpClient *http.Client) *LLMProviderTesterHandler {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &LLMProviderTesterHandler{
		storage:        storage,
		localServerURL: localServerURL,
		httpClient:     httpClient,
	}
}

func (proxyDecidesModelAccess) UserHasAccessToModel(kuser.Info, string) (bool, error) {
	return true, nil
}

func (h *LLMProviderTesterHandler) Chat(req api.Context) error {
	provider := req.PathValue("provider")
	if provider == "" {
		return types.NewErrBadRequest("provider is required")
	}

	testRequest, err := readLLMProviderTestRequest(&req)
	if err != nil {
		return err
	}

	if err := checkTestAPIKey(req, testRequest.APIKeyID); err != nil {
		return err
	}

	model, err := mcptester.ResolveModel(req.Context(), h.storage, proxyDecidesModelAccess{}, req.User, testRequest.Model)
	if err != nil {
		return modelResolutionHTTPError(err)
	}
	if modelProvider, _, _ := strings.Cut(model.Target, "/"); modelProvider != provider {
		return types.NewErrBadRequest("model %q is not served by provider %q", model.ID, provider)
	}

	body, err := mcptester.BuildModelRequest(types.MCPTesterChatRequest{
		Round: 1,
		Messages: []types.MCPTesterChatMessage{{
			Role: types.MCPTesterChatRoleUser,
			Content: []types.MCPTesterContent{{
				Type: types.MCPTesterContentTypeText,
				Text: testRequest.Message,
			}},
		}},
	}, model, "")
	if err != nil {
		return types.NewErrBadRequest("%v", err)
	}

	ctx, cancel := context.WithTimeout(req.Context(), llmProviderTesterTimeout)
	defer cancel()

	// The selected key's secret is not available, so the admin's own
	// credentials authenticate the request, as in the MCP tester.
	proxyRequest, err := mcptester.NewLLMProxyRequest(ctx, h.localServerURL, model, body, req.Request.Header)
	if err != nil {
		return fmt.Errorf("failed to prepare LLM proxy request: %w", err)
	}
	proxyRequest.Header.Set("User-Agent", types.LLMProviderTesterClientName)

	req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	req.ResponseWriter.Header().Set("Cache-Control", "no-cache")
	req.ResponseWriter.Header().Set("X-Accel-Buffering", "no")
	req.WriteHeader(http.StatusOK)
	req.Flush()

	// Once the stream has started, failures are reported as events; a write
	// error only means the browser went away, so it is not returned.
	emit := func(event types.LLMProviderTestEvent) error {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(req.ResponseWriter, "data: %s\n\n", data); err != nil {
			return err
		}
		req.Flush()
		return nil
	}

	start := time.Now()
	elapsed := func() int64 {
		// Report at least 1ms so a terminal event always carries a duration.
		return max(time.Since(start).Milliseconds(), 1)
	}

	response, err := h.httpClient.Do(proxyRequest)
	if err != nil {
		message := fmt.Sprintf("failed to reach the LLM proxy: %v", err)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = llmProviderTesterTimeoutReason
		}
		_ = emit(types.LLMProviderTestEvent{
			Type:       types.LLMProviderTestEventError,
			Error:      message,
			DurationMs: elapsed(),
		})
		return nil
	}
	defer response.Body.Close()

	servedProvider := response.Header.Get(gserver.HeaderServedProvider)
	servedModel := response.Header.Get(gserver.HeaderServedModel)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		upstreamBody, _ := io.ReadAll(io.LimitReader(response.Body, llmProviderTesterErrorLimit))
		message := strings.TrimSpace(string(upstreamBody))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		_ = emit(types.LLMProviderTestEvent{
			Type:        types.LLMProviderTestEventError,
			Status:      response.StatusCode,
			Error:       message,
			Provider:    servedProvider,
			ServedModel: servedModel,
			DurationMs:  elapsed(),
		})
		return nil
	}

	var reply strings.Builder
	_ = mcptester.NormalizeStream(ctx, model.Dialect, nil, response.Body, func(event types.MCPTesterStreamEvent) error {
		switch event.Type {
		case types.MCPTesterStreamEventTextDelta:
			reply.WriteString(event.Delta)
			return emit(types.LLMProviderTestEvent{
				Type:  types.LLMProviderTestEventTextDelta,
				Delta: event.Delta,
			})
		case types.MCPTesterStreamEventCompletion:
			return emit(types.LLMProviderTestEvent{
				Type:        types.LLMProviderTestEventReply,
				Reply:       reply.String(),
				Provider:    servedProvider,
				ServedModel: servedModel,
				DurationMs:  elapsed(),
			})
		case types.MCPTesterStreamEventError:
			message := "the model stream failed"
			if event.Error != nil {
				message = event.Error.Message
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				message = llmProviderTesterTimeoutReason
			}
			return emit(types.LLMProviderTestEvent{
				Type:        types.LLMProviderTestEventError,
				Error:       message,
				Provider:    servedProvider,
				ServedModel: servedModel,
				DurationMs:  elapsed(),
			})
		default:
			return nil
		}
	})
	return nil
}

func readLLMProviderTestRequest(req *api.Context) (types.LLMProviderTestRequest, error) {
	body, err := req.Body(api.BodyOptions{MaxBytes: llmProviderTesterMaxBodyBytes})
	if err != nil {
		return types.LLMProviderTestRequest{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request types.LLMProviderTestRequest
	if err := decoder.Decode(&request); err != nil {
		return types.LLMProviderTestRequest{}, types.NewErrBadRequest("invalid request body: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return types.LLMProviderTestRequest{}, types.NewErrBadRequest("request body must contain exactly one JSON value")
	}

	request.Model = strings.TrimSpace(request.Model)
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		request.Message = types.LLMProviderTesterDefaultMessage
	}
	if len(request.Message) > types.LLMProviderTesterMaxMessageSize || !utf8.ValidString(request.Message) {
		return types.LLMProviderTestRequest{}, types.NewErrBadRequest("message must be valid text of at most %d bytes", types.LLMProviderTesterMaxMessageSize)
	}
	if request.Model == "" {
		return types.LLMProviderTestRequest{}, types.NewErrBadRequest("model is required")
	}
	if request.APIKeyID == 0 {
		return types.LLMProviderTestRequest{}, types.NewErrBadRequest("apiKeyId is required")
	}
	return request, nil
}

// checkTestAPIKey accepts only the caller's own Obot API keys that can be used
// with the LLM proxy right now. Other users' keys are reported as not found so
// the endpoint can't be used to probe them.
func checkTestAPIKey(req api.Context, keyID uint) error {
	userID, err := strconv.ParseUint(req.User.GetUID(), 10, 0)
	if err != nil {
		return types.NewErrForbidden("only users can test providers with an API key")
	}
	key, err := req.GatewayClient.GetAPIKey(req.Context(), uint(userID), keyID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("API key %d not found", keyID)
	} else if err != nil {
		return fmt.Errorf("failed to get API key %d: %w", keyID, err)
	}
	switch {
	case key.RevokedAt != nil:
		return types.NewErrBadRequest("API key %q is revoked", key.Name)
	case key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now()):
		return types.NewErrBadRequest("API key %q has expired", key.Name)
	case !key.CanAccessLLMProxy:
		return types.NewErrForbidden("API key %q cannot access the LLM proxy", key.Name)
	}
	return nil
}

func modelResolutionHTTPError(err error) error {
	switch {
	case mcptester.IsModelResolutionError(err, mcptester.ModelResolutionErrorMissing):
		return types.NewErrNotFound("%v", err)
	case mcptester.IsModelResolutionError(err, mcptester.ModelResolutionErrorInactive),
		mcptester.IsModelResolutionError(err, mcptester.ModelResolutionErrorInvalid):
		return types.NewErrBadRequest("%v", err)
	default:
		return err
	}
}
