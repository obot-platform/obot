package vmcp

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// componentOAuthRecheckInterval is how long a probe that succeeded stands before the URL is probed again, to notice
	// a server that starts or stops asking for OAuth.
	componentOAuthRecheckInterval = 24 * time.Hour
	// componentOAuthRetryBase and componentOAuthRetryMax bound the wait before a URL whose probe failed is probed again.
	// The wait doubles with each failure in a row.
	componentOAuthRetryBase = time.Minute
	componentOAuthRetryMax  = time.Hour
	// componentOAuthProbeTimeout bounds one probe, all its requests included. Each request is also bounded by the
	// OAuth client's own timeout.
	componentOAuthProbeTimeout = 30 * time.Second
	// componentOAuthProbeConcurrency is how many URLs of one vMCP are probed at once.
	componentOAuthProbeConcurrency = 4
)

// ComponentOAuthProber probes remote MCP servers for whether they ask for OAuth, and resolves the URL that an agent's
// connection to a component reaches. The MCP session manager implements it.
type ComponentOAuthProber interface {
	vmcpconfig.ComponentTargetResolver
	ProbeOAuthRequirement(ctx context.Context, url, tunnelName string) (bool, error)
}

// componentOAuthTarget is one URL of a component to probe.
type componentOAuthTarget struct {
	url, digest string
}

// componentOAuthTargets are the URLs of one component to probe, for its definition.
type componentOAuthTargets struct {
	definitionHash string
	tunnelName     string
	targets        []componentOAuthTarget
}

// componentOAuthProbeJob is one URL due for a probe, and where its result goes.
type componentOAuthProbeJob struct {
	componentID string
	index       int
	url, tunnel string
	required    bool
	err         error
}

// SetComponentOAuthProber sets the prober of remote components for OAuth (ProbeComponentOAuth). Without one, nothing
// is probed. Set it before the handler runs.
func (h *Handler) SetComponentOAuthProber(prober ComponentOAuthProber) {
	h.componentOAuth = prober
}

// ProbeComponentOAuth learns whether the remote components that agents reach ask for OAuth, by probing them without any
// credential, and records what it finds in the vMCP's status (v1.VMCPStatus.ComponentOAuth), where agent readiness and
// profile status read it (vmcpconfig.ProbedOAuthRequirement). That is how a server that asks for OAuth, with dynamic
// client registration rather than static OAuth, is known to need a grant before an agent connects to it. Probes never
// run on a request.
//
// It probes each remote component that an agent-capable profile grants and that is probed at all
// (vmcpconfig.ComponentOAuthProbed), at each URL agents reach: the URL its definition fixes, or each distinct URL that
// its URL configuration resolves to for the profiles that grant it (vmcpconfig.ProfileGrantSelection). A URL is probed:
//
//   - when it is first seen: a component was added or granted, its definition changed, or a profile's configuration
//     now resolves to a URL that has not been probed;
//   - componentOAuthRecheckInterval after it was last probed successfully, to notice a server that starts or stops
//     asking for OAuth;
//   - after a probe that failed, once a wait that doubles with each failure in a row, from componentOAuthRetryBase up
//     to componentOAuthRetryMax, has passed.
//
// A failed probe records VMCPComponentOAuthUnknown, which counts as not required, or keeps what the last successful
// probe of the same URL found. Results for URLs, components or definitions no longer probed are dropped. Only digests
// of URLs are recorded, never a URL. Probes of one vMCP run a few at a time, each bounded by
// componentOAuthProbeTimeout, and the vMCP is requeued for the next one that is due.
func (h *Handler) ProbeComponentOAuth(req router.Request, resp router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	if !vmcp.DeletionTimestamp.IsZero() || h.componentOAuth == nil {
		return nil
	}
	desired, err := h.componentOAuthTargets(req.Ctx, *vmcp)
	if err != nil {
		return err
	}

	now := h.clock().Truncate(time.Second)
	var (
		next = map[string]v1.VMCPComponentOAuth{}
		due  []componentOAuthProbeJob
	)
	for _, componentID := range slices.Sorted(maps.Keys(desired)) {
		want := desired[componentID]
		previous := vmcp.Status.ComponentOAuth[componentID]
		record := v1.VMCPComponentOAuth{DefinitionHash: want.definitionHash}
		for _, target := range want.targets {
			probe := v1.VMCPComponentOAuthProbe{URLDigest: target.digest, Required: v1.VMCPComponentOAuthUnknown}
			if previous.DefinitionHash == want.definitionHash {
				if index := slices.IndexFunc(previous.Probes, func(p v1.VMCPComponentOAuthProbe) bool { return p.URLDigest == target.digest }); index >= 0 {
					probe = previous.Probes[index]
				}
			}
			if probe.CheckedAt.IsZero() || !now.Before(nextComponentOAuthProbe(probe)) {
				due = append(due, componentOAuthProbeJob{componentID: componentID, index: len(record.Probes), url: target.url, tunnel: want.tunnelName})
			}
			record.Probes = append(record.Probes, probe)
		}
		next[componentID] = record
	}

	h.runComponentOAuthProbes(req.Ctx, due)
	for _, job := range due {
		probe := &next[job.componentID].Probes[job.index]
		probe.CheckedAt = metav1.NewTime(now)
		if job.err != nil {
			probe.Failures++
			if probe.Required != v1.VMCPComponentOAuthRequired && probe.Required != v1.VMCPComponentOAuthNotRequired {
				probe.Required = v1.VMCPComponentOAuthUnknown
			}
			// The error may hold the URL, which may come from configuration.
			slog.Warn("Failed to learn whether a vMCP component asks for OAuth; agents are not kept from it for that", "vmcp", vmcp.Name, "component", job.componentID,
				"failures", probe.Failures, "nextProbe", nextComponentOAuthProbe(*probe), "error", vmcpconfig.RedactURL(job.err.Error(), job.url))
			continue
		}
		required := v1.VMCPComponentOAuthNotRequired
		if job.required {
			required = v1.VMCPComponentOAuthRequired
		}
		if probe.Required != required {
			slog.Info("Probed whether a vMCP component asks for OAuth", "vmcp", vmcp.Name, "component", job.componentID, "required", required)
		}
		probe.Required, probe.Failures = required, 0
	}

	var wait time.Duration
	for _, record := range next {
		for _, probe := range record.Probes {
			if until := nextComponentOAuthProbe(probe).Sub(now); wait == 0 || until < wait {
				wait = until
			}
		}
	}
	if wait > 0 {
		resp.RetryAfter(wait)
	}

	if len(next) == 0 {
		next = nil
	}
	if equality.Semantic.DeepEqual(next, vmcp.Status.ComponentOAuth) {
		return nil
	}
	return h.saveComponentOAuth(req, vmcp, next)
}

// saveComponentOAuth records next as vmcp's probes. A conflict, from a change made while the probes ran, is retried on
// the vMCP as it is now, with the probes of components that are gone or whose definition changed left out; probes of
// URLs that are no longer reached are dropped by the next run, which the save starts.
func (h *Handler) saveComponentOAuth(req router.Request, vmcp *v1.VMCP, next map[string]v1.VMCPComponentOAuth) error {
	first := true
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !first {
			if err := req.Client.Get(req.Ctx, kclient.ObjectKeyFromObject(vmcp), vmcp); err != nil {
				return err
			}
		}
		first = false
		current := map[string]string{}
		for _, component := range vmcpconfig.ComponentsForInstance(*vmcp, v1.VMCPInstance{}) {
			current[component.ID] = vmcpconfig.ProfileOAuthGrantDefinitionHash(component)
		}
		kept := maps.Clone(next)
		maps.DeleteFunc(kept, func(componentID string, record v1.VMCPComponentOAuth) bool {
			return current[componentID] != record.DefinitionHash
		})
		if len(kept) == 0 {
			kept = nil
		}
		vmcp.Status.ComponentOAuth = kept
		return req.Client.Status().Update(req.Ctx, vmcp)
	})
}

// runComponentOAuthProbes probes the URL of each job, componentOAuthProbeConcurrency at a time, each bounded by
// componentOAuthProbeTimeout, and records the result in the job.
func (h *Handler) runComponentOAuthProbes(ctx context.Context, jobs []componentOAuthProbeJob) {
	var (
		wg      sync.WaitGroup
		running = make(chan struct{}, componentOAuthProbeConcurrency)
	)
	for i := range jobs {
		wg.Add(1)
		running <- struct{}{}
		go func(job *componentOAuthProbeJob) {
			defer func() {
				<-running
				wg.Done()
			}()
			probeCtx, cancel := context.WithTimeout(ctx, componentOAuthProbeTimeout)
			defer cancel()
			job.required, job.err = h.componentOAuth.ProbeOAuthRequirement(probeCtx, job.url, job.tunnel)
		}(&jobs[i])
	}
	wg.Wait()
}

// componentOAuthTargets returns, by component ID, the URLs of vmcp's components to probe (ProbeComponentOAuth). Only a
// shared vMCP's components are, since agents connect to no other, and only those that an agent-capable profile grants.
// A URL that comes from configuration is resolved for each such profile as for an agent that uses the profile's grants
// (vmcpconfig.ProfileGrantSelection), and skipped when it lacks a value or is refused: configuration readiness reports
// that.
func (h *Handler) componentOAuthTargets(ctx context.Context, vmcp v1.VMCP) (map[string]componentOAuthTargets, error) {
	if vmcp.Spec.UserID != "" {
		return nil, nil
	}
	var profiles []types.VMCPProfile
	for _, profile := range vmcp.Spec.Manifest.Profiles {
		if profile.IsAgentCapable() {
			profiles = append(profiles, profile)
		}
	}
	if len(profiles) == 0 {
		return nil, nil
	}

	result := map[string]componentOAuthTargets{}
	for _, component := range vmcpconfig.ComponentsForInstance(vmcp, v1.VMCPInstance{}) {
		remote := component.CatalogEntry.Manifest.RemoteConfig
		if component.ID == "" || remote == nil || !vmcpconfig.ComponentOAuthProbed(component) {
			continue
		}
		granting := slices.DeleteFunc(slices.Clone(profiles), func(profile types.VMCPProfile) bool {
			return !vmcpconfig.ProfileGrantsComponent(profile, component.ID)
		})
		if len(granting) == 0 {
			continue
		}

		var urls []string
		for _, profile := range granting {
			target, refused, err := vmcpconfig.ProfileGrantTarget(ctx, h.componentOAuth, vmcp, profile, component)
			if err != nil {
				return nil, fmt.Errorf("resolve the upstream URL of component %q of vMCP %q for profile %q: %w", component.ID, vmcp.Name, profile.ID, err)
			}
			if refused == nil && target.URL != "" && !slices.Contains(urls, target.URL) {
				urls = append(urls, target.URL)
			}
		}
		if len(urls) == 0 {
			continue
		}

		want := componentOAuthTargets{definitionHash: vmcpconfig.ProfileOAuthGrantDefinitionHash(component), tunnelName: remote.TunnelName}
		for _, url := range urls {
			want.targets = append(want.targets, componentOAuthTarget{url: url, digest: vmcpconfig.ComponentOAuthURLDigest(vmcp.Name, component.ID, url)})
		}
		// The order of the profiles is not the order of the probes, so that reordering profiles changes nothing.
		slices.SortFunc(want.targets, func(a, b componentOAuthTarget) int { return cmp.Compare(a.digest, b.digest) })
		result[component.ID] = want
	}
	return result, nil
}

// nextComponentOAuthProbe is when the URL that probe is for is probed next: componentOAuthRecheckInterval after a probe
// that succeeded, and sooner after one that failed, the more failures in a row the later.
func nextComponentOAuthProbe(probe v1.VMCPComponentOAuthProbe) time.Time {
	if probe.Failures == 0 {
		return probe.CheckedAt.Add(componentOAuthRecheckInterval)
	}
	wait := componentOAuthRetryMax
	if probe.Failures <= 7 {
		wait = min(componentOAuthRetryBase<<(probe.Failures-1), componentOAuthRetryMax)
	}
	return probe.CheckedAt.Add(wait)
}

func (h *Handler) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}
