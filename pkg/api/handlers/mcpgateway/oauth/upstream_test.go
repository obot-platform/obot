package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestCapturedOAuthHandoffAndAllComponentCompletion(t *testing.T) {
	testCapturedOAuthFlow(t, 12, false, false, false)
}

func TestCapturedOAuthURLUsesPendingState(t *testing.T) {
	testCapturedOAuthFlow(t, 1, true, false, false)
}

func TestCapturedOAuthConcurrentCallback(t *testing.T) {
	testCapturedOAuthFlow(t, 1, false, true, false)
}

func TestCapturedOAuthRetriesSetupFailure(t *testing.T) {
	testCapturedOAuthFlow(t, 1, false, false, true)
}

func testCapturedOAuthFlow(t *testing.T, componentCount int, checkPendingURL, duplicateCallback, retrySetup bool) {
	t.Helper()
	tokenStarted := make(chan struct{})
	releaseToken := make(chan struct{})
	var base string
	var operations, registrations, exchanges atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/metadata":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"resource":              base,
				"authorization_servers": []string{base},
			}))
		case "/.well-known/oauth-authorization-server":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                base,
				"response_types_supported":              []string{"code"},
				"authorization_endpoint":                base + "/authorize",
				"token_endpoint":                        base + "/token",
				"registration_endpoint":                 base + "/register",
				"token_endpoint_auth_methods_supported": []string{"none"},
			}))
		case "/register":
			attempt := registrations.Add(1)
			if retrySetup && attempt == 1 {
				http.Error(w, "temporary outage", http.StatusServiceUnavailable)

				return
			}

			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"client_id":                  "upstream-client",
				"token_endpoint_auth_method": "none",
			}))
		case "/token":
			count := exchanges.Add(1)
			if duplicateCallback {
				if count == 1 {
					close(tokenStarted)
					<-releaseToken
				} else {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))

					return
				}
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token": "new-upstream-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			}))
		default:
			operations.Add(1)
			http.Error(w, "MCP operations must not be probed", http.StatusInternalServerError)
		}
	}))
	defer provider.Close()
	defer func() {
		select {
		case <-releaseToken:
		default:
			close(releaseToken)
		}
	}()
	base = provider.URL
	vmcp := vmcpComponentVMCP(true)
	instance := vmcpComponentInstance("vmcpi1captured", vmcp.Name)
	vmcp.Spec.Manifest.Components = nil
	objects := []kclient.Object{vmcp, instance}

	for i := range componentCount {
		id := fmt.Sprintf("component%d", i)
		vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, types.VMCPComponent{
			ID:              id,
			ForceSingleUser: true,
		})
		server := vmcpComponentServer("ms1"+id, instance.Name, "")
		server.Spec.VMCPComponentID = id
		server.Spec.Manifest.Runtime = types.RuntimeRemote
		server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{URL: base + "/" + id}
		objects = append(objects, server)
	}

	syncedVMCPInstance(instance, vmcp)
	storage := &vmcpOAuthInitialEventsClient{WithWatch: vmcpConsentStorage(objects...)}
	gateway := vmcpConsentGateway(t)
	tokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "http://obot.example", 8080,
		mcp.Options{
			MCPRuntimeBackend: mcp.RuntimeBackendKubernetes,
			MCPNamespace:      "mcp",
		}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)

	factory := NewMCPOAuthHandlerFactory("http://obot.example", manager, storage, gateway, tokens, "", true)
	factory.resolveOAuthClient = func(context.Context, kclient.Client, string) (v1.OAuthClient, error) {
		return v1.OAuthClient{Spec: v1.OAuthClientSpec{Manifest: types.OAuthClientManifest{ClientName: "test-client"}}}, nil
	}
	h := &handler{
		baseURL:      "http://obot.example",
		oauthChecker: factory,
	}
	req := vmcpComponentRequest(storage, instance.Name, "")
	req.GatewayClient = gateway
	authRequest := &v1.OAuthAuthRequest{
		Name:      "oar1captured",
		Namespace: system.DefaultNamespace,
		Spec: v1.OAuthAuthRequestSpec{
			MCPID:                  instance.Name,
			ClientID:               "client",
			UserID:                 42,
			Resource:               "http://obot.example/mcp-connect/" + instance.Name,
			ConsentApproved:        true,
			ConsentPrepared:        true,
			ConsentMCPAuthRequired: true,
			RedirectURI:            "http://client.example/callback",
		},
	}
	require.NoError(t, storage.Create(t.Context(), authRequest))

	batch := &gatewaytypes.MCPOAuthChallenge{
		UserID:    "42",
		Namespace: system.DefaultNamespace,
		MCPID:     instance.Name,
		ClientID:  authRequest.Spec.ClientID,
		Resource:  authRequest.Spec.Resource,
	}
	_, _, aggregate, err := manager.ServerForActionWithConnectID(t.Context(), instance.Name, req.User)
	require.NoError(t, err)
	require.Len(t, aggregate.Components, componentCount)

	for i, target := range aggregate.Components {
		server, cfg, err := manager.ServerForAction(t.Context(), target.ConnectID(), req.User)
		require.NoError(t, err)

		batch.Requirements = append(batch.Requirements, v1.UpstreamOAuthRequirement{
			Component:        server.Name,
			ConnectID:        target.ConnectID(),
			URL:              cfg.URL,
			TargetHash:       mcp.OAuthTargetHash(server, cfg),
			StatusCode:       401 + i%2*2,
			ResourceMetadata: base + "/metadata",
			Scope:            fmt.Sprintf("write%d", i),
		})

		// A valid but underscoped credential must not suppress captured requirements.
		require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "42", target.ConnectID(), cfg.URL, "", &oauth2.Config{
			ClientID: "old-client",
			Scopes:   []string{"read"},
		}, &oauth2.Token{
			AccessToken: "old-token",
			Expiry:      time.Now().Add(time.Hour),
		}))
	}

	// A second ticket can ask the same provider for another scope; authorize their union once.
	extra := batch.Requirements[0]
	extra.Scope = "admin0"
	// More than eight captured headers from one component are valid; duplicates merge.
	for range 9 {
		batch.Requirements = append(batch.Requirements, extra)
	}
	require.NoError(t, gateway.CreateMCPOAuthChallenge(t.Context(), batch))

	authRequest.Spec.Scope = "ordinary " + gatewayclient.UpstreamOAuthScopePrefix + "zzzzzzzzzzzzzzzzzzzzzzzzzz " + gatewayclient.UpstreamOAuthScopePrefix + batch.ID
	require.NoError(t, req.Update(authRequest))

	// Fail the durable transfer once: the source must stay available for this transaction.
	fail := true
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
		if request, ok := obj.(*v1.OAuthAuthRequest); ok && len(request.Spec.UpstreamOAuth) > 0 && fail {
			fail = false

			return errors.New("durable write unavailable")
		}

		return c.Update(ctx, obj, opts...)
	}})
	failureReq := req
	failureReq.Storage = wrapped

	require.ErrorContains(t, h.handoffUpstreamChallenges(failureReq, authRequest, aggregate), "durable write unavailable")
	require.NoError(t, h.handoffUpstreamChallenges(req, authRequest, aggregate))
	require.Len(t, authRequest.Spec.UpstreamOAuth, componentCount)

	_, err = gateway.ClaimMCPOAuthChallenge(t.Context(), batch.ID, "42", system.DefaultNamespace, instance.Name, "client", batch.Resource, authRequest.Name)

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Source deletion and repeated handoff do not lose the transaction payload.
	require.NoError(t, h.handoffUpstreamChallenges(req, authRequest, aggregate))
	require.Len(t, authRequest.Spec.UpstreamOAuth, componentCount)
	require.ErrorContains(t, h.ensureMCPAuthComplete(req, *authRequest), "not complete")

	if retrySetup {
		_, err := factory.pendingCapturedAuth(req, authRequest.Name, "")
		require.Error(t, err)
		require.NoError(t, req.Get(authRequest, authRequest.Name))
		require.False(t, authRequest.Spec.UpstreamOAuth[0].Started)
		require.False(t, authRequest.Spec.UpstreamOAuth[0].Failed)
	}

	var originalState string
	if checkPendingURL {
		// Retrying an already-started handoff must compare permissions, not scope order.
		retryBatch := *batch
		retryBatch.ID = ""
		retryBatch.Requirements = []v1.UpstreamOAuthRequirement{batch.Requirements[0]}
		retryBatch.Requirements[0].Scope = "admin0 write0"
		require.NoError(t, gateway.CreateMCPOAuthChallenge(t.Context(), &retryBatch))

		authRequest.Spec.Scope = gatewayclient.UpstreamOAuthScopePrefix + retryBatch.ID
		authRequest.Spec.UpstreamOAuth[0].Scope = "write0 admin0 write0"
		authRequest.Spec.UpstreamOAuth[0].Started = true
		require.NoError(t, req.Update(authRequest))
		require.NoError(t, h.handoffUpstreamChallenges(req, authRequest, aggregate))
		require.True(t, authRequest.Spec.UpstreamOAuth[0].Started)
		require.Equal(t, "admin0 write0", authRequest.Spec.UpstreamOAuth[0].Scope)

		// A genuinely new permission must still be rejected once setup has started.
		retryBatch.ID = ""
		retryBatch.Requirements[0].Scope = "new-permission"
		require.NoError(t, gateway.CreateMCPOAuthChallenge(t.Context(), &retryBatch))
		authRequest.Spec.Scope = gatewayclient.UpstreamOAuthScopePrefix + retryBatch.ID
		require.ErrorContains(t, h.handoffUpstreamChallenges(req, authRequest, aggregate), "upstream scopes changed")

		authRequest.Spec.UpstreamOAuth[0].Started = false
		require.NoError(t, req.Update(authRequest))

		writes := 0
		urlStorage := interceptor.NewClient(storage, interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if _, ok := obj.(*v1.OAuthAuthRequest); ok {
				writes++
				if writes > 1 {
					return errors.New("unexpected authorization URL write")
				}
			}

			return c.Update(ctx, obj, opts...)
		}})
		urlReq := req
		urlReq.Storage = urlStorage
		first, err := factory.pendingCapturedAuth(urlReq, authRequest.Name, "")
		require.NoError(t, err)

		second, err := factory.pendingCapturedAuth(urlReq, authRequest.Name, "")
		require.NoError(t, err)
		require.Equal(t, first, second, "reconstruct the same URL from pending state")
		require.Equal(t, 1, writes, "only the launch claim writes the OAuth request")

		requirement := authRequest.Spec.UpstreamOAuth[0]
		ps, err := gateway.FindMCPOAuthPendingState(t.Context(), req.User.GetUID(), requirement.ConnectID, requirement.URL, authRequest.Name, upstreamRequirementKey(requirement))
		require.NoError(t, err)

		originalState = ps.State
	}

	pending, err := factory.pendingCapturedAuth(req, authRequest.Name, "")
	require.NoError(t, err)
	require.Len(t, pending, componentCount)

	again, err := factory.pendingCapturedAuth(req, authRequest.Name, "")
	require.NoError(t, err)
	require.Equal(t, pending, again)
	expectedRegistrations := int32(componentCount)
	if retrySetup {
		expectedRegistrations++
	}

	require.Equal(t, expectedRegistrations, registrations.Load(), "polls must reuse pending attempts")
	require.Zero(t, operations.Load())

	for i, item := range pending {
		parsed, err := url.Parse(item.AuthURL)
		require.NoError(t, err)

		wantScope := fmt.Sprintf("write%d", i)
		if i == 0 {
			wantScope = "admin0 write0"
		}

		require.Equal(t, wantScope, parsed.Query().Get("scope"))

		state := parsed.Query().Get("state")
		if originalState != "" {
			require.Equal(t, originalState, state, "URL reconstruction must reuse the pending state")
		}

		if i == 0 {
			ps, err := gateway.GetMCPOAuthPendingState(t.Context(), state)
			require.NoError(t, err)

			stranger := req
			stranger.User = &kuser.DefaultInfo{
				UID:    "43",
				Groups: []string{types.GroupAuthenticated},
			}

			require.Error(t, factory.validateCapturedCallback(stranger, ps))

			expired := *ps
			expired.CreatedAt = time.Now().Add(-time.Hour)

			require.ErrorContains(t, factory.validateCapturedCallback(req, &expired), "expired")
		}

		callbackReq := req
		callbackReq.Request = httptest.NewRequest(http.MethodGet, "http://obot.example/oauth/mcp/callback?state="+url.QueryEscape(state)+"&code=code", nil)
		callbackReq.ResponseWriter = httptest.NewRecorder()
		if duplicateCallback {
			finished := make(chan error, 1)
			go func() { finished <- h.oauthCallback(callbackReq) }()

			select {
			case <-tokenStarted:
			case <-time.After(10 * time.Second):
				t.Fatal("first callback did not reach token exchange")
			}

			duplicate := callbackReq
			duplicate.ResponseWriter = httptest.NewRecorder()

			require.Error(t, h.oauthCallback(duplicate))

			close(releaseToken)
			require.NoError(t, <-finished, "duplicate callback must not poison the successful exchange")
		} else {
			// Simulate a completed exchange whose separate transaction-progress write fails.
			if checkPendingURL {
				callbackReq.Storage = interceptor.NewClient(storage, interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if request, ok := obj.(*v1.OAuthAuthRequest); ok && request.Spec.UpstreamOAuth[0].Completed {
						return errors.New("completion write unavailable")
					}

					return c.Update(ctx, obj, opts...)
				}})
				require.ErrorContains(t, h.oauthCallback(callbackReq), "completion write unavailable")
				require.NoError(t, req.Get(authRequest, authRequest.Name))
				require.False(t, authRequest.Spec.UpstreamOAuth[0].Completed)

				remaining, err := factory.pendingCapturedAuth(req, authRequest.Name, "")
				require.NoError(t, err)
				require.Empty(t, remaining)

				for _, phase := range []string{"consent", "completion"} {
					t.Run("recover during "+phase, func(t *testing.T) {
						require.NoError(t, req.Get(authRequest, authRequest.Name))
						authRequest.Spec.UpstreamOAuth[0].Completed = false
						authRequest.Spec.Scope = "ordinary"
						require.NoError(t, req.Update(authRequest))

						if phase == "consent" {
							require.NoError(t, h.prepareOAuthConsent(req, authRequest))
						} else {
							completionReq := req
							completionReq.Request = req.Clone(req.Context())
							completionReq.SetPathValue("oauth_auth_request", authRequest.Name)
							recorder := httptest.NewRecorder()
							completionReq.ResponseWriter = recorder
							require.NoError(t, h.oauthComplete(completionReq))
							require.Equal(t, http.StatusFound, recorder.Code)
							require.Contains(t, recorder.Header().Get("Location"), "code=")
						}

						require.NoError(t, req.Get(authRequest, authRequest.Name))
						require.True(t, authRequest.Spec.UpstreamOAuth[0].Completed)
					})
				}
			} else {
				require.NoError(t, h.oauthCallback(callbackReq))
			}
		}

		require.NoError(t, req.Get(authRequest, authRequest.Name))

		if i+1 < componentCount {
			require.Error(t, h.ensureMCPAuthComplete(req, *authRequest))
		}
	}

	require.NoError(t, h.ensureMCPAuthComplete(req, *authRequest))
	require.Equal(t, int32(componentCount), exchanges.Load())
	require.Zero(t, operations.Load(), "OAuth must never replay MCP calls")

	// A changed target remains invalid even after the last callback completed.
	var changed v1.MCPServer
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      batch.Requirements[0].Component,
	}, &changed))

	changed.Spec.Manifest.RemoteConfig.URL = base + "/changed"
	require.NoError(t, storage.Update(t.Context(), &changed))
	require.Error(t, h.ensureMCPAuthComplete(req, *authRequest))
}

func TestCapturedOAuthRecoversSavedTokenAfterNotificationFailure(t *testing.T) {
	gateway := vmcpConsentGateway(t, func(context.Context, string) error {
		return errors.New("controller notification failed")
	})
	requirement := v1.UpstreamOAuthRequirement{
		Component:  "ms1component",
		ConnectID:  "ms1component",
		URL:        "https://provider.test/mcp",
		TargetHash: "target",
		Started:    true,
	}
	request := &v1.OAuthAuthRequest{
		Name:      "oar1notification",
		Namespace: system.DefaultNamespace,
		Spec: v1.OAuthAuthRequestSpec{
			UserID:          42,
			ConsentApproved: true,
			UpstreamOAuth:   []v1.UpstreamOAuthRequirement{requirement},
		},
	}
	req := vmcpComponentRequest(vmcpConsentStorage(request), "vmcpi1test", requirement.Component)
	req.GatewayClient = gateway
	key := upstreamRequirementKey(requirement)
	err := gateway.ReplaceMCPOAuthToken(t.Context(), "42", requirement.ConnectID, requirement.URL, request.Name, &oauth2.Config{}, &oauth2.Token{AccessToken: "saved"}, key)
	require.ErrorContains(t, err, "controller notification failed")

	factory := &MCPOAuthHandlerFactory{}
	pending := &gatewaytypes.MCPOAuthPendingState{OAuthAuthRequestID: request.Name, UpstreamChallenge: key}
	require.NoError(t, factory.completeCapturedCallback(req, pending, err))
	require.NoError(t, req.Get(request, request.Name))
	require.True(t, request.Spec.UpstreamOAuth[0].Failed)

	completed, err := reconcileCapturedRequirement(req, request.Name, request.Spec.UpstreamOAuth[0])
	require.NoError(t, err)
	require.True(t, completed)
	require.NoError(t, req.Get(request, request.Name))
	require.True(t, request.Spec.UpstreamOAuth[0].Completed)
	require.False(t, request.Spec.UpstreamOAuth[0].Failed)
}

func TestTokenConfigurationDoesNotCrossMCPResources(t *testing.T) {
	gateway := vmcpConsentGateway(t)
	conf := &oauth2.Config{
		Endpoint: oauth2.Endpoint{AuthURL: "https://provider.test/authorize", TokenURL: "https://provider.test/token"},
		Scopes:   []string{"old-resource-scope"},
	}
	require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "42", "ms1remote", "https://provider.test/old", "", conf, &oauth2.Token{AccessToken: "old"}))

	store := mcp.NewGlobalTokenStore(gateway)
	old, _, err := store.ForUserAndMCP("42", "ms1remote", "https://provider.test/old").GetTokenConfig(t.Context())
	require.NoError(t, err)
	require.Equal(t, conf.Scopes, old.Scopes)

	changed, token, err := store.ForUserAndMCP("42", "ms1remote", "https://provider.test/new").GetTokenConfig(t.Context())
	require.NoError(t, err)
	require.Nil(t, changed)
	require.Nil(t, token)
}

func TestCapturedOAuthDenialStaysIncompleteWithoutStartingAnotherAttempt(t *testing.T) {
	requirement := v1.UpstreamOAuthRequirement{
		Component:  "ms1component",
		ConnectID:  "ms1component",
		TargetHash: "target",
		StatusCode: http.StatusUnauthorized,
		Started:    true,
	}
	request := &v1.OAuthAuthRequest{
		Name:      "oar1denied",
		Namespace: system.DefaultNamespace,
		Spec: v1.OAuthAuthRequestSpec{
			UserID:          42,
			ConsentApproved: true,
			UpstreamOAuth:   []v1.UpstreamOAuthRequirement{requirement},
		},
	}
	storage := vmcpConsentStorage(request)
	req := vmcpComponentRequest(storage, "vmcpi1test", requirement.Component)
	factory := &MCPOAuthHandlerFactory{}
	pending := &gatewaytypes.MCPOAuthPendingState{
		OAuthAuthRequestID: request.Name,
		UpstreamChallenge:  upstreamRequirementKey(requirement),
	}
	require.NoError(t, factory.completeCapturedCallback(req, pending, errors.New("access_denied")))
	require.NoError(t, req.Get(request, request.Name))
	require.True(t, request.Spec.UpstreamOAuth[0].Failed)
	require.False(t, request.Spec.UpstreamOAuth[0].Completed)

	req.GatewayClient = vmcpConsentGateway(t)
	completed, err := reconcileCapturedRequirement(req, request.Name, request.Spec.UpstreamOAuth[0])
	require.NoError(t, err)
	require.False(t, completed, "a denial without a matching saved token must remain incomplete")

	// No manager is configured: a poll must stop before any new attempt.
	_, err = factory.capturedAuthURL(req, request.Name, request.Spec.UpstreamOAuth[0], v1.MCPServer{}, mcp.ServerConfig{})

	require.ErrorContains(t, err, "upstream authentication failed")

	request.Spec.UpstreamOAuth[0].Completed = true
	require.NoError(t, req.Update(request))
	require.NoError(t, factory.completeCapturedCallback(req, pending, errors.New("duplicate denial")))
	require.NoError(t, req.Get(request, request.Name))
	require.True(t, request.Spec.UpstreamOAuth[0].Completed)
}

func TestCapturedOAuthCanceledSetupClearsClaim(t *testing.T) {
	request := &v1.OAuthAuthRequest{
		Name:      "oar1canceled",
		Namespace: system.DefaultNamespace,
		Spec: v1.OAuthAuthRequestSpec{
			UserID: 42,
			UpstreamOAuth: []v1.UpstreamOAuthRequirement{{
				Component: "ms1component",
				ConnectID: "ms1component",
			}},
		},
	}
	storage := vmcpConsentStorage(request)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Cancel immediately after persisting the claim. The fake storage must enforce
	// context cancellation so rollback cannot silently succeed on a canceled request.
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			return c.Get(ctx, key, obj, opts...)
		},
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			if err := c.Update(ctx, obj, opts...); err != nil {
				return err
			}

			if obj.(*v1.OAuthAuthRequest).Spec.UpstreamOAuth[0].Started {
				cancel()
			} else {
				_, bounded := ctx.Deadline()
				require.True(t, bounded, "cleanup must have a deadline")
			}

			return nil
		},
	})
	req := vmcpComponentRequest(wrapped, "vmcpi1test", "ms1component")
	req.Request = req.WithContext(ctx)
	req.GatewayClient = vmcpConsentGateway(t)
	factory := &MCPOAuthHandlerFactory{}

	_, err := factory.capturedAuthURL(req, request.Name, request.Spec.UpstreamOAuth[0], v1.MCPServer{}, mcp.ServerConfig{})
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(request), request))
	require.False(t, request.Spec.UpstreamOAuth[0].Started)
	require.Zero(t, request.Spec.UpstreamOAuth[0].StartedAt)
	require.False(t, request.Spec.UpstreamOAuth[0].Failed)
}

func TestCapturedOAuthMissingStateHasBoundedPreparationWindow(t *testing.T) {
	gateway := vmcpConsentGateway(t)
	req := vmcpComponentRequest(vmcpConsentStorage(), "vmcpi1test", "ms1test")
	req.GatewayClient = gateway
	factory := &MCPOAuthHandlerFactory{}
	requirement := v1.UpstreamOAuthRequirement{
		Component: "ms1test",
		ConnectID: "ms1test",
		Started:   true,
		StartedAt: time.Now().Unix(),
	}

	_, err := factory.pendingCapturedAuthURL(req, "oar1test", requirement, mcp.ServerConfig{})
	require.ErrorContains(t, err, "retry shortly")

	requirement.StartedAt = time.Now().Add(-3 * time.Minute).Unix()
	_, err = factory.pendingCapturedAuthURL(req, "oar1test", requirement, mcp.ServerConfig{})
	require.ErrorContains(t, err, "start a new authorization")
}

func TestCapturedLocalLoginValidationFailureReachesCLI(t *testing.T) {
	gateway := vmcpConsentGateway(t)
	state := "ui-local-captured"
	conf := &oauth2.Config{
		ClientID:    "client",
		RedirectURL: "http://localhost:12345/provider/callback",
	}
	require.NoError(t, gateway.CreateMCPOAuthPendingState(t.Context(), "other-user", "ms1test", "https://provider.test/mcp", "", "oar1test", state, "verifier", "", conf, "captured"))

	req := vmcpComponentRequest(vmcpConsentStorage(), "vmcpi1test", "ms1test")
	req.Request = httptest.NewRequest(http.MethodGet, "http://obot.example/oauth/mcp/callback?state="+state, nil)
	recorder := httptest.NewRecorder()
	req.ResponseWriter = recorder
	h := &handler{oauthChecker: &MCPOAuthHandlerFactory{stateMgr: newStateManager(gateway, nil)}}

	require.NoError(t, h.oauthCallback(req))
	require.Equal(t, http.StatusFound, recorder.Code)
	location, err := url.Parse(recorder.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "localhost:12345", location.Host)
	require.Equal(t, "authentication_failed", location.Query().Get("error"))

	// A failed validation must not consume another user's pending attempt.
	_, err = gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
}
