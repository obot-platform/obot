package apiproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/obot-platform/obot/pkg/system"
)

const RoutePattern = "/api-proxy/{vmcp_id}/{component_id}/{path...}"

type Route struct {
	VMCPID      string
	ComponentID string
	RawPath     string
}

func ParseRoute(req *http.Request) (Route, error) {
	escapedPath := req.URL.EscapedPath()
	remainder, ok := strings.CutPrefix(escapedPath, "/api-proxy/")
	if !ok {
		return Route{}, NewError("invalid_request", "invalid API proxy route")
	}
	firstSlash := strings.IndexByte(remainder, '/')
	if firstSlash < 1 {
		return Route{}, NewError("invalid_request", "API proxy route not found")
	}
	vmcpSegment := remainder[:firstSlash]
	remainder = remainder[firstSlash+1:]
	secondSlash := strings.IndexByte(remainder, '/')
	var componentSegment, pathSegment string
	if secondSlash < 0 {
		componentSegment = remainder
	} else {
		componentSegment = remainder[:secondSlash]
		pathSegment = remainder[secondSlash:]
	}

	vmcpID, err := decodeRouteSegment(vmcpSegment)
	if err != nil {
		return Route{}, NewError("invalid_request", "invalid API proxy VMCP segment")
	}
	if !system.IsVMCPID(vmcpID) {
		return Route{}, NewError("resource_hidden", "API proxy VMCP not found")
	}
	componentID, err := decodeRouteSegment(componentSegment)
	if err != nil || componentID == "" {
		return Route{}, NewError("invalid_request", "invalid API proxy component segment")
	}
	if pathSegment != "" {
		if err := validateRawPath(pathSegment); err != nil {
			return Route{}, NewError("invalid_request", "invalid API proxy path")
		}
	}
	return Route{VMCPID: vmcpID, ComponentID: componentID, RawPath: pathSegment}, nil
}

func validateRawPath(rawPath string) error {
	if !strings.HasPrefix(rawPath, "/") || strings.HasSuffix(rawPath, "/") || strings.Contains(rawPath, "//") {
		return errors.New("empty path segment")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(rawPath, "/"), "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || !utf8.ValidString(decoded) || decoded == "." || decoded == ".." ||
			strings.ContainsAny(decoded, "\x00\r\n\t") {
			return errors.New("invalid path segment")
		}
	}
	return nil
}

func decodeRouteSegment(segment string) (string, error) {
	if segment == "" {
		return "", errors.New("empty segment")
	}
	decoded, err := url.PathUnescape(segment)
	if err != nil || !utf8.ValidString(decoded) || strings.ContainsAny(decoded, "\x00\r\n\t") {
		return "", errors.New("invalid segment")
	}
	if decoded == "." || decoded == ".." || strings.Contains(decoded, "/") {
		return "", errors.New("invalid segment")
	}
	return decoded, nil
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestID,omitempty"`
	Retryable bool   `json:"retryable"`
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteErrorWithRequestMethod(w, http.MethodGet, status, code, message, "")
}

func WriteErrorWithRequestMethod(w http.ResponseWriter, method string, status int, code, message, requestID string) {
	if requestID == "" {
		requestID = NewRequestID()
	}
	body, err := json.Marshal(ErrorBody{
		Code:      code,
		Message:   message,
		RequestID: requestID,
		Retryable: errorRetryable(code),
	})
	if err != nil {
		body = []byte(fmt.Sprintf(`{"code":"internal_error","message":%q,"requestID":%q,"retryable":false}`, "platform error", requestID))
		status = http.StatusInternalServerError
		code = "internal_error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Obot-Error-Code", code)
	w.Header().Set("X-Obot-Request-ID", requestID)
	w.WriteHeader(status)
	if method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func errorRetryable(code string) bool {
	switch code {
	case "budget_rejected", "budget_identity_unavailable", "worker_unavailable", "deadline_exceeded",
		"response_too_large", "transport_uncertain", "snapshot_required", "snapshot_invalid":
		return true
	default:
		return false
	}
}
