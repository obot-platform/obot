package vmcp

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// componentOAuthRecheckInterval is how long a probe that succeeded stands before the component is probed again, to
	// notice a server that starts or stops asking for OAuth.
	componentOAuthRecheckInterval = 24 * time.Hour
	// componentOAuthRetryBase and componentOAuthRetryMax bound the wait before a component whose probe failed is probed
	// again. The wait doubles with each failure in a row.
	componentOAuthRetryBase = time.Minute
	componentOAuthRetryMax  = time.Hour
	// componentOAuthProbeTimeout bounds one probe, all its requests included. Each request is also bounded by the
	// OAuth client's own timeout.
	componentOAuthProbeTimeout = 30 * time.Second
	// componentOAuthProbeConcurrency is how many components of one vMCP are probed at once.
	componentOAuthProbeConcurrency = 4
)

// ComponentOAuthProber probes remote MCP servers for whether they ask for OAuth, and resolves the URL that an agent's
// connection to a component reaches. The MCP session manager implements it.
type ComponentOAuthProber interface {
	vmcpconfig.ComponentTargetResolver
	ProbeOAuthRequirement(ctx context.Context, url, tunnelName string) (bool, error)
}

// componentOAuthTarget is the URL of a component to probe, for its current target.
type componentOAuthTarget struct {
	targetHash string
	url        string
	tunnelName string
}

// componentOAuthProbeJob is one component due for a probe, and what the probe found.
type componentOAuthProbeJob struct {
	componentID string
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
// the vMCP's agent setup read it (vmcpconfig.ProbedOAuthRequirement). That is how a server that asks for OAuth, with
// dynamic client registration rather than static OAuth, is known to need a grant before an agent connects to it.
// Probes never run on a request.
//
// It probes each remote component that any profile naming an agent grants (vmcpconfig.AgentGrantedComponents) and that
// is probed at all (vmcpconfig.ComponentOAuthProbed), at the URL agents reach: the URL its definition fixes, or the one
// built from the values the vMCP and the catalog entry fix (vmcpconfig.ResolveComponentTarget). A component is probed:
//
//   - when it is first seen, or its target changes (vmcpconfig.ComponentOAuthTargetHash): a component was added or
//     granted, its definition changed, or the values its URL is built from changed;
//   - componentOAuthRecheckInterval after it was last probed successfully, to notice a server that starts or stops
//     asking for OAuth;
//   - after a probe that failed, once a wait that doubles with each failure in a row, from componentOAuthRetryBase up
//     to componentOAuthRetryMax, has passed.
//
// A failed probe records VMCPComponentOAuthUnknown, which counts as not required, or keeps what the last successful
// probe of the same target found. Results for components or targets no longer probed are dropped. No URL is recorded.
// Probes of one vMCP run a few at a time, each bounded by componentOAuthProbeTimeout, and the vMCP is requeued for the
// next one that is due.
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
		record := v1.VMCPComponentOAuth{TargetHash: want.targetHash, Required: v1.VMCPComponentOAuthUnknown}
		if previous, ok := vmcp.Status.ComponentOAuth[componentID]; ok && previous.TargetHash == want.targetHash {
			record = previous
		}
		if record.CheckedAt.IsZero() || !now.Before(nextComponentOAuthProbe(record)) {
			due = append(due, componentOAuthProbeJob{componentID: componentID, url: want.url, tunnel: want.tunnelName})
		}
		next[componentID] = record
	}

	h.runComponentOAuthProbes(req.Ctx, due)
	for _, job := range due {
		record := next[job.componentID]
		record.CheckedAt = metav1.NewTime(now)
		if job.err != nil {
			record.Failures++
			if record.Required != v1.VMCPComponentOAuthRequired && record.Required != v1.VMCPComponentOAuthNotRequired {
				record.Required = v1.VMCPComponentOAuthUnknown
			}
			next[job.componentID] = record
			// The error may hold the URL, which may come from the vMCP's fixed configuration.
			slog.Warn("Failed to learn whether a vMCP component asks for OAuth; agents are not kept from it for that", "vmcp", vmcp.Name, "component", job.componentID,
				"failures", record.Failures, "nextProbe", nextComponentOAuthProbe(record), "error", vmcpconfig.RedactURL(job.err.Error(), job.url))
			continue
		}
		required := v1.VMCPComponentOAuthNotRequired
		if job.required {
			required = v1.VMCPComponentOAuthRequired
		}
		if record.Required != required {
			slog.Info("Probed whether a vMCP component asks for OAuth", "vmcp", vmcp.Name, "component", job.componentID, "required", required)
		}
		record.Required, record.Failures = required, 0
		next[job.componentID] = record
	}

	var wait time.Duration
	for _, record := range next {
		if until := nextComponentOAuthProbe(record).Sub(now); wait == 0 || until < wait {
			wait = until
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
// the vMCP as it is now, with the probes of components that are gone or whose target changed left out; the save starts
// the next run, which probes them.
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
			current[component.ID] = vmcpconfig.ComponentOAuthTargetHash(*vmcp, component)
		}
		kept := maps.Clone(next)
		maps.DeleteFunc(kept, func(componentID string, record v1.VMCPComponentOAuth) bool {
			return current[componentID] != record.TargetHash
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

// componentOAuthTargets returns, by component ID, the URLs of vmcp's components to probe (ProbeComponentOAuth): those
// that any profile naming an agent grants (vmcpconfig.AgentGrantedComponents), which only a shared vMCP has, since
// agents connect to no other. A component whose URL lacks a value that the vMCP and the catalog entry do not fix, or
// whose fixed URL is refused, is skipped: configuration readiness reports it.
func (h *Handler) componentOAuthTargets(ctx context.Context, vmcp v1.VMCP) (map[string]componentOAuthTarget, error) {
	components := vmcpconfig.AgentGrantedComponents(vmcp)
	if len(components) == 0 {
		return nil, nil
	}

	result := map[string]componentOAuthTarget{}
	for _, component := range components {
		remote := component.CatalogEntry.Manifest.RemoteConfig
		if component.ID == "" || remote == nil || !vmcpconfig.ComponentOAuthProbed(component) {
			continue
		}
		target, refused, err := vmcpconfig.ResolveComponentTarget(ctx, h.componentOAuth, vmcp, component)
		if err != nil {
			return nil, fmt.Errorf("resolve the upstream URL of component %q of vMCP %q: %w", component.ID, vmcp.Name, err)
		}
		if refused != nil || target.URL == "" {
			continue
		}
		result[component.ID] = componentOAuthTarget{
			targetHash: vmcpconfig.ComponentOAuthTargetHash(vmcp, component),
			url:        target.URL,
			tunnelName: remote.TunnelName,
		}
	}
	return result, nil
}

// nextComponentOAuthProbe is when the component that probe is for is probed next: componentOAuthRecheckInterval after a
// probe that succeeded, and sooner after one that failed, the more failures in a row the later.
func nextComponentOAuthProbe(probe v1.VMCPComponentOAuth) time.Time {
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
