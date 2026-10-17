package llm

import (
	"encoding/json"
	"net/http"
	"strings"
)

const (
	// ProxyPathPrefix is where Obot serves its LLM proxy.
	ProxyPathPrefix = "/api/llm-proxy/"
)

// IsProxyPath reports whether path is on Obot's LLM proxy.
func IsProxyPath(path string) bool {
	return strings.HasPrefix(path, ProxyPathPrefix)
}

// ErrorDialect is the API format an LLM proxy request expects its errors in: Anthropic's for
// the Messages API and for Anthropic clients (which send anthropic-version on every request,
// including model listing), and OpenAI's otherwise.
func ErrorDialect(req *http.Request) Dialect {
	path := strings.TrimSuffix(req.URL.Path, "/")
	if strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/messages/count_tokens") ||
		strings.HasPrefix(path, ProxyPathPrefix+"anthropic/") || req.Header.Get("anthropic-version") != "" {
		return DialectAnthropicMessages
	}
	return DialectOpenAIChatCompletions
}

// WriteError writes an error response to an LLM proxy request in the API format the request
// expects, so API clients and SDKs report the error as they would the provider's own.
func WriteError(w http.ResponseWriter, req *http.Request, message string, code int) {
	var body any
	if ErrorDialect(req) == DialectAnthropicMessages {
		body = map[string]any{
			"type":  "error",
			"error": map[string]any{"type": anthropicErrorType(code), "message": message},
		}
	} else {
		body = map[string]any{
			"error": map[string]any{"message": message, "type": openAIErrorType(code), "param": nil, "code": nil},
		}
	}
	data, _ := json.Marshal(body)
	h := w.Header()
	h.Del("Content-Length")
	h.Set("Content-Type", "application/json")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_, _ = w.Write(append(data, '\n'))
}

// anthropicErrorType is the error type Anthropic's API uses for an HTTP status.
func anthropicErrorType(code int) string {
	switch code {
	case http.StatusBadRequest, http.StatusGone, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable, 529:
		return "overloaded_error"
	}
	if code < http.StatusInternalServerError {
		return "invalid_request_error"
	}
	return "api_error"
}

// openAIErrorType is the error type OpenAI's API uses for an HTTP status.
func openAIErrorType(code int) string {
	switch code {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_denied"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	}
	if code < http.StatusInternalServerError {
		return "invalid_request_error"
	}
	return "server_error"
}
