package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// retryResponse is the response of a controller handler, which records the delay it asks to be run again after.
type retryResponse struct {
	delay time.Duration
}

// wholeStatusClient saves a status update as an update of the whole object, for test storage without a status
// subresource.
type wholeStatusClient struct {
	kclient.WithWatch
}

type wholeStatusWriter struct {
	kclient.SubResourceWriter
	client kclient.Client
}

func (*retryResponse) Attributes() map[string]any {
	return map[string]any{}
}

func (r *retryResponse) RetryAfter(delay time.Duration) {
	if r.delay == 0 || delay < r.delay {
		r.delay = delay
	}
}

func (c wholeStatusClient) Status() kclient.SubResourceWriter {
	return wholeStatusWriter{SubResourceWriter: c.WithWatch.Status(), client: c.WithWatch}
}

func (w wholeStatusWriter) Update(ctx context.Context, obj kclient.Object, _ ...kclient.SubResourceUpdateOption) error {
	return w.client.Update(ctx, obj)
}

// probeManager is the session manager of a probe, which may reach the test servers on the loopback.
func probeManager() *SessionManager {
	return &SessionManager{remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true}}
}

// TestProbeOAuthRequirement checks what the background probe of the vMCP controller finds, from unauthenticated
// requests that always end their session: a server that challenges initialize, as one with dynamic client registration
// does, one that challenges only tools/list, and one that only publishes its protected resource metadata ask for OAuth;
// one that does none of these does not; and a server whose answers say nothing either way is an error, which the
// controller records as unknown.
func TestProbeOAuthRequirement(t *testing.T) {
	initialized := []string{"initialize", "notifications/initialized", "tools/list", http.MethodDelete}
	for _, tt := range []struct {
		name      string
		challenge string
		wellKnown bool
		toolsList http.HandlerFunc
		want      bool
		// wantErr is part of the error of a probe that cannot tell.
		wantErr     string
		wantMethods []string
	}{
		{
			name:        "initialize challenge",
			challenge:   "initialize",
			want:        true,
			wantMethods: []string{"initialize"},
		},
		{
			name:        "tools/list challenge",
			challenge:   "tools/list",
			want:        true,
			wantMethods: initialized,
		},
		{
			name:        "only published metadata",
			wellKnown:   true,
			want:        true,
			wantMethods: initialized,
		},
		{
			name:        "no OAuth",
			wantMethods: initialized,
		},
		{
			name: "tools/list failure",
			toolsList: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			},
			wantErr:     "tools/list was answered with status 503",
			wantMethods: initialized,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := newLateChallengeUpstream(t, tt.challenge, tt.wellKnown, tt.toolsList)
			required, err := probeManager().ProbeOAuthRequirement(t.Context(), upstream.mcpURL(), "")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, required)
			}
			requests := upstream.recorded()
			methods := make([]string, 0, len(requests))
			for _, request := range requests {
				methods = append(methods, request.method)
			}
			require.Equal(t, tt.wantMethods, methods)
			requireProbesUnauthenticated(t, requests)
		})
	}
}

// TestProbeOAuthRequirementIsUnknownWithoutEvidence checks that a server whose answer is not an OAuth challenge is not
// taken to ask for OAuth: a 401 from which no authorization could start, as a server that wants an API key may send,
// and an initialize that is refused outright, are both errors, which the controller records as unknown.
func TestProbeOAuthRequirementIsUnknownWithoutEvidence(t *testing.T) {
	for _, tt := range []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name: "a 401 without OAuth metadata",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
				http.Error(w, "an API key is required", http.StatusUnauthorized)
			},
			wantErr: "publishes no OAuth authorization server metadata",
		},
		{
			name:    "initialize refused",
			handler: http.NotFound,
			wantErr: "initialize was answered with status 404",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			t.Cleanup(server.Close)
			required, err := probeManager().ProbeOAuthRequirement(t.Context(), server.URL+"/mcp", "")
			require.ErrorContains(t, err, tt.wantErr)
			require.False(t, required)
		})
	}
}

// TestProbeOAuthRequirementIsBounded checks that a server that never answers tools/list fails the probe once its time
// runs out, and that the session the probe started is still ended.
func TestProbeOAuthRequirementIsBounded(t *testing.T) {
	upstream := newLateChallengeUpstream(t, "", true, func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	started := time.Now()
	_, err := probeManager().ProbeOAuthRequirement(ctx, upstream.mcpURL(), "")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 10*time.Second)
	// The DELETE is sent once the hung tools/list ran out of time.
	require.Eventually(t, func() bool {
		requests := upstream.recorded()
		return len(requests) == 4 && requests[3].status != 0
	}, 5*time.Second, 10*time.Millisecond)
	var methods []string
	for _, request := range upstream.recorded() {
		methods = append(methods, request.method)
		if request.method == http.MethodDelete {
			require.Equal(t, http.StatusNoContent, request.status, "the probe's session was not ended")
		}
	}
	require.ElementsMatch(t, []string{"initialize", "notifications/initialized", "tools/list", http.MethodDelete}, methods)
}

// TestAgentReadinessReportAgreesWithConnectForProbedOAuth is TestAgentReadinessReportAgreesWithConnect for a component
// without static OAuth that uses dynamic client registration and challenges initialize, from the vMCP controller's
// probe of it to the agent's connection. Before the probe, nothing shows that it needs OAuth, so every agent is ready.
// Once the controller has probed it, readiness and connect both refuse every agent granted the component while the
// vMCP holds no grant for it, with oauth_required, and the vMCP's agent setup says the component requires OAuth; one
// grant, authorized on the vMCP, makes them all ready. An agent whose profiles do not grant the component is ready
// throughout. The status of the vMCP holds no URL. Users connect as before throughout.
func TestAgentReadinessReportAgreesWithConnectForProbedOAuth(t *testing.T) {
	upstream := newLateChallengeUpstream(t, "initialize", false, nil)
	vmcp, one, two := grantVMCP("vmcp1agreeprobe", false)
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.FixedURL = upstream.mcpURL()
	one.Spec.Manifest.RemoteConfig.URL = upstream.mcpURL()
	storageClient := newVMCPTestStorage(vmcp, one, two)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	gateway := newGrantTestGateway(t)
	manager, grants := newGrantTestManager(storageClient, gateway)
	stored := func() *v1.VMCP {
		t.Helper()
		current := &v1.VMCP{}
		require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		return current
	}
	requireUser := func() {
		t.Helper()
		cfg := connectVMCP(t, manager, vmcp.Name, principaltest.AsUser(&kuser.DefaultInfo{UID: "7"}))
		require.Nil(t, cfg.Components[0].OAuthGrant)
	}

	requireReadinessAgreesWithConnect(t, manager, stored(), "not probed yet", map[uint]bool{7: true, 8: true, 9: true})
	requireUser()

	handler := vmcphandler.New(gateway, nil, nil)
	handler.SetComponentOAuthProber(manager)
	response := &retryResponse{}
	require.NoError(t, handler.ProbeComponentOAuth(router.Request{Ctx: t.Context(), Client: wholeStatusClient{storageClient}, Object: stored()}, response))
	probed := stored()
	record := probed.Status.ComponentOAuth["one"]
	require.Equal(t, v1.VMCPComponentOAuthRequired, record.Required)
	encoded, err := json.Marshal(probed.Status)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(encoded), upstream.server.URL), "the status holds the URL probed")
	// The next probe is the daily recheck.
	require.InDelta(t, (24 * time.Hour).Seconds(), response.delay.Seconds(), 5)
	requireProbesUnauthenticated(t, upstream.recorded())

	reasons := requireReadinessAgreesWithConnect(t, manager, probed, "probed: asks for OAuth", map[uint]bool{7: false, 8: false, 9: true})
	required := []vmcpaccess.ReadinessReason{{ComponentID: "one", Code: types.AgentReadinessOAuthRequired}}
	require.Equal(t, required, reasons["agent:7"])
	require.Equal(t, required, reasons["agent:8"])
	setup, err := vmcpaccess.AgentSetup(t.Context(), grants, *probed)
	require.NoError(t, err)
	require.Equal(t, "one", setup.Components[0].ComponentID)
	require.True(t, setup.Components[0].OAuth.Required)
	require.Equal(t, required, setup.Components[0].Reasons)
	requireUser()

	storeGrant(t, gateway, vmcp.Name, upstream.mcpURL(), "grant-one")
	requireReadinessAgreesWithConnect(t, manager, probed, "the vMCP authorized", map[uint]bool{7: true, 8: true, 9: true})
	requireUser()
}
