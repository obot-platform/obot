package vmcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	probeFixedURL = "https://fixed.example.com/mcp"
	// probeConfiguredURL is the URL the vMCP fixes for the hostname-constrained component.
	probeConfiguredURL = "https://tenant.example.com/seven?token=secret"
	probeSeven         = "vp1seven"
	probeEight         = "vp1eight"
	probeUsers         = "vp1users"
)

// fakeProber answers probes from a result per URL, and resolves the URL that the vMCP fixes for a component whose URL
// comes from configuration.
type fakeProber struct {
	lock sync.Mutex
	// results are what a probe of each URL finds; a URL without one fails.
	results map[string]bool
	// failing are URLs whose probe fails.
	failing map[string]bool
	// urls are the URLs the vMCP fixes, by component ID.
	urls   map[string]string
	probed []string
}

type probeResponse struct {
	delay time.Duration
}

// probeHarness runs ProbeComponentOAuth on a vMCP in storage at a time the test sets.
type probeHarness struct {
	t       *testing.T
	client  kclient.WithWatch
	key     kclient.ObjectKey
	prober  *fakeProber
	handler *Handler
	now     time.Time
}

func (f *fakeProber) ProbeOAuthRequirement(_ context.Context, url, _ string) (bool, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.probed = append(f.probed, url)
	required, ok := f.results[url]
	if !ok || f.failing[url] {
		return false, errors.New("Post \"" + url + "\": timed out")
	}
	return required, nil
}

func (f *fakeProber) ComponentTarget(_ context.Context, _ v1.VMCP, component types.VMCPComponent) (vmcpconfig.ComponentTarget, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if url := f.urls[component.ID]; url != "" {
		return vmcpconfig.ComponentTarget{URL: url}, nil
	}
	return vmcpconfig.ComponentTarget{Missing: []string{"__url"}}, nil
}

// takeProbed returns the URLs probed since it was last called.
func (f *fakeProber) takeProbed() []string {
	f.lock.Lock()
	defer f.lock.Unlock()
	probed := f.probed
	f.probed = nil
	slices.Sort(probed)
	return probed
}

func (*probeResponse) Attributes() map[string]any {
	return map[string]any{}
}

func (r *probeResponse) RetryAfter(delay time.Duration) {
	if r.delay == 0 || delay < r.delay {
		r.delay = delay
	}
}

func probeRemote(id, fixedURL, hostname string) types.VMCPComponent {
	return types.VMCPComponent{ID: id, Name: id, MCPServerCatalogEntryID: id + "-entry", CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{FixedURL: fixedURL, Hostname: hostname},
	}}}
}

// probeTestVMCP returns a shared vMCP with a remote component whose definition fixes its URL, one whose URL the vMCP
// fixes, and components that are not probed: a remote whose URL nothing fixes, a remote that takes a header, one with
// static OAuth, a server that is not remote, and a remote that no agent-capable profile grants. Agents 7 and 8 have a
// profile each, and every user a third, which names no agent and alone grants the last remote.
func probeTestVMCP() *v1.VMCP {
	header := probeRemote("header", "https://header.example.com/mcp", "")
	header.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-API-Key", Usage: types.Header}}
	static := probeRemote("static", "https://static.example.com/mcp", "")
	static.CatalogEntry.Manifest.RemoteConfig.StaticOAuthRequired = true
	agentComponents := map[string]types.VMCPComponentSet{"fixed": {}, "configured": {}, "unfixed": {}, "header": {}, "static": {}, "local": {}}
	return &v1.VMCP{Name: "vmcp1probe", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{
				probeRemote("fixed", probeFixedURL, ""),
				probeRemote("configured", "", "tenant.example.com"),
				probeRemote("unfixed", "", "tenant.example.com"),
				header,
				static,
				{ID: "local", Name: "local"},
				probeRemote("users-only", "https://users.example.com/mcp", ""),
			},
			Profiles: []types.VMCPProfile{
				{ID: probeSeven, Name: "seven", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: agentComponents}},
				{ID: probeEight, Name: "eight", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: agentComponents}},
				{ID: probeUsers, Name: "users", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			},
		},
		ComponentStaticConfigurationHashes: map[string]string{"configured": "fixed-url"},
	}}
}

func newProbeHarness(t *testing.T, vmcp *v1.VMCP) *probeHarness {
	t.Helper()
	h := &probeHarness{
		t:      t,
		client: fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1.VMCP{}).WithObjects(vmcp).Build(),
		key:    kclient.ObjectKeyFromObject(vmcp),
		prober: &fakeProber{
			results: map[string]bool{probeFixedURL: true},
			failing: map[string]bool{},
			urls:    map[string]string{"configured": probeConfiguredURL},
		},
		now: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
	}
	h.handler = &Handler{now: func() time.Time { return h.now }}
	h.handler.SetComponentOAuthProber(h.prober)
	return h
}

func (h *probeHarness) stored() *v1.VMCP {
	h.t.Helper()
	current := &v1.VMCP{}
	require.NoError(h.t, h.client.Get(h.t.Context(), h.key, current))
	return current
}

// run runs the handler and returns the URLs it probed, the delay it asked to be run again after, and the vMCP stored
// after it.
func (h *probeHarness) run() ([]string, time.Duration, *v1.VMCP) {
	h.t.Helper()
	response := &probeResponse{}
	require.NoError(h.t, h.handler.ProbeComponentOAuth(router.Request{Ctx: h.t.Context(), Client: h.client, Object: h.stored()}, response))
	return h.prober.takeProbed(), response.delay, h.stored()
}

func (h *probeHarness) update(change func(*v1.VMCP)) {
	h.t.Helper()
	current := h.stored()
	change(current)
	require.NoError(h.t, h.client.Update(h.t.Context(), current))
}

// targetHash returns the target hash of vmcp's component with componentID.
func targetHash(t *testing.T, vmcp *v1.VMCP, componentID string) string {
	t.Helper()
	components := vmcpconfig.ComponentsForInstance(*vmcp, v1.VMCPInstance{})
	index := slices.IndexFunc(components, func(component types.VMCPComponent) bool { return component.ID == componentID })
	require.GreaterOrEqual(t, index, 0)
	return vmcpconfig.ComponentOAuthTargetHash(*vmcp, components[index])
}

// TestProbeComponentOAuthProbesWhatAgentsReach checks which components are probed: each remote component that an
// agent-capable profile grants, at the URL its definition or the vMCP fixes, and none whose URL nothing fixes, that
// takes headers, requires static OAuth, is not remote, or that only users are granted. Each result is recorded once
// per component, for its target, and the status never holds a URL.
func TestProbeComponentOAuthProbesWhatAgentsReach(t *testing.T) {
	h := newProbeHarness(t, probeTestVMCP())
	probed, delay, stored := h.run()
	require.Equal(t, []string{probeFixedURL, probeConfiguredURL}, probed)
	require.Equal(t, []string{"configured", "fixed"}, slices.Sorted(func(yield func(string) bool) {
		for id := range stored.Status.ComponentOAuth {
			if !yield(id) {
				return
			}
		}
	}))
	for componentID, record := range stored.Status.ComponentOAuth {
		require.Equal(t, targetHash(t, stored, componentID), record.TargetHash)
	}
	require.Equal(t, v1.VMCPComponentOAuthRequired, stored.Status.ComponentOAuth["fixed"].Required)
	failed := stored.Status.ComponentOAuth["configured"]
	require.Equal(t, v1.VMCPComponentOAuthUnknown, failed.Required)
	require.Equal(t, 1, failed.Failures)
	require.Equal(t, h.now, failed.CheckedAt.UTC())
	// The failed probe is retried first.
	require.Equal(t, componentOAuthRetryBase, delay)

	encoded, err := json.Marshal(stored.Status)
	require.NoError(t, err)
	for _, url := range []string{probeFixedURL, probeConfiguredURL, "tenant.example.com", "secret"} {
		require.NotContains(t, string(encoded), url)
	}

	// Nothing is due, so nothing is probed or saved again.
	resourceVersion := stored.ResourceVersion
	h.now = h.now.Add(30 * time.Second)
	probed, delay, stored = h.run()
	require.Empty(t, probed)
	require.Equal(t, 30*time.Second, delay)
	require.Equal(t, resourceVersion, stored.ResourceVersion)
}

// TestProbeComponentOAuthSchedule checks when components are probed again: a failed probe after a wait that doubles
// with each failure in a row, up to componentOAuthRetryMax, and a successful one componentOAuthRecheckInterval later.
// A failure after a success keeps what the success found; a first failure is unknown.
func TestProbeComponentOAuthSchedule(t *testing.T) {
	h := newProbeHarness(t, probeTestVMCP())
	start := h.now
	h.run()

	// The failing component is probed again after 1, 2, 4 and 8 minutes, each wait doubling.
	waits := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}
	for i, wait := range waits[:len(waits)-1] {
		h.now = h.now.Add(wait)
		probed, delay, stored := h.run()
		require.Equal(t, []string{probeConfiguredURL}, probed, "retry %d", i+1)
		require.Equal(t, waits[i+1], delay, "retry %d", i+1)
		record := stored.Status.ComponentOAuth["configured"]
		require.Equal(t, i+2, record.Failures)
		require.Equal(t, v1.VMCPComponentOAuthUnknown, record.Required)
	}
	require.Equal(t, componentOAuthRetryMax, nextComponentOAuthProbe(v1.VMCPComponentOAuth{Failures: 30}).Sub(time.Time{}))

	// It succeeds: the next probe of every component is the daily recheck.
	h.prober.results[probeConfiguredURL] = true
	h.now = h.now.Add(8 * time.Minute)
	probed, delay, stored := h.run()
	require.Equal(t, []string{probeConfiguredURL}, probed)
	record := stored.Status.ComponentOAuth["configured"]
	require.Equal(t, v1.VMCPComponentOAuthRequired, record.Required)
	require.Zero(t, record.Failures)
	require.Equal(t, start.Add(componentOAuthRecheckInterval).Sub(h.now), delay)

	// The daily recheck probes every component again. A failure keeps what the last success found.
	h.prober.failing[probeConfiguredURL] = true
	h.now = h.now.Add(componentOAuthRecheckInterval)
	probed, delay, stored = h.run()
	require.Equal(t, []string{probeFixedURL, probeConfiguredURL}, probed)
	record = stored.Status.ComponentOAuth["configured"]
	require.Equal(t, v1.VMCPComponentOAuthRequired, record.Required)
	require.Equal(t, 1, record.Failures)
	require.Equal(t, componentOAuthRetryBase, delay)
	require.Equal(t, v1.VMCPComponentOAuthRequired, stored.Status.ComponentOAuth["fixed"].Required)
}

// TestProbeComponentOAuthReprobesChanges checks that a change to what agents reach is probed at once: a component
// whose definition changes, and one whose URL the vMCP now fixes to another value; and that results are dropped for
// components no agent-capable profile grants any more, and for a personal vMCP.
func TestProbeComponentOAuthReprobesChanges(t *testing.T) {
	h := newProbeHarness(t, probeTestVMCP())
	h.run()

	// The vMCP now fixes another URL for the component.
	const changedURL = "https://tenant.example.com/changed"
	h.prober.urls["configured"] = changedURL
	h.prober.results[changedURL] = true
	h.update(func(vmcp *v1.VMCP) {
		vmcp.Spec.ComponentStaticConfigurationHashes["configured"] = "changed-url"
	})
	probed, _, stored := h.run()
	require.Equal(t, []string{changedURL}, probed)
	require.Equal(t, v1.VMCPComponentOAuthRequired, stored.Status.ComponentOAuth["configured"].Required)
	require.Zero(t, stored.Status.ComponentOAuth["configured"].Failures, "the result for the URL no longer fixed is dropped")
	require.Equal(t, targetHash(t, stored, "configured"), stored.Status.ComponentOAuth["configured"].TargetHash)

	// A new definition is probed anew, even at the same URL.
	h.update(func(vmcp *v1.VMCP) {
		vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.TunnelName = "tunnel"
	})
	probed, _, stored = h.run()
	require.Equal(t, []string{probeFixedURL}, probed)
	require.Equal(t, targetHash(t, stored, "fixed"), stored.Status.ComponentOAuth["fixed"].TargetHash)

	// A component no agent-capable profile grants is not probed, and its result is dropped.
	h.update(func(vmcp *v1.VMCP) {
		for i := range vmcp.Spec.Manifest.Profiles[:2] {
			delete(vmcp.Spec.Manifest.Profiles[i].Permissions.AllowedComponents, "fixed")
		}
	})
	probed, _, stored = h.run()
	require.Empty(t, probed)
	require.NotContains(t, stored.Status.ComponentOAuth, "fixed")
	require.Contains(t, stored.Status.ComponentOAuth, "configured")

	// Without agent-capable profiles, nothing is probed or recorded.
	h.update(func(vmcp *v1.VMCP) {
		vmcp.Spec.Manifest.Profiles = vmcp.Spec.Manifest.Profiles[2:]
	})
	probed, delay, stored := h.run()
	require.Empty(t, probed)
	require.Zero(t, delay)
	require.Nil(t, stored.Status.ComponentOAuth)

	// Nor is a personal vMCP probed, whatever its profiles: agents only connect to shared vMCPs.
	personal := probeTestVMCP()
	personal.Spec.UserID = "1"
	h = newProbeHarness(t, personal)
	probed, _, stored = h.run()
	require.Empty(t, probed)
	require.Nil(t, stored.Status.ComponentOAuth)
}
