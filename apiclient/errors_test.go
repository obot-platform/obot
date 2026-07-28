package apiclient

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

// trackedBody reports whether the response body was closed.
type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestErrFromResponse(t *testing.T) {
	const htmlPage = `<!doctype html><html><head><title>404</title></head><body>`

	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantMax int
	}{
		{
			name:   "an API error document is preserved",
			status: http.StatusBadRequest,
			body:   `{"error":"enforcement allowlist server entry 0 must identify a server"}`,
			want:   `{"error":"enforcement allowlist server entry 0 must identify a server"}`,
		},
		{
			name:   "surrounding whitespace is trimmed",
			status: http.StatusForbidden,
			body:   "\n  forbidden  \n",
			want:   "forbidden",
		},
		{
			// Without a body there is nothing to report but the status line.
			name:   "an empty body falls back to the status",
			status: http.StatusUnauthorized,
			body:   "",
			want:   "401 Unauthorized",
		},
		{
			// The case this bound exists for: a wrong base path answers with a whole
			// page, and callers render these messages for people and models.
			name:    "a long page is truncated",
			status:  http.StatusNotFound,
			body:    htmlPage + strings.Repeat("padding ", 100_000),
			want:    htmlPage,
			wantMax: maxErrorBodyBytes + len("…"),
		},
		{
			// Multibyte content cut mid-rune must still read as text.
			name:    "a multibyte page is truncated on a rune boundary",
			status:  http.StatusBadGateway,
			body:    strings.Repeat("é", maxErrorBodyBytes),
			want:    "é",
			wantMax: maxErrorBodyBytes + len("…"),
		},
		{
			name:   "a binary body is described, not rendered",
			status: http.StatusInternalServerError,
			body:   "\xff\xfe\x00\x01binary",
			want:   "non-text response body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tt.body)}
			err := errFromResponse(&http.Response{
				StatusCode: tt.status,
				// Shaped like net/http's own: "401 Unauthorized".
				Status: fmt.Sprintf("%d %s", tt.status, http.StatusText(tt.status)),
				Body:   body,
			})

			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) {
				t.Fatalf("err = %T (%v), want *types.ErrHTTP", err, err)
			}
			if httpErr.Code != tt.status {
				t.Errorf("Code = %d, want %d", httpErr.Code, tt.status)
			}
			if !strings.Contains(httpErr.Message, tt.want) {
				t.Errorf("Message = %q, want it to contain %q", truncateForLog(httpErr.Message), tt.want)
			}
			if tt.wantMax > 0 {
				if got := len(httpErr.Message); got > tt.wantMax {
					t.Errorf("Message is %d bytes, want at most %d", got, tt.wantMax)
				}
				if !strings.HasSuffix(httpErr.Message, "…") {
					t.Errorf("a truncated message does not say so: %q", truncateForLog(httpErr.Message))
				}
			}
			// Callers get no response on this path, so this is the only chance to
			// close the body.
			if !body.closed {
				t.Error("the response body was left open")
			}
		})
	}
}

// The status-line fallback needs the real Status, which only a live response
// carries; this also covers the whole path from doRequest.
func TestNonSuccessSurfacesBoundedErrHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, strings.Repeat("padding ", 100_000))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	_, resp, err := c.doRequest(t.Context(), http.MethodGet, "/whatever", nil)
	if resp != nil {
		t.Error("a non-2xx returned a response alongside the error")
	}

	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("err = %T (%v), want *types.ErrHTTP", err, err)
	}
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("Code = %d, want 404", httpErr.Code)
	}
	if len(httpErr.Message) > maxErrorBodyBytes+len("…") {
		t.Errorf("Message is %d bytes, want it bounded", len(httpErr.Message))
	}
	if len(err.Error()) > maxErrorBodyBytes+128 {
		t.Errorf("Error() is %d bytes, want it bounded", len(err.Error()))
	}
}

func truncateForLog(s string) string {
	if len(s) <= 120 {
		return s
	}
	return s[:120] + "…"
}
