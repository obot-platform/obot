package server

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// hijackableRecorder stands in for the connection the http server provides,
// which httptest.ResponseRecorder alone cannot hijack.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

type pushRecorder struct {
	*httptest.ResponseRecorder
	target string
}

func (r *pushRecorder) Push(target string, _ *http.PushOptions) error {
	r.target = target
	return nil
}

func (r *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.hijacked = true
	return nil, nil, nil
}

// The terminal is a websocket under /api/, so it passes through both response
// writer wrappers before it upgrades. A wrapper that hides the Hijacker breaks
// the upgrade outright, and only at runtime -- nothing about the types says so.
func TestResponseWritersStayHijackable(t *testing.T) {
	for _, tt := range []struct {
		name string
		wrap func(http.ResponseWriter) http.ResponseWriter
	}{
		{
			name: "headers",
			wrap: func(rw http.ResponseWriter) http.ResponseWriter {
				return &headersResponseWriter{ResponseWriter: rw}
			},
		},
		{
			name: "audit",
			wrap: func(rw http.ResponseWriter) http.ResponseWriter {
				return &responseWriter{ResponseWriter: rw}
			},
		},
		{
			name: "audit over headers",
			wrap: func(rw http.ResponseWriter) http.ResponseWriter {
				return &responseWriter{ResponseWriter: &headersResponseWriter{ResponseWriter: rw}}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}

			if _, _, err := http.NewResponseController(tt.wrap(recorder)).Hijack(); err != nil {
				t.Fatalf("Hijack through the wrapper: %v", err)
			}
			if !recorder.hijacked {
				t.Error("the hijack did not reach the underlying writer")
			}
		})
	}
}

func TestTracedResponseWritersStayHijackable(t *testing.T) {
	for _, path := range []string{"/api/terminal", "/api/healthz", "/_app/immutable/app.js"} {
		t.Run(path, func(t *testing.T) {
			recorder := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
			handler := traced(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				wrapped := &responseWriter{ResponseWriter: &headersResponseWriter{ResponseWriter: w}}
				if _, _, err := http.NewResponseController(wrapped).Hijack(); err != nil {
					t.Fatalf("hijack through tracing: %v", err)
				}
			}))
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if !recorder.hijacked {
				t.Fatal("tracing hid the underlying hijacker")
			}
		})
	}
}

func TestTracedResponseWritersPreservePush(t *testing.T) {
	recorder := &pushRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler := traced(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := (&headersResponseWriter{ResponseWriter: w}).Push("/asset.js", nil); err != nil {
			t.Errorf("push through tracing: %v", err)
		}
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/stream", nil))
	if recorder.target != "/asset.js" {
		t.Fatal("push did not reach the underlying writer")
	}
	unsupported := traced(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := (&headersResponseWriter{ResponseWriter: w}).Push("/asset.js", nil); err != http.ErrNotSupported {
			t.Errorf("push on unsupported writer: %v", err)
		}
	}))
	unsupported.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/stream", nil))
}
