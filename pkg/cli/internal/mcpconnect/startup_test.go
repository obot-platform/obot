package mcpconnect

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type probeOAuthHandler struct {
	auth.OAuthHandler
	status int
}

func (*probeOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return nil, nil
}

func (h *probeOAuthHandler) Authorize(_ context.Context, _ *http.Request, response *http.Response) error {
	h.status = response.StatusCode
	return nil
}

func TestAuthenticateResponseStatuses(t *testing.T) {
	for _, status := range []int{200, 400, 401, 403, 404, 405, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()
			handler := &probeOAuthHandler{}
			err := authenticate(t.Context(), server.URL, handler, server.Client())
			if status == http.StatusInternalServerError {
				require.ErrorContains(t, err, "HTTP 500")
			} else {
				require.NoError(t, err)
			}
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				require.Equal(t, status, handler.status)
			} else {
				require.Zero(t, handler.status)
			}
		})
	}
}
