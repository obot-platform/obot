// Package agentsource syncs hosted agents from a git repository, mirroring the
// skillrepository package for skills.
//
// The discovery half is a placeholder: nothing is cloned and no agents are
// produced. The surrounding reconciler is real — it throttles, records status,
// honors the force-sync annotation, and prunes agents whose source no longer
// lists them — so replacing the fetcher with a real one is the only work left.
// See README.md in the hostedagent handler package for the full list of gaps.
package agentsource

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const syncInterval = time.Hour

// fetchedSource is what a real fetcher would hand back: a checked out copy of
// the repository plus the commit it resolved to.
type fetchedSource struct {
	RepoRoot  string
	CommitSHA string
	Cleanup   func()
}

type sourceFetcher interface {
	Fetch(ctx context.Context, repoURL, ref string) (*fetchedSource, error)
}

// placeholderFetcher stands in for real git access. It resolves nothing and
// checks nothing out, so discovery always yields zero agents.
type placeholderFetcher struct{}

func (placeholderFetcher) Fetch(_ context.Context, _, _ string) (*fetchedSource, error) {
	return &fetchedSource{Cleanup: func() {}}, nil
}

type Handler struct {
	fetcher sourceFetcher
	now     func() time.Time
}

func New() *Handler {
	return &Handler{
		fetcher: placeholderFetcher{},
		now:     time.Now,
	}
}

func (h *Handler) Sync(req router.Request, resp router.Response) error {
	source := req.Object.(*v1.AgentSource)
	namespace := source.Namespace

	forceSync := source.Annotations[v1.AgentSourceSyncAnnotation] == "true"
	if !forceSync && !source.Status.LastSyncTime.IsZero() {
		timeSinceLastSync := h.now().Sub(source.Status.LastSyncTime.Time)
		if timeSinceLastSync < syncInterval {
			resp.RetryAfter(syncInterval - timeSinceLastSync)
			return nil
		}
	}

	source.Status.IsSyncing = true
	if err := req.Client.Status().Update(req.Ctx, source); err != nil {
		return fmt.Errorf("failed to mark agent source syncing: %w", err)
	}

	defer h.clearIsSyncing(req.Ctx, req.Client, namespace, source.Name)

	fetched, err := h.fetcher.Fetch(req.Ctx, source.Spec.RepoURL, source.Spec.Ref)
	if err != nil {
		if statusErr := h.recordFailure(req.Ctx, req.Client, namespace, source.Name, err); statusErr != nil {
			return statusErr
		}
		resp.RetryAfter(syncInterval)
		return nil
	}
	defer fetched.Cleanup()

	agents, err := buildAgentsFromSource(fetched.RepoRoot, source, fetched.CommitSHA)
	if err != nil {
		if statusErr := h.recordFailure(req.Ctx, req.Client, namespace, source.Name, err); statusErr != nil {
			return statusErr
		}
		resp.RetryAfter(syncInterval)
		return nil
	}

	if err := upsertAgents(req.Ctx, req.Client, namespace, source.Name, agents); err != nil {
		if statusErr := h.recordFailure(req.Ctx, req.Client, namespace, source.Name, err); statusErr != nil {
			return statusErr
		}
		resp.RetryAfter(syncInterval)
		return nil
	}

	if err := h.recordSuccess(req.Ctx, req.Client, namespace, source.Name, fetched.CommitSHA, len(agents)); err != nil {
		return err
	}

	if forceSync {
		if err := clearSyncAnnotation(req.Ctx, req.Client, namespace, source.Name); err != nil {
			return err
		}
	}

	resp.RetryAfter(syncInterval)
	return nil
}

// buildAgentsFromSource is where a real implementation would walk the checked
// out repository and turn each agent definition it finds into a HostedAgent,
// the way skillrepository's scan looks for SKILL.md. It currently finds nothing.
func buildAgentsFromSource(_ string, _ *v1.AgentSource, _ string) ([]*v1.HostedAgent, error) {
	return nil, nil
}

// upsertAgents reconciles the agents a source claims against the ones already
// stored for it: create what is new, update what changed, delete what the source
// no longer lists. Agents registered by hand carry no SourceID and are never
// listed here, so they are untouched.
func upsertAgents(ctx context.Context, c client.Client, namespace, sourceID string, agents []*v1.HostedAgent) error {
	existing, err := listAgentsForSource(ctx, c, namespace, sourceID)
	if err != nil {
		return err
	}

	desired := make(map[string]*v1.HostedAgent, len(agents))
	for _, agent := range agents {
		desired[agent.Name] = agent
	}

	for _, agent := range agents {
		current, ok := existing[agent.Name]
		if !ok {
			if err := c.Create(ctx, agent); err != nil {
				return fmt.Errorf("failed to create hosted agent %s: %w", agent.Name, err)
			}
			continue
		}

		current.Spec = agent.Spec
		if err := c.Update(ctx, current); err != nil {
			return fmt.Errorf("failed to update hosted agent %s: %w", agent.Name, err)
		}
	}

	for name, current := range existing {
		if _, ok := desired[name]; ok {
			continue
		}
		if err := c.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete hosted agent %s: %w", name, err)
		}
	}

	return nil
}

func listAgentsForSource(ctx context.Context, c client.Client, namespace, sourceID string) (map[string]*v1.HostedAgent, error) {
	var list v1.HostedAgentList
	if err := c.List(ctx, &list, client.InNamespace(namespace), client.MatchingFields{"spec.sourceID": sourceID}); err != nil {
		return nil, fmt.Errorf("failed to list agents for source: %w", err)
	}

	result := make(map[string]*v1.HostedAgent, len(list.Items))
	for i := range list.Items {
		result[list.Items[i].Name] = &list.Items[i]
	}
	return result, nil
}

func (h *Handler) recordFailure(ctx context.Context, c client.Client, namespace, name string, syncErr error) error {
	var source v1.AgentSource
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload agent source: %w", err)
	}

	source.Status.LastSyncTime = metav1.NewTime(h.now())
	source.Status.SyncError = syncErr.Error()
	return c.Status().Update(ctx, &source)
}

func (h *Handler) recordSuccess(ctx context.Context, c client.Client, namespace, name, commitSHA string, agentCount int) error {
	var source v1.AgentSource
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload agent source: %w", err)
	}

	source.Status.LastSyncTime = metav1.NewTime(h.now())
	source.Status.SyncError = ""
	source.Status.ResolvedCommitSHA = commitSHA
	source.Status.DiscoveredAgentCount = agentCount
	return c.Status().Update(ctx, &source)
}

func (h *Handler) clearIsSyncing(ctx context.Context, c client.Client, namespace, name string) {
	var source v1.AgentSource
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Errorf("failed to reload agent source %s to clear syncing bit: %v", name, err)
		}
		return
	}

	if !source.Status.IsSyncing {
		return
	}

	source.Status.IsSyncing = false
	if err := c.Status().Update(ctx, &source); err != nil && !apierrors.IsNotFound(err) {
		log.Errorf("failed to clear syncing bit for agent source %s: %v", name, err)
	}
}

func clearSyncAnnotation(ctx context.Context, c client.Client, namespace, name string) error {
	var source v1.AgentSource
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload agent source for annotation cleanup: %w", err)
	}

	if source.Annotations == nil {
		return nil
	}
	if _, ok := source.Annotations[v1.AgentSourceSyncAnnotation]; !ok {
		return nil
	}

	delete(source.Annotations, v1.AgentSourceSyncAnnotation)
	return c.Update(ctx, &source)
}
