package apiproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

type recordingExecutor struct {
	description   Description
	result        ExecutionResult
	executeErr    error
	descriptions  int
	executions    int
	lastPrincipal kuser.Info
	lastInvoke    Invocation
}

func (e *recordingExecutor) Describe(_ context.Context, _ kuser.Info, _ Reference) (Description, error) {
	e.descriptions++
	return e.description, nil
}

func (e *recordingExecutor) Execute(_ context.Context, principal kuser.Info, invocation Invocation) (ExecutionResult, error) {
	e.executions++
	e.lastPrincipal = principal
	e.lastInvoke = invocation
	return e.result, e.executeErr
}

func testAPIContext(t *testing.T, method, target string, body []byte) api.Context {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	return api.Context{ResponseWriter: recorder, Request: req, User: &kuser.DefaultInfo{UID: "7"}}
}

func testResponse(ctx api.Context) *httptest.ResponseRecorder {
	return ctx.ResponseWriter.(*httptest.ResponseRecorder)
}

func TestHandlerExecutesUniqueMatchAndPreservesBusinessResult(t *testing.T) {
	executor := &recordingExecutor{
		description: Description{
			Operations: []OperationDescription{{
				Operation: Operation{OperationKey: "GET /items/{id}", LegacyToolName: "getItem", Method: "GET", PathTemplate: "/items/{id}"},
			}},
			HeaderPolicies: []HeaderPolicy{
				{Name: "X-Business", Forwardable: true},
				{Name: "authtoken", Credential: true},
			},
		},
		result: ExecutionResult{
			Kind:   "business",
			Status: http.StatusForbidden,
			Headers: []HeaderValue{
				{Name: "Content-Type", Values: []string{"application/problem+json"}},
				{Name: "Set-Cookie", Values: []string{"business=one", "business=two"}},
				{Name: "X-Upstream", Values: []string{"first", "second"}},
				{Name: "X-Obot-Error-Code", Values: []string{"forged"}},
				{Name: "Content-Length", Values: []string{"999"}},
				{Name: "Connection", Values: []string{"close"}},
			},
			Body: []byte("business error"),
		},
	}
	handler := &Handler{Executor: executor}
	ctx := testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/it%65ms/a%252Fb?z=2&a=1&a=1", nil)
	ctx.Request.Header.Set("Authorization", "Bearer caller")
	ctx.Request.Header.Set("X-API-Key", "caller-key")
	ctx.Request.Header.Set("authtoken", "caller-credential")
	ctx.Request.Header.Add("X-Business", "one")
	ctx.Request.Header.Add("X-Business", "two")

	require.NoError(t, handler.Proxy(ctx))

	require.Equal(t, 1, executor.descriptions)
	require.Equal(t, 1, executor.executions)
	require.Equal(t, "7", executor.lastPrincipal.GetUID())
	require.Equal(t, "GET /items/{id}", executor.lastInvoke.OperationKey)
	require.Equal(t, "/it%65ms/a%252Fb", executor.lastInvoke.RawPath)
	require.Equal(t, "z=2&a=1&a=1", executor.lastInvoke.RawQuery)
	require.Equal(t, []string{"one", "two"}, executor.lastInvoke.Headers.Values("X-Business"))
	require.Empty(t, executor.lastInvoke.Headers.Get("Authorization"))
	require.Empty(t, executor.lastInvoke.Headers.Get("X-API-Key"))
	require.Empty(t, executor.lastInvoke.Headers.Get("authtoken"))

	response := testResponse(ctx)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
	require.Equal(t, []string{"business=one", "business=two"}, response.Header().Values("Set-Cookie"))
	require.Equal(t, []string{"first", "second"}, response.Header().Values("X-Upstream"))
	require.Empty(t, response.Header().Get("X-Obot-Error-Code"))
	require.Empty(t, response.Header().Get("Content-Length"))
	require.Empty(t, response.Header().Get("Connection"))
	require.Equal(t, "business error", response.Body.String())
}

func TestHandlerRejectsBeforeExecuteAndClassifies405(t *testing.T) {
	executor := &recordingExecutor{
		description: Description{Operations: []OperationDescription{{
			Operation: Operation{OperationKey: "GET /items", Method: "GET", PathTemplate: "/items"},
		}}},
	}
	handler := &Handler{Executor: executor}

	ctx := testAPIContext(t, http.MethodPost, "/api-proxy/vmcp1abc/component/items", []byte("{}"))
	require.NoError(t, handler.Proxy(ctx))
	require.Equal(t, http.StatusMethodNotAllowed, testResponse(ctx).Code)
	require.Equal(t, "GET", ctx.Request.Header.Get("Allow"))
	require.Zero(t, executor.executions)

	ctx = testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/missing", nil)
	require.NoError(t, handler.Proxy(ctx))
	require.Equal(t, http.StatusNotFound, testResponse(ctx).Code)
	require.Zero(t, executor.executions)
}

func TestHandlerRejectsAmbiguousOperationBeforeBody(t *testing.T) {
	executor := &recordingExecutor{description: Description{Operations: []OperationDescription{
		{Operation: Operation{OperationKey: "GET /items/{first}", Method: "GET", PathTemplate: "/items/{first}"}},
		{Operation: Operation{OperationKey: "GET /items/{second}", Method: "GET", PathTemplate: "/items/{second}"}},
	}}}
	handler := &Handler{Executor: executor}
	ctx := testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/items/value", []byte("too late"))

	require.NoError(t, handler.Proxy(ctx))

	response := testResponse(ctx)
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Equal(t, "ambiguous_operation", response.Header().Get("X-Obot-Error-Code"))
	require.NotEmpty(t, response.Header().Get("X-Obot-Request-ID"))
	require.Zero(t, executor.executions)
	var errorBody struct {
		RequestID string `json:"requestID"`
		Retryable bool   `json:"retryable"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &errorBody))
	require.Equal(t, response.Header().Get("X-Obot-Request-ID"), errorBody.RequestID)
	require.False(t, errorBody.Retryable)
}

func TestHandlerRejectsSSEOnlyAccept(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   bool
	}{
		{name: "SSE only", accept: "text/event-stream", want: true},
		{name: "SSE with parameters", accept: "text/event-stream; charset=utf-8;q=1", want: true},
		{name: "mixed JSON and SSE", accept: "application/json, text/event-stream;q=0.1", want: false},
		{name: "SSE explicitly disabled", accept: "text/event-stream;q=0", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, acceptRequiresEventStream(tt.accept))
		})
	}

	executor := &recordingExecutor{description: Description{Operations: []OperationDescription{{
		Operation: Operation{OperationKey: "GET /items", Method: "GET", PathTemplate: "/items"},
	}}}}
	handler := &Handler{Executor: executor}
	ctx := testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/items", nil)
	ctx.Request.Header.Set("Accept", "text/event-stream")

	require.NoError(t, handler.Proxy(ctx))

	response := testResponse(ctx)
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Equal(t, "upstream_protocol_unsupported", response.Header().Get("X-Obot-Error-Code"))
	require.Zero(t, executor.executions)
}

func TestHandlerRejectsRequestBodyOverLimit(t *testing.T) {
	executor := &recordingExecutor{description: Description{Operations: []OperationDescription{{
		Operation: Operation{OperationKey: "POST /items", Method: http.MethodPost, PathTemplate: "/items"},
	}}}}
	handler := &Handler{Executor: executor}
	ctx := testAPIContext(t, http.MethodPost, "/api-proxy/vmcp1abc/component/items", bytes.Repeat([]byte{'x'}, MaxRequestBodyBytes+1))

	require.NoError(t, handler.Proxy(ctx))

	response := testResponse(ctx)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.Equal(t, "request_too_large", response.Header().Get("X-Obot-Error-Code"))
	require.Zero(t, executor.executions)
}

func TestHandlerRejectsResponseOverLimit(t *testing.T) {
	executor := &recordingExecutor{
		description: Description{Operations: []OperationDescription{{
			Operation: Operation{OperationKey: "GET /items", Method: http.MethodGet, PathTemplate: "/items"},
		}}},
		result: ExecutionResult{Kind: "business", Status: http.StatusOK, Body: bytes.Repeat([]byte{'x'}, MaxResponseBodyBytes+1)},
	}
	handler := &Handler{Executor: executor}
	ctx := testAPIContext(t, http.MethodGet, "/api-proxy/vmcp1abc/component/items", nil)

	require.NoError(t, handler.Proxy(ctx))

	response := testResponse(ctx)
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Equal(t, "response_too_large", response.Header().Get("X-Obot-Error-Code"))
	require.Equal(t, "response_too_large", errorRequestBody(t, response).Code)
	require.Equal(t, 1, executor.executions)
}

func TestHandlerPlatformErrorHasRequestIDAndHEADHasNoBody(t *testing.T) {
	executor := &recordingExecutor{
		description: Description{Operations: []OperationDescription{{
			Operation: Operation{OperationKey: "HEAD /items", Method: http.MethodHead, PathTemplate: "/items"},
		}}},
		executeErr: errors.New("internal failure"),
	}
	handler := &Handler{Executor: executor}
	ctx := testAPIContext(t, http.MethodHead, "/api-proxy/vmcp1abc/component/items", nil)

	require.NoError(t, handler.Proxy(ctx))

	response := testResponse(ctx)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Equal(t, "platform_error", response.Header().Get("X-Obot-Error-Code"))
	require.NotEmpty(t, response.Header().Get("X-Obot-Request-ID"))
	require.Empty(t, response.Body.String())
}
