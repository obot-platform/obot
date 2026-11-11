package apiproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRoutePreservesRawPathAndIDs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api-proxy/vmcp1abc/component%20one/items/a%2Fb?b=2&a=1&&a=", nil)

	route, err := ParseRoute(req)

	require.NoError(t, err)
	require.Equal(t, "vmcp1abc", route.VMCPID)
	require.Equal(t, "component one", route.ComponentID)
	require.Equal(t, "/items/a%2Fb", route.RawPath)
	require.Equal(t, "b=2&a=1&&a=", req.URL.RawQuery)
}

func TestParseRouteRejectsUnsafeSegments(t *testing.T) {
	for _, tt := range []struct {
		name string
		path string
	}{
		{name: "non-canonical VMCP ID", path: "/api-proxy/not-vmcp/component/items"},
		{name: "encoded slash in ID", path: "/api-proxy/vmcp1abc%2Fother/component/items"},
		{name: "empty component", path: "/api-proxy/vmcp1abc//items"},
		{name: "empty business segment", path: "/api-proxy/vmcp1abc/component/items//a"},
		{name: "trailing slash", path: "/api-proxy/vmcp1abc/component/items/"},
		{name: "dot segment", path: "/api-proxy/vmcp1abc/component/%2E"},
		{name: "parent segment", path: "/api-proxy/vmcp1abc/component/items/%2E%2E"},
		{name: "control character", path: "/api-proxy/vmcp1abc/component/items/a%00b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRoute(httptest.NewRequest(http.MethodGet, tt.path, nil))
			require.Error(t, err)
		})
	}
}

func TestParseRouteAllowsEmptyBusinessPath(t *testing.T) {
	route, err := ParseRoute(httptest.NewRequest(http.MethodGet, "/api-proxy/vmcp1abc/component", nil))

	require.NoError(t, err)
	require.Equal(t, "vmcp1abc", route.VMCPID)
	require.Equal(t, "component", route.ComponentID)
	require.Empty(t, route.RawPath)
}

func TestWriteError(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteError(recorder, http.StatusBadGateway, "upstream_protocol_unsupported", "upstream protocol is unsupported")

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, "upstream_protocol_unsupported", recorder.Header().Get("X-Obot-Error-Code"))
	require.Equal(t, recorder.Header().Get("X-Obot-Request-ID"), errorRequestBody(t, recorder).RequestID)
	require.False(t, errorRequestBody(t, recorder).Retryable)
	require.Equal(t, "upstream_protocol_unsupported", errorRequestBody(t, recorder).Code)
	require.Equal(t, "upstream protocol is unsupported", errorRequestBody(t, recorder).Message)
}

func TestWriteErrorHEADHasNoBody(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteErrorWithRequestMethod(recorder, http.MethodHead, http.StatusBadGateway, "upstream_protocol_unsupported", "event streams are unsupported", "request-1")

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, "request-1", recorder.Header().Get("X-Obot-Request-ID"))
	require.Empty(t, recorder.Body.String())
}

func errorRequestBody(t *testing.T, recorder *httptest.ResponseRecorder) ErrorBody {
	t.Helper()
	var body ErrorBody
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}
