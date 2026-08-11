//go:build integration

package harness

import (
	"net/http"
	"testing"
	"time"
)

func TestNonStreamingRequestsUseDeadline(t *testing.T) {
	for _, request := range []struct {
		name string
		call func(*Harness, *testing.T)
	}{
		{
			name: "get",
			call: func(h *Harness, t *testing.T) {
				h.Get(t, "/", nil)
			},
		},
		{
			name: "status",
			call: func(h *Harness, t *testing.T) {
				h.Status(t, http.MethodGet, "/")
			},
		},
	} {
		t.Run(request.name, func(t *testing.T) {
			h := &Harness{
				BaseURL: "http://example.test",
				HTTP: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					deadline, ok := req.Context().Deadline()
					if !ok || time.Until(deadline) > requestTimeout {
						t.Error("request context has no bounded deadline")
					}
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
				})},
			}
			request.call(h, t)
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
