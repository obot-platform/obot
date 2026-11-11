package apiproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	MaxRequestBodyBytes  = 1 << 20
	MaxResponseBodyBytes = 8 << 20
	requestTimeout       = 30 * time.Second
)

type Reference struct {
	VMCPID      string
	ComponentID string
}

type OperationDescription struct {
	Operation
	DisplayName       string
	Description       string
	Parameters        []ParameterDocumentation
	RequestMediaTypes []string
}

type ParameterDocumentation struct {
	Name     string          `json:"name"`
	In       string          `json:"in"`
	Required bool            `json:"required,omitempty"`
	Schema   json.RawMessage `json:"schema,omitempty"`
}

type Description struct {
	ExecutionDigest string
	Operations      []OperationDescription
	HeaderPolicies  []HeaderPolicy
}

type Invocation struct {
	Reference    Reference
	OperationKey string
	Method       string
	RawPath      string
	RawQuery     string
	Headers      http.Header
	Body         []byte
	BodyMimeType string
	Deadline     time.Time
}

type HeaderValue struct {
	Name   string
	Values []string
}

type ExecutionResult struct {
	Kind      string
	RequestID string
	Status    int
	Headers   []HeaderValue
	Body      []byte
	Outcome   string
}

type Executor interface {
	Describe(ctx context.Context, principal kuser.Info, reference Reference) (Description, error)
	Execute(ctx context.Context, principal kuser.Info, invocation Invocation) (ExecutionResult, error)
}

type Handler struct {
	Executor Executor
	Audit    *AuditWriter
}

func (h *Handler) Proxy(req api.Context) error {
	started := time.Now()
	route, err := ParseRoute(req.Request)
	if err != nil {
		writeHandlerError(req.ResponseWriter, req.Request.Method, err)
		return nil
	}
	if acceptRequiresEventStream(req.Request.Header.Get("Accept")) {
		writeRequestError(req, http.StatusBadGateway, "upstream_protocol_unsupported", "event streams are unsupported")
		return nil
	}

	description, err := h.Executor.Describe(req.Context(), req.User, Reference{VMCPID: route.VMCPID, ComponentID: route.ComponentID})
	if err != nil {
		writeHandlerError(req.ResponseWriter, req.Request.Method, err)
		return nil
	}
	operations := make([]Operation, 0, len(description.Operations))
	for _, candidate := range description.Operations {
		operations = append(operations, candidate.Operation)
	}
	match, matchError := MatchOperations(operations, req.Method, route.RawPath)
	if matchError != "" {
		if MethodExistsForPath(operations, req.Method, route.RawPath) {
			allow := allowedMethodsForPath(operations, req.Method, route.RawPath)
			req.Request.Header.Set("Allow", strings.Join(allow, ", "))
			writeRequestError(req, http.StatusMethodNotAllowed, "method_not_allowed", "method is not published for this path")
			return nil
		}
		status := http.StatusNotFound
		message := "operation not found"
		if matchError == "ambiguous_operation" {
			message = "multiple operations match the method and path"
		} else if matchError == "invalid_path" {
			status = http.StatusBadRequest
			message = "invalid raw path"
		}
		writeRequestError(req, status, matchError, message)
		return nil
	}

	body, err := readBody(req)
	if err != nil {
		writeHandlerError(req.ResponseWriter, req.Request.Method, err)
		return nil
	}
	invocation := Invocation{
		Reference:    Reference{VMCPID: route.VMCPID, ComponentID: route.ComponentID},
		OperationKey: match.Operation.OperationKey,
		Method:       req.Method,
		RawPath:      route.RawPath,
		RawQuery:     req.URL.RawQuery,
		Headers:      SanitizeInputHeaders(req.Request.Header, description.HeaderPolicies),
		Body:         body,
		BodyMimeType: req.Request.Header.Get("Content-Type"),
		Deadline:     time.Now().Add(requestTimeout),
	}
	result, err := h.Executor.Execute(req.Context(), req.User, invocation)
	if err != nil {
		h.Audit.Record(req, route, match.Operation, nil, err, started)
		writeHandlerError(req.ResponseWriter, req.Request.Method, err)
		return nil
	}
	if result.Kind != "business" || result.Status < 100 || result.Status > 599 {
		h.Audit.Record(req, route, match.Operation, &result, nil, started)
		writeRequestError(req, http.StatusBadGateway, "protocol_error", "worker returned an invalid result")
		return nil
	}
	if len(result.Body) > MaxResponseBodyBytes {
		h.Audit.Record(req, route, match.Operation, &result, nil, started)
		writeRequestError(req, http.StatusBadGateway, "response_too_large", "worker response exceeds 8 MiB")
		return nil
	}
	h.Audit.Record(req, route, match.Operation, &result, nil, started)
	WriteBusinessHeaders(req.ResponseWriter.Header(), result.Headers)
	req.WriteHeader(result.Status)
	if req.Method != http.MethodHead {
		_, _ = req.ResponseWriter.Write(result.Body)
	}
	return nil
}

func readBody(req api.Context) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(req.ResponseWriter, req.Request.Body, MaxRequestBodyBytes+1))
	if err != nil || len(body) > MaxRequestBodyBytes {
		return nil, NewError("request_too_large", "request body exceeds 1 MiB")
	}
	return body, nil
}

func writeRequestError(req api.Context, status int, code, message string) {
	WriteErrorWithRequestMethod(req.ResponseWriter, req.Request.Method, status, code, message, "")
}

func acceptRequiresEventStream(accept string) bool {
	sawEventStream := false
	for _, entry := range strings.Split(accept, ",") {
		mediaType, parameters, _ := strings.Cut(entry, ";")
		mediaType = strings.TrimSpace(mediaType)
		if mediaType == "" {
			continue
		}
		if strings.EqualFold(mediaType, "text/event-stream") {
			if !acceptQualityZero(parameters) {
				sawEventStream = true
			}
			continue
		}
		if !acceptQualityZero(parameters) {
			return false
		}
	}
	return sawEventStream
}

func acceptQualityZero(parameters string) bool {
	for _, parameter := range strings.Split(parameters, ";") {
		name, value, _ := strings.Cut(parameter, "=")
		if strings.EqualFold(strings.TrimSpace(name), "q") {
			quality, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			return err == nil && quality == 0
		}
	}
	return false
}

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
	RequestID  string
	Retryable  bool
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: errorStatus(code)}
}

func errorStatus(code string) int {
	switch code {
	case "platform_unauthenticated":
		return http.StatusUnauthorized
	case "platform_forbidden", "resource_hidden":
		return http.StatusForbidden
	case "operation_not_found":
		return http.StatusNotFound
	case "business_authorization_required":
		return http.StatusConflict
	case "request_too_large":
		return http.StatusRequestEntityTooLarge
	case "budget_rejected":
		return http.StatusTooManyRequests
	case "budget_identity_unavailable", "worker_unavailable":
		return http.StatusServiceUnavailable
	case "deadline_exceeded":
		return http.StatusGatewayTimeout
	case "unsupported_media_type", "upstream_protocol_unsupported", "response_too_large":
		return http.StatusBadGateway
	case "transport_uncertain", "snapshot_required", "snapshot_invalid":
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

func writeHandlerError(w http.ResponseWriter, method string, err error) {
	var proxyError *Error
	if !errors.As(err, &proxyError) {
		proxyError = NewError("platform_error", "platform error")
		proxyError.HTTPStatus = http.StatusInternalServerError
	}
	if proxyError.RequestID == "" {
		proxyError.RequestID = NewRequestID()
	}
	WriteErrorWithRequestMethod(w, method, proxyError.HTTPStatus, proxyError.Code, proxyError.Message, proxyError.RequestID)
}

func NewRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "request-unavailable"
	}
	return hex.EncodeToString(value[:])
}

func allowedMethodsForPath(operations []Operation, method, rawPath string) []string {
	var result []string
	for _, operation := range operations {
		if strings.EqualFold(operation.Method, method) || !MethodExistsForPath([]Operation{operation}, method, rawPath) {
			continue
		}
		if !containsFold(result, operation.Method) {
			result = append(result, strings.ToUpper(operation.Method))
		}
	}
	return result
}

func containsFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}
