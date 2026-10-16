package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type refreshTokenUpdateStorage struct {
	storage.Client
	update func(context.Context, kclient.Object, ...kclient.UpdateOption) error
}

type lostTokenResponse struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (s *refreshTokenUpdateStorage) Update(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
	return s.update(ctx, obj, opts...)
}

func (s *refreshTokenUpdateStorage) Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	// Unlike the fake client, the HTTP storage client stops reads when their context is canceled.
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Client.Get(ctx, key, obj, opts...)
}

func (w *lostTokenResponse) Write([]byte) (int, error) {
	w.cancel()
	return 0, io.ErrClosedPipe
}

func newRefreshTokenRequest(t *testing.T) (api.Context, *handler, v1.OAuthClient, string) {
	t.Helper()

	const original = "legacy-refresh-token"
	token := &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      refreshTokenHash(original),
		Spec: v1.OAuthTokenSpec{
			ClientID: "oauth-client",
			Resource: "https://obot.example.com/mcp-connect/" + system.VMCPPrefix + "test",
			MCPID:    system.VMCPPrefix + "test",
			UserID:   42,
		},
	}
	token.UID = "grant-uid"
	client, gateway, tokens := newOAuthTokenTestServices(t, token)
	return api.Context{
		Storage:        client,
		GatewayClient:  gateway,
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil).WithContext(t.Context()),
		ResponseWriter: httptest.NewRecorder(),
	}, &handler{baseURL: "https://obot.example.com", tokenService: tokens}, v1.OAuthClient{
		Namespace: system.DefaultNamespace,
		Name:      token.Spec.ClientID,
	}, original
}

func readRefreshGrant(t *testing.T, req api.Context, original string) *v1.OAuthToken {
	t.Helper()

	var token v1.OAuthToken
	require.NoError(t, req.Storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      refreshTokenHash(original),
	}, &token))
	return &token
}

func refreshForTest(t *testing.T, req api.Context, h *handler, client v1.OAuthClient, token string) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	req.ResponseWriter = recorder
	require.NoError(t, h.doRefreshToken(req, client, token))
	var response types.OAuthToken
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotEmpty(t, response.AccessToken)
	return response.RefreshToken
}

func TestRefreshTokenRecoversLostResponseAcrossHandlers(t *testing.T) {
	req, h, client, original := newRefreshTokenRequest(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	lost := req
	lost.Request = req.WithContext(ctx)
	lost.ResponseWriter = &lostTokenResponse{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	require.ErrorIs(t, h.doRefreshToken(lost, client, original), io.ErrClosedPipe)
	require.ErrorIs(t, ctx.Err(), context.Canceled)

	committed := readRefreshGrant(t, req, original)
	// Recovery uses storage and the presented secret, with no state from the first handler.
	restarted := &handler{baseURL: h.baseURL, tokenService: h.tokenService}
	next := refreshForTest(t, req, restarted, client, original)
	assert.Equal(t, committed.Spec.CurrentTokenHash, refreshTokenHash(next))
	assert.NotContains(t, committed.Spec.EncryptedCurrentToken, next)
	assert.Equal(t, next, refreshForTest(t, req, restarted, client, next))
	assert.Equal(t, next, refreshForTest(t, req, restarted, client, original))
	assert.Equal(t, committed.Spec, readRefreshGrant(t, req, original).Spec, "retries must not extend the window")

	var grants v1.OAuthTokenList
	require.NoError(t, req.Storage.List(t.Context(), &grants))
	assert.Len(t, grants.Items, 1)
}

func TestRefreshTokenConcurrentRequestsConverge(t *testing.T) {
	req, h, client, original := newRefreshTokenRequest(t)
	base := req.Storage
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var updates atomic.Int32
	req.Storage = &refreshTokenUpdateStorage{
		Client: base,
		update: func(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
			// Both requests must prepare their update from the same resourceVersion.
			if updates.Add(1) <= 2 {
				ready <- struct{}{}
				<-release
			}
			return base.Update(ctx, obj, opts...)
		},
	}
	type result struct {
		err      error
		response types.OAuthToken
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			request := req
			recorder := httptest.NewRecorder()
			request.ResponseWriter = recorder
			err := h.doRefreshToken(request, client, original)
			var response types.OAuthToken
			if err == nil {
				err = json.Unmarshal(recorder.Body.Bytes(), &response)
			}
			results <- result{err: err, response: response}
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("refresh did not reach the concurrent update")
		}
	}
	unblock()

	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotEmpty(t, first.response.RefreshToken)
	assert.Equal(t, first.response.RefreshToken, second.response.RefreshToken)
	assert.EqualValues(t, 2, updates.Load())
	assert.Equal(t, refreshTokenHash(first.response.RefreshToken), readRefreshGrant(t, req, original).Spec.CurrentTokenHash)
}

func TestRefreshTokenRotationStorageFailures(t *testing.T) {
	for _, mode := range []string{"before commit", "lost update response", "canceled after commit", "deleted before commit", "conflict exhaustion"} {
		t.Run(mode, func(t *testing.T) {
			req, h, client, original := newRefreshTokenRequest(t)
			base := req.Storage
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request := req
			request.Request = req.WithContext(ctx)
			request.Storage = &refreshTokenUpdateStorage{
				Client: base,
				update: func(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
					switch mode {
					case "before commit":
						return errors.New("storage unavailable")
					case "conflict exhaustion":
						return apierrors.NewConflict(v1.SchemeGroupVersion.WithResource("oauthtokens").GroupResource(), obj.GetName(), errors.New("conflict"))
					case "deleted before commit":
						if err := base.Delete(ctx, obj); err != nil {
							return err
						}
						return base.Update(ctx, obj, opts...)
					}
					if err := base.Update(ctx, obj, opts...); err != nil {
						return err
					}
					if mode == "canceled after commit" {
						cancel()
					}
					return io.ErrUnexpectedEOF
				},
			}
			if mode == "lost update response" {
				require.NoError(t, h.doRefreshToken(request, client, original))
			} else {
				require.Error(t, h.doRefreshToken(request, client, original))
			}

			if mode == "deleted before commit" {
				err := h.doRefreshToken(req, client, original)
				assert.ErrorContains(t, err, "invalid_grant")
				var grants v1.OAuthTokenList
				require.NoError(t, base.List(t.Context(), &grants))
				assert.Empty(t, grants.Items, "refresh must not recreate a revoked grant")
				return
			}
			stored := readRefreshGrant(t, req, original)
			next := refreshForTest(t, req, h, client, original)
			if mode == "before commit" || mode == "conflict exhaustion" {
				assert.Empty(t, stored.Spec.CurrentTokenHash)
			} else {
				assert.Equal(t, stored.Spec.CurrentTokenHash, refreshTokenHash(next))
			}
		})
	}
}

func TestRefreshTokenExpiryAndNextRotation(t *testing.T) {
	req, h, client, original := newRefreshTokenRequest(t)
	first := refreshForTest(t, req, h, client, original)
	token := readRefreshGrant(t, req, original)
	token.Spec.PreviousValidUntil.Time = time.Now().Add(-time.Second).Truncate(time.Second)
	require.NoError(t, req.Storage.Update(t.Context(), token))
	assert.ErrorContains(t, h.doRefreshToken(req, client, original), "invalid_grant")
	assert.Equal(t, token.Spec, readRefreshGrant(t, req, original).Spec)

	second := refreshForTest(t, req, h, client, first)
	assert.NotEqual(t, first, second)
	assert.Equal(t, second, refreshForTest(t, req, h, client, first))
	assert.ErrorContains(t, h.doRefreshToken(req, client, original), "invalid_grant")

	wrongClient := client
	wrongClient.Name = "another-client"
	assert.ErrorContains(t, h.doRefreshToken(req, wrongClient, second), "invalid_grant")
	assert.Equal(t, second, refreshForTest(t, req, h, client, second))
}

func TestRefreshTokenRecoveryAuthenticatesStoredState(t *testing.T) {
	req, h, client, original := newRefreshTokenRequest(t)
	next := refreshForTest(t, req, h, client, original)
	stored := readRefreshGrant(t, req, original)
	recovered, err := recoverRefreshToken(stored, original)
	require.NoError(t, err)
	assert.Equal(t, next, recovered)
	_, err = recoverRefreshToken(stored, stored.Spec.PreviousTokenHash)
	require.Error(t, err, "the stored hash must not be a usable recovery key")

	tests := []struct {
		name   string
		mutate func(*v1.OAuthToken)
	}{
		{
			name:   "ciphertext",
			mutate: func(token *v1.OAuthToken) { token.Spec.EncryptedCurrentToken = "AAAA" },
		},
		{
			name:   "UID",
			mutate: func(token *v1.OAuthToken) { token.UID = "other-grant" },
		},
		{
			name:   "client",
			mutate: func(token *v1.OAuthToken) { token.Spec.ClientID = "other-client" },
		},
		{
			name:   "current hash",
			mutate: func(token *v1.OAuthToken) { token.Spec.CurrentTokenHash = refreshTokenHash("other") },
		},
		{
			name:   "previous hash",
			mutate: func(token *v1.OAuthToken) { token.Spec.PreviousTokenHash = refreshTokenHash("other") },
		},
		{
			name: "deadline",
			mutate: func(token *v1.OAuthToken) {
				token.Spec.PreviousValidUntil.Time = token.Spec.PreviousValidUntil.Add(time.Second)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := stored.DeepCopy()
			tt.mutate(token)
			_, err := recoverRefreshToken(token, original)
			require.Error(t, err)
		})
	}

	stored.Spec.EncryptedCurrentToken = "AAAA"
	require.NoError(t, req.Storage.Update(t.Context(), stored))
	require.Error(t, h.doRefreshToken(req, client, original))
	assert.Equal(t, next, refreshForTest(t, req, h, client, next), "corrupt retry data must not revoke the current token")
}

func TestRefreshTokenRejectsMalformedTokens(t *testing.T) {
	for _, token := range []string{"v2.id.secret", "v1...", "v1." + strings.Repeat("z", 64) + "." + strings.Repeat("a", 43), "v1." + strings.Repeat("a", 64) + ".short", "v1." + strings.Repeat("a", 64) + "." + strings.Repeat("!", 43)} {
		_, err := refreshTokenName(token)
		require.Error(t, err)
	}
}

func TestRefreshTokenRejectsLargeInputWithoutAmplifyingMemory(t *testing.T) {
	token := strings.Repeat(".", 1<<20)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := refreshTokenName(token)
	runtime.ReadMemStats(&after)

	require.Error(t, err)
	// Allow ample logging overhead, but catch allocating a string entry for every separator.
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(len(token)))
}

func TestRevokeIfUserDeniedPreservesGrantOnCancellation(t *testing.T) {
	req, _, _, original := newRefreshTokenRequest(t)
	token := readRefreshGrant(t, req, original)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req.Request = req.WithContext(ctx)
	require.ErrorIs(t, revokeIfUserDenied(req, token), context.Canceled)
	assert.Equal(t, token.Spec, readRefreshGrant(t, req, original).Spec)
}
