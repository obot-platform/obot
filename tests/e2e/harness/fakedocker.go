//go:build e2e

package harness

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
)

// FakeDocker is a minimal Docker Engine API that reports no containers and refuses to run
// any. Obot's docker MCP backend talks to it instead of the machine's real daemon, so test
// replicas never see, stop or remove containers that belong to anything else on the host
// (system MCP servers share container names across Obot installations). The tests only use
// remote MCP servers, which need no containers.
type FakeDocker struct {
	// PretendContainer makes Obot believe it runs in a container on the docker bridge, so it
	// takes its address from the container instead of probing the network for one. Older
	// Obot releases can't start on a machine without a default route otherwise.
	PretendContainer bool

	server   *http.Server
	listener net.Listener
	hostname string
}

// StartFakeDocker starts the fake API on a random localhost port.
func StartFakeDocker() (*FakeDocker, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	f := &FakeDocker{listener: l, hostname: hostname}
	f.server = &http.Server{Handler: http.HandlerFunc(f.serve)}
	go func() { _ = f.server.Serve(l) }()
	return f, nil
}

// Host is the value for DOCKER_HOST.
func (f *FakeDocker) Host() string {
	return "tcp://" + f.listener.Addr().String()
}

// Close stops the fake API.
func (f *FakeDocker) Close() {
	_ = f.server.Close()
}

func (f *FakeDocker) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Api-Version", "1.43")
	path := r.URL.Path
	writeJSON := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	case strings.HasSuffix(path, "/_ping"):
		_, _ = w.Write([]byte("OK"))
	case strings.HasSuffix(path, "/containers/json"), strings.HasSuffix(path, "/networks"), strings.HasSuffix(path, "/volumes"):
		writeJSON(http.StatusOK, []any{})
	case strings.HasSuffix(path, "/events"):
		// Hold the event stream open until the client goes away.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-r.Context().Done()
	case strings.HasSuffix(path, "/version"):
		writeJSON(http.StatusOK, map[string]any{"ApiVersion": "1.43", "Version": "fake"})
	case f.PretendContainer && f.hostname != "" && strings.HasSuffix(path, "/containers/"+f.hostname+"/json"):
		writeJSON(http.StatusOK, map[string]any{
			"Id":   f.hostname,
			"Name": "/" + f.hostname,
			"NetworkSettings": map[string]any{"Networks": map[string]any{
				"bridge": map[string]any{"IPAddress": "127.0.0.1"},
			}},
		})
	case strings.HasSuffix(path, "/info"):
		writeJSON(http.StatusOK, map[string]any{"OSType": "linux"})
	default:
		// Inspecting, stopping or removing anything reports that it doesn't exist; creating
		// anything fails.
		if r.Method == http.MethodGet || r.Method == http.MethodDelete || strings.HasSuffix(path, "/stop") || strings.HasSuffix(path, "/kill") {
			writeJSON(http.StatusNotFound, map[string]string{"message": "No such object (fake docker)"})
			return
		}
		writeJSON(http.StatusInternalServerError, map[string]string{"message": "the e2e fake docker can't run containers"})
	}
}
