package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestRemoteOAuthContextBlocksLoopbackRefresh(t *testing.T) {
	var requests atomic.Int32
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(tokenEndpoint.Close)

	ctx := remoteOAuthContext(context.Background(), RemoteMCPURLValidationConfig{}, nil)
	httpClient, ok := ctx.Value(oauth2.HTTPClient).(*http.Client)
	require.True(t, ok)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint.URL, nil)
	require.NoError(t, err)
	_, err = httpClient.Do(request)
	require.Error(t, err)
	require.Zero(t, requests.Load())
}
