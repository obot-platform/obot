package mcpgateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	authorizationMapping     = "obot-mcp-requests-v2"
	authorizationTimeout     = 5 * time.Second
	maxAuthorizationResponse = 64 << 10
)

type authorizationEntity struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type authorizationRequest struct {
	Subject authorizationEntity `json:"subject"`
	Action  struct {
		Name       string         `json:"name"`
		Properties map[string]any `json:"properties,omitempty"`
	} `json:"action"`
	Resource authorizationEntity `json:"resource"`
	Context  struct {
		Obot struct {
			Version        int    `json:"version"`
			Mapping        string `json:"mapping"`
			Authority      string `json:"authority"`
			EvaluationID   string `json:"evaluation_id"`
			CatalogID      string `json:"catalog_id,omitempty"`
			CatalogEntryID string `json:"catalog_entry_id,omitempty"`
		} `json:"obot"`
	} `json:"context"`
}

// Credentials and transport are resolved at invocation, never put in cached hook mappings.
type hookAuthorizer struct {
	subject                   authorizationEntity
	serverID, authority       string
	catalogID, catalogEntryID string
	client                    func(string) (*http.Client, error)
	secret                    func(context.Context, string, string) (string, error)
}

type authorizationError struct {
	denied      bool
	rateLimited bool
	retryAfter  int
}

func (h *Handler) authorizer(req api.Context, server mcp.ServerConfig) *hookAuthorizer {
	return &hookAuthorizer{
		subject:        authorizationEntity{Type: "user", ID: req.User.GetUID()},
		serverID:       server.MCPServerName,
		catalogID:      server.MCPCatalogName,
		catalogEntryID: server.MCPCatalogEntryName,
		authority:      h.serverURL,
		client: func(string) (*http.Client, error) {
			if h.authorizationClient == nil {
				return nil, errors.New("authorization client unavailable")
			}
			return h.authorizationClient, nil
		},
		secret: func(ctx context.Context, filterID, endpoint string) (string, error) {
			cred, err := req.GatewayClient.RevealCredential(ctx, []string{system.MCPWebhookValidationCredentialContext}, filterID)
			if err != nil {
				return "", err
			}
			if cred.Secrets[types.MCPFilterAuthorizationURLKey] != endpoint {
				return "", &authorizationError{}
			}
			return cred.Secrets[types.MCPFilterAuthorizationSecretKey], nil
		},
	}
}

func (e *authorizationError) Error() string {
	if e.rateLimited {
		if e.retryAfter > 0 {
			return fmt.Sprintf("Rate limit exceeded. Try again in %d seconds.", e.retryAfter)
		}
		return "Rate limit exceeded. Try again later."
	}
	if e.denied {
		return "Request denied by authorization policy"
	}
	return "Authorization evaluation unavailable"
}
func hookHTTPStatus(err error) int {
	if authorizationErr, ok := errors.AsType[*authorizationError](err); ok {
		if authorizationErr.rateLimited {
			return http.StatusTooManyRequests
		}
		if authorizationErr.denied {
			return http.StatusForbidden
		}
		return http.StatusServiceUnavailable
	}
	return http.StatusOK // Preserve the existing content-filter response contract.
}

func setAuthorizationResponseHeaders(headers http.Header, err error) {
	if e, ok := errors.AsType[*authorizationError](err); ok && e.rateLimited {
		headers.Set("Cache-Control", "no-store")
		if e.retryAfter > 0 {
			headers.Set("Retry-After", fmt.Sprint(e.retryAfter))
		}
	}
}

func (a *hookAuthorizer) evaluate(ctx context.Context, hook *mcp.AuthorizationHook, message mcp.Message) (status hookStatus, err error) {
	started := time.Now()
	evaluationID := uuid.NewString()
	status = hookStatus{typeName: "authorization", method: message.Method, name: hook.FilterID, status: "error"}
	defer func() {
		status.filterID = hook.FilterID
		status.evaluationID = evaluationID
		status.mapping = authorizationMapping
		status.durationMS = max(time.Since(started).Milliseconds(), 1)
	}()
	unavailable := &authorizationError{}
	if a == nil || a.subject.ID == "" || a.serverID == "" {
		return status, unavailable
	}
	ctx, cancel := context.WithTimeout(ctx, authorizationTimeout)
	defer cancel()
	var secret string
	if hook.SigningEnabled {
		var err error
		secret, err = a.secret(ctx, hook.FilterID, hook.URL)
		if err != nil || secret == "" {
			return status, unavailable
		}
	}
	envelope := authorizationRequest{Subject: a.subject, Resource: authorizationEntity{Type: "mcp-server", ID: a.serverID}}
	envelope.Action.Name = message.Method
	properties, err := mcpHookActionProperties(message)
	if err != nil {
		return status, unavailable
	}
	envelope.Action.Properties = properties
	envelope.Context.Obot.Version = 1
	envelope.Context.Obot.Mapping = authorizationMapping
	envelope.Context.Obot.Authority = a.authority
	envelope.Context.Obot.EvaluationID = evaluationID
	envelope.Context.Obot.CatalogID = a.catalogID
	envelope.Context.Obot.CatalogEntryID = a.catalogEntryID
	body, err := json.Marshal(envelope)
	if err != nil {
		return status, unavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return status, unavailable
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if hook.SigningEnabled {
		signature := hmac.New(sha256.New, []byte(secret))
		_, _ = signature.Write(body)
		request.Header.Set("X-Obot-Signature-256", "sha256="+hex.EncodeToString(signature.Sum(nil)))
	}
	client, err := a.client(hook.URL)
	if err != nil {
		return status, unavailable
	}
	response, err := client.Do(request)
	if err != nil {
		return status, unavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return status, unavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAuthorizationResponse+1))
	if err != nil || len(data) > maxAuthorizationResponse {
		return status, unavailable
	}
	var decision struct {
		Decision *bool `json:"decision"`
		Context  struct {
			Obot struct {
				Reason            string `json:"reason"`
				RetryAfterSeconds *int   `json:"retry_after_seconds"`
			} `json:"obot"`
		} `json:"context"`
	}
	if json.Unmarshal(data, &decision) != nil || decision.Decision == nil {
		return status, unavailable
	}
	if !*decision.Decision {
		if decision.Context.Obot.Reason == "rate_limited" {
			retry := 0
			if seconds := decision.Context.Obot.RetryAfterSeconds; seconds != nil {
				if *seconds < 1 || *seconds > 86400 {
					return status, unavailable
				}
				retry = *seconds
			}
			status.status = "deny"
			return status, &authorizationError{denied: true, rateLimited: true, retryAfter: retry}
		}
		status.status = "deny"
		return status, &authorizationError{denied: true}
	}
	status.status = "allow"
	return status, nil
}

// Authorize the original client request once per attached filter, using the
// same compiled selectors as content hooks. No server messages or notifications.
func (h *hookProcessor) authorize(message mcp.Message, origin hookOrigin) hookResult {
	result := hookResult{message: message}
	if origin != hookOriginClient || message.ID == nil {
		return result
	}
	ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
	defer cancel()
	seen := map[string]bool{}
	params := map[string]string{"name": mcpHookMessageName(message), "direction": "request", "callOnError": "false", "method": message.Method}
	for _, hook := range h.hooks {
		if hook.Authorization == nil || seen[hook.Authorization.FilterID] || (!hook.Matches(message.Method, params) && !matchesGenericCallHook(hook, message.Method, params, h.resolved != nil)) {
			continue
		}
		seen[hook.Authorization.FilterID] = true
		status, err := h.authorizer.evaluate(ctx, hook.Authorization, message)
		result.statuses = append(result.statuses, status)
		if err != nil {
			result.err = err
			break
		}
	}
	return result
}

func (h *hookProcessor) hasAuthorization() bool {
	return h.hooks.HasAuthorization()
}
