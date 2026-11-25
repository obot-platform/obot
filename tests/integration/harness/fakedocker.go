//go:build integration

package harness

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
)

// FakeDocker is a Docker Engine API with no containers that can't run any, so Obot's Docker MCP backend starts
// without a Docker daemon and never touches the machine's containers. Remote and vMCP servers, which Obot reaches
// directly, still work.
type FakeDocker struct {
	*httptest.Server
}

// StartFakeDocker starts a FakeDocker. Point DOCKER_HOST at Host before StartApp.
func StartFakeDocker() *FakeDocker {
	return &FakeDocker{Server: httptest.NewServer(http.HandlerFunc(serveFakeDocker))}
}

// Host is the value for DOCKER_HOST.
func (f *FakeDocker) Host() string {
	return "tcp://" + f.Listener.Addr().String()
}

func serveFakeDocker(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Api-Version", "1.43")
	writeJSON := func(code int, v any) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(code)
		_ = json.NewEncoder(rw).Encode(v)
	}
	path := req.URL.Path
	switch {
	case strings.HasSuffix(path, "/_ping"):
		_, _ = rw.Write([]byte("OK"))
	case strings.HasSuffix(path, "/containers/json"), strings.HasSuffix(path, "/networks"), strings.HasSuffix(path, "/volumes"):
		writeJSON(http.StatusOK, []any{})
	case strings.HasSuffix(path, "/events"):
		// Hold the event stream open until Obot goes away.
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		if flusher, ok := rw.(http.Flusher); ok {
			flusher.Flush()
		}
		<-req.Context().Done()
	case strings.HasSuffix(path, "/version"):
		writeJSON(http.StatusOK, map[string]any{"ApiVersion": "1.43", "Version": "fake"})
	case strings.HasSuffix(path, "/info"):
		writeJSON(http.StatusOK, map[string]any{"OSType": "linux"})
	case req.Method == http.MethodGet, req.Method == http.MethodDelete, strings.HasSuffix(path, "/stop"), strings.HasSuffix(path, "/kill"):
		writeJSON(http.StatusNotFound, map[string]string{"message": "No such object (fake Docker)"})
	default:
		writeJSON(http.StatusInternalServerError, map[string]string{"message": "the fake Docker can't run containers"})
	}
}
