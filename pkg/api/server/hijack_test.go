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
		{"headers", func(rw http.ResponseWriter) http.ResponseWriter {
			return &headersResponseWriter{ResponseWriter: rw}
		}},
		{"audit", func(rw http.ResponseWriter) http.ResponseWriter {
			return &responseWriter{ResponseWriter: rw}
		}},
		{"audit over headers", func(rw http.ResponseWriter) http.ResponseWriter {
			return &responseWriter{ResponseWriter: &headersResponseWriter{ResponseWriter: rw}}
		}},
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
