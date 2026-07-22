// Package agentsource syncs hosted agents and harnesses from a git
// repository, mirroring the skillrepository package for skills.
//
// The discovery half is a placeholder: nothing is cloned and nothing is
// produced. The surrounding reconciler is real — it throttles, records status,
// honors the force-sync annotation, resolves harness references, and prunes
// resources the source no longer lists — so replacing the fetcher with a real
// one is the only work left. See README.md in the hostedagent handler package
// for the full list of gaps.
//
// ID alignment: a repository cannot know the generated resource names its
// harnesses will be stored under, so discovered agents reference harnesses by
// the harness's relative path within the same source. Stored object names are
// deterministic (harnessObjectName), and resolveHarnessReferences rewrites
// each path reference to that name before anything is persisted. A reference
// that already carries the harness ID prefix is passed through untouched — it
// names a harness registered outside the source.
package agentsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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

	found, err := buildFromSource(fetched.RepoRoot, source, fetched.CommitSHA)
	if err == nil {
		err = resolveHarnessReferences(found.Agents, found.Harnesses)
	}
	if err == nil {
		err = syncDiscovered(req.Ctx, req.Client, namespace, source.Name, found)
	}
	if err != nil {
		if statusErr := h.recordFailure(req.Ctx, req.Client, namespace, source.Name, err); statusErr != nil {
			return statusErr
		}
		resp.RetryAfter(syncInterval)
		return nil
	}

	if err := h.recordSuccess(req.Ctx, req.Client, namespace, source.Name, fetched.CommitSHA, len(found.Agents), len(found.Harnesses)); err != nil {
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

// discovered is everything one sync of a source yields: harnesses and the
// agents built on them.
type discovered struct {
	Harnesses []*v1.Harness
	Agents    []*v1.HostedAgent
}

// buildFromSource is where a real implementation would walk the checked out
// repository and turn each definition it finds into a Harness or HostedAgent,
// the way skillrepository's scan looks for SKILL.md. It currently finds
// nothing.
//
// Contract for a real implementation:
//   - object names come from harnessObjectName / agentObjectName so the same
//     definition maps to the same resource on every sync;
//   - each object carries SourceID, RelativePath, and CommitSHA;
//   - an agent's HarnessID may be either the relative path of a harness in
//     this same source, or a full ID (system.HarnessPrefix) of a harness that
//     already exists outside it. Path references are rewritten by
//     resolveHarnessReferences after this returns.
func buildFromSource(_ string, _ *v1.AgentSource, _ string) (*discovered, error) {
	return &discovered{}, nil
}

// resolveHarnessReferences rewrites each discovered agent's harness reference
// to the object name of the harness discovered alongside it. This is the ID
// alignment step: repositories reference harnesses by relative path because
// they cannot know generated resource names, and object names are
// deterministic, so the same path resolves to the same resource on every
// sync. An unknown reference fails the whole sync — better a visible sync
// error than a stored agent pointing at nothing.
func resolveHarnessReferences(agents []*v1.HostedAgent, harnesses []*v1.Harness) error {
	byPath := make(map[string]string, len(harnesses))
	for _, harness := range harnesses {
		byPath[harness.Spec.RelativePath] = harness.Name
	}

	for _, agent := range agents {
		ref := agent.Spec.Manifest.HarnessID
		if ref == "" {
			return fmt.Errorf("agent %s does not name a harness", agent.Spec.RelativePath)
		}
		if strings.HasPrefix(ref, system.HarnessPrefix) {
			continue
		}
		resolved, ok := byPath[ref]
		if !ok {
			return fmt.Errorf("agent %s references harness %q, which this source does not contain", agent.Spec.RelativePath, ref)
		}
		agent.Spec.Manifest.HarnessID = resolved
	}

	return nil
}

// syncDiscovered reconciles both kinds in an order that keeps references
// intact at every step: new harnesses are stored before the agents that
// reference them, and stale agents are removed before the harnesses they
// referenced.
func syncDiscovered(ctx context.Context, c client.Client, namespace, sourceID string, found *discovered) error {
	staleHarnesses, err := upsertHarnesses(ctx, c, namespace, sourceID, found.Harnesses)
	if err != nil {
		return err
	}

	if err := upsertAgents(ctx, c, namespace, sourceID, found.Agents); err != nil {
		return err
	}

	for _, harness := range staleHarnesses {
		// A harness the source dropped can still be referenced by an agent
		// registered outside it. Leave it in place rather than dangling that
		// agent; it keeps its SourceID, so a later sync retries once the
		// reference is gone.
		var agents v1.HostedAgentList
		if err := c.List(ctx, &agents, client.InNamespace(namespace), client.MatchingFields{"spec.harnessID": harness.Name}); err != nil {
			return fmt.Errorf("failed to list agents for harness %s: %w", harness.Name, err)
		}
		if len(agents.Items) > 0 {
			log.Infof("keeping harness %s dropped by source %s: still referenced by %d agent(s)", harness.Name, sourceID, len(agents.Items))
			continue
		}

		if err := c.Delete(ctx, harness); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete harness %s: %w", harness.Name, err)
		}
	}

	return nil
}

// upsertHarnesses creates and updates the harnesses a source claims. Stale
// ones — stored for this source but no longer listed by it — are returned
// rather than deleted, so the caller can remove them after the agents that
// referenced them are gone. Harnesses registered by hand carry no SourceID
// and are never listed here, so they are untouched.
func upsertHarnesses(ctx context.Context, c client.Client, namespace, sourceID string, harnesses []*v1.Harness) ([]*v1.Harness, error) {
	var list v1.HarnessList
	if err := c.List(ctx, &list, client.InNamespace(namespace), client.MatchingFields{"spec.sourceID": sourceID}); err != nil {
		return nil, fmt.Errorf("failed to list harnesses for source: %w", err)
	}

	existing := make(map[string]*v1.Harness, len(list.Items))
	for i := range list.Items {
		existing[list.Items[i].Name] = &list.Items[i]
	}

	desired := make(map[string]struct{}, len(harnesses))
	for _, harness := range harnesses {
		desired[harness.Name] = struct{}{}

		current, ok := existing[harness.Name]
		if !ok {
			if err := c.Create(ctx, harness); err != nil {
				return nil, fmt.Errorf("failed to create harness %s: %w", harness.Name, err)
			}
			continue
		}

		current.Spec = harness.Spec
		if err := c.Update(ctx, current); err != nil {
			return nil, fmt.Errorf("failed to update harness %s: %w", harness.Name, err)
		}
	}

	var stale []*v1.Harness
	for objectName, current := range existing {
		if _, ok := desired[objectName]; !ok {
			stale = append(stale, current)
		}
	}
	return stale, nil
}

// harnessObjectName and agentObjectName give discovered resources stable,
// deterministic names, copying the skillrepository scheme: the visible
// portion is sanitized for RFC 1123, and a hash over the raw inputs keeps
// paths that sanitize identically from colliding.
func harnessObjectName(sourceID, relPath string) string {
	return sourceObjectName(system.HarnessPrefix, sourceID, relPath)
}

func agentObjectName(sourceID, relPath string) string {
	return sourceObjectName(system.HostedAgentPrefix, sourceID, relPath)
}

func sourceObjectName(prefix, sourceID, relPath string) string {
	fragment := sanitizeNameFragment(relPath)
	if fragment == "" {
		fragment = "item"
	}
	d := sha256.New()
	for _, part := range []string{prefix, sourceID, fragment, relPath} {
		d.Write([]byte(part))
		d.Write([]byte{0})
	}
	suffix := hex.EncodeToString(d.Sum(nil))[:8]
	return name.SafeConcatName(prefix, sourceID, fragment, suffix)
}

func sanitizeNameFragment(value string) string {
	replacer := strings.NewReplacer("/", "-", "_", "-", ".", "-", " ", "-")
	value = strings.ToLower(replacer.Replace(value))

	var b strings.Builder
	lastDash := false
	for _, ch := range value {
		valid := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if valid {
			b.WriteRune(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
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

	for objectName, current := range existing {
		if _, ok := desired[objectName]; ok {
			continue
		}
		if err := c.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete hosted agent %s: %w", objectName, err)
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

func (h *Handler) recordSuccess(ctx context.Context, c client.Client, namespace, sourceName, commitSHA string, agentCount, harnessCount int) error {
	var source v1.AgentSource
	if err := c.Get(ctx, router.Key(namespace, sourceName), &source); err != nil {
		return fmt.Errorf("failed to reload agent source: %w", err)
	}

	source.Status.LastSyncTime = metav1.NewTime(h.now())
	source.Status.SyncError = ""
	source.Status.ResolvedCommitSHA = commitSHA
	source.Status.DiscoveredAgentCount = agentCount
	source.Status.DiscoveredHarnessCount = harnessCount
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
