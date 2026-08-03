// Package agentcatalog syncs hosted agents and harnesses from a git repository,
// mirroring the skillrepository package for skills.
//
// ID alignment: a repository cannot know the generated resource names its
// harnesses will be stored under, so discovered agents reference harnesses by
// the harness's relative path within the same source. Stored object names are
// deterministic (harnessObjectName), and resolveHarnessReferences rewrites
// each path reference to that name before anything is persisted. A reference
// that already carries the harness ID prefix is passed through untouched — it
// names a harness registered outside the source.
package agentcatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	gitpkg "github.com/obot-platform/obot/pkg/git"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

const (
	syncInterval       = time.Hour
	harnessDefinition  = "harness.yaml"
	agentDefinition    = "agent.yaml"
	maxDefinitionBytes = 1024 * 1024
	maxDefinitions     = 1000
)

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

type gitSourceFetcher struct{}

func (gitSourceFetcher) Fetch(ctx context.Context, repoURL, ref string) (*fetchedSource, error) {
	if localPath, ok, err := localRepositoryPath(repoURL); err != nil {
		return nil, err
	} else if ok {
		return fetchLocalRepository(localPath, ref)
	}

	dir, commitSHA, cleanup, err := gitpkg.Clone(ctx, repoURL, "", ref)
	if err != nil {
		return nil, err
	}
	return &fetchedSource{
		RepoRoot:  dir,
		CommitSHA: commitSHA,
		Cleanup:   cleanup,
	}, nil
}

type Handler struct {
	fetcher sourceFetcher
	now     func() time.Time
}

func New() *Handler {
	return &Handler{
		fetcher: gitSourceFetcher{},
		now:     time.Now,
	}
}

func (h *Handler) Sync(req router.Request, resp router.Response) error {
	source := req.Object.(*v1.AgentCatalog)
	namespace := source.Namespace

	forceSync := source.Annotations[v1.AgentCatalogSyncAnnotation] == "true"
	// A source observed by the former placeholder fetcher has a sync time but
	// no resolved commit. Do not throttle it after upgrading to the real
	// backend; reconcile it immediately.
	if !forceSync && source.Status.ResolvedCommitSHA != "" && !source.Status.LastSyncTime.IsZero() {
		timeSinceLastSync := h.now().Sub(source.Status.LastSyncTime.Time)
		if timeSinceLastSync < syncInterval {
			resp.RetryAfter(syncInterval - timeSinceLastSync)
			return nil
		}
	}

	source.Status.IsSyncing = true
	if err := req.Client.Status().Update(req.Ctx, source); err != nil {
		return fmt.Errorf("failed to mark agent catalog syncing: %w", err)
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

// buildFromSource recursively discovers strict YAML manifests. Symlinks and
// .git metadata are ignored so definitions cannot escape the checked-out
// repository or accidentally ingest Git internals.
func buildFromSource(repoRoot string, source *v1.AgentCatalog, commitSHA string) (*discovered, error) {
	definitions, err := discoverDefinitions(repoRoot)
	if err != nil {
		return nil, err
	}

	result := &discovered{}
	for _, definition := range definitions {
		relPath, err := filepath.Rel(repoRoot, definition.path)
		if err != nil {
			return nil, fmt.Errorf("determine repository-relative definition path: %w", err)
		}
		relPath = filepath.ToSlash(relPath)

		switch definition.kind {
		case harnessDefinition:
			var manifest types.HarnessManifest
			if err := readDefinition(definition.path, &manifest); err != nil {
				return nil, fmt.Errorf("%s: %w", relPath, err)
			}
			if err := manifest.Validate(); err != nil {
				return nil, fmt.Errorf("%s: invalid harness: %w", relPath, err)
			}
			result.Harnesses = append(result.Harnesses, &v1.Harness{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "Harness",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      harnessObjectName(source.Name, relPath),
					Namespace: source.Namespace,
				},
				Spec: v1.HarnessSpec{
					Manifest:     manifest,
					SourceID:     source.Name,
					RelativePath: relPath,
					CommitSHA:    commitSHA,
				},
			})
		case agentDefinition:
			var manifest types.HostedAgentManifest
			if err := readDefinition(definition.path, &manifest); err != nil {
				return nil, fmt.Errorf("%s: %w", relPath, err)
			}
			if err := manifest.Validate(); err != nil {
				return nil, fmt.Errorf("%s: invalid hosted agent: %w", relPath, err)
			}
			for _, env := range manifest.Env {
				if env.Sensitive && env.Value != "" {
					return nil, fmt.Errorf("%s: sensitive environment value %s must not be stored in source control", relPath, env.Key)
				}
			}
			result.Agents = append(result.Agents, &v1.HostedAgent{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "HostedAgent",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentObjectName(source.Name, relPath),
					Namespace: source.Namespace,
				},
				Spec: v1.HostedAgentSpec{
					Manifest:     manifest,
					SourceID:     source.Name,
					RelativePath: relPath,
					CommitSHA:    commitSHA,
				},
			})
		}
	}

	return result, nil
}

type sourceDefinition struct {
	path string
	kind string
}

func discoverDefinitions(repoRoot string) ([]sourceDefinition, error) {
	var result []sourceDefinition
	err := filepath.WalkDir(repoRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		switch entry.Name() {
		case harnessDefinition, agentDefinition:
			if len(result) >= maxDefinitions {
				return fmt.Errorf("too many agent catalog definitions (limit: %d)", maxDefinitions)
			}
			result = append(result, sourceDefinition{path: currentPath, kind: entry.Name()})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover agent catalog definitions: %w", err)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].path < result[j].path
	})
	return result, nil
}

func readDefinition(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open definition: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxDefinitionBytes+1))
	if err != nil {
		return fmt.Errorf("read definition: %w", err)
	}
	if len(content) > maxDefinitionBytes {
		return fmt.Errorf("definition exceeds maximum size of %d bytes", maxDefinitionBytes)
	}
	if err := yaml.UnmarshalStrict(content, target); err != nil {
		return fmt.Errorf("parse definition: %w", err)
	}
	return nil
}

func localRepositoryPath(repoURL string) (string, bool, error) {
	repoURL = strings.TrimSpace(repoURL)
	if filepath.IsAbs(repoURL) {
		return filepath.Clean(repoURL), true, nil
	}
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", false, fmt.Errorf("parse agent catalog URL: %w", err)
	}
	if parsed.Scheme != "file" {
		return "", false, nil
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		return "", false, fmt.Errorf("file repository host must be empty or localhost")
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", false, fmt.Errorf("decode file repository path: %w", err)
	}
	if !filepath.IsAbs(path) {
		return "", false, fmt.Errorf("file repository path must be absolute")
	}
	return filepath.Clean(path), true, nil
}

func fetchLocalRepository(repoRoot, ref string) (*fetchedSource, error) {
	info, err := os.Stat(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect local repository: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("local repository %s is not a directory", repoRoot)
	}

	repository, err := gogit.PlainOpen(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("open local Git repository: %w", err)
	}
	head, err := repository.Head()
	if err != nil {
		return nil, fmt.Errorf("resolve local repository HEAD: %w", err)
	}
	if ref != "" {
		hash, err := resolveLocalRef(repository, ref)
		if err != nil {
			return nil, err
		}
		if hash != head.Hash() {
			return nil, fmt.Errorf("local repository ref %q resolves to %s but the checked-out worktree is at %s; check out the requested ref first", ref, hash, head.Hash())
		}
	}

	return &fetchedSource{
		RepoRoot:  repoRoot,
		CommitSHA: head.Hash().String(),
		Cleanup:   func() {},
	}, nil
}

func resolveLocalRef(repository *gogit.Repository, ref string) (plumbing.Hash, error) {
	if plumbing.IsHash(ref) {
		hash := plumbing.NewHash(ref)
		if _, err := repository.CommitObject(hash); err != nil {
			return plumbing.ZeroHash, fmt.Errorf("resolve local repository commit %q: %w", ref, err)
		}
		return hash, nil
	}
	for _, candidate := range []plumbing.ReferenceName{
		plumbing.NewBranchReferenceName(ref),
		plumbing.NewTagReferenceName(ref),
		plumbing.ReferenceName(ref),
	} {
		reference, err := repository.Reference(candidate, true)
		if err == nil {
			return reference.Hash(), nil
		}
	}
	return plumbing.ZeroHash, fmt.Errorf("local repository ref %q was not found", ref)
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
	var source v1.AgentCatalog
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload agent catalog: %w", err)
	}

	source.Status.LastSyncTime = metav1.NewTime(h.now())
	source.Status.SyncError = syncErr.Error()
	return c.Status().Update(ctx, &source)
}

func (h *Handler) recordSuccess(ctx context.Context, c client.Client, namespace, sourceName, commitSHA string, agentCount, harnessCount int) error {
	var source v1.AgentCatalog
	if err := c.Get(ctx, router.Key(namespace, sourceName), &source); err != nil {
		return fmt.Errorf("failed to reload agent catalog: %w", err)
	}

	source.Status.LastSyncTime = metav1.NewTime(h.now())
	source.Status.SyncError = ""
	source.Status.ResolvedCommitSHA = commitSHA
	source.Status.DiscoveredAgentCount = agentCount
	source.Status.DiscoveredHarnessCount = harnessCount
	return c.Status().Update(ctx, &source)
}

func (h *Handler) clearIsSyncing(ctx context.Context, c client.Client, namespace, name string) {
	var source v1.AgentCatalog
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Errorf("failed to reload agent catalog %s to clear syncing bit: %v", name, err)
		}
		return
	}

	if !source.Status.IsSyncing {
		return
	}

	source.Status.IsSyncing = false
	if err := c.Status().Update(ctx, &source); err != nil && !apierrors.IsNotFound(err) {
		log.Errorf("failed to clear syncing bit for agent catalog %s: %v", name, err)
	}
}

func clearSyncAnnotation(ctx context.Context, c client.Client, namespace, name string) error {
	var source v1.AgentCatalog
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload agent catalog for annotation cleanup: %w", err)
	}

	if source.Annotations == nil {
		return nil
	}
	if _, ok := source.Annotations[v1.AgentCatalogSyncAnnotation]; !ok {
		return nil
	}

	delete(source.Annotations, v1.AgentCatalogSyncAnnotation)
	return c.Update(ctx, &source)
}
