// Package hostedagent reconciles hosted agent instances through the
// implementation-neutral agent backend contract.
package hostedagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/agentbackend"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/hostedagentrefs"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfields "k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const (
	transitionalPollInterval = 10 * time.Second
	readyPollInterval        = 5 * time.Minute
	agentConfigPath          = "/etc/obot/agent.json"
	// agentSecretsPath holds the credentials for the endpoints named in the
	// config. It is a separate file so the config itself needs no protection.
	agentSecretsPath = "/etc/obot/secrets.json"
	// workspacePath mirrors where backends mount the pool volume. It is told to
	// the agent so the writable, surviving directory does not have to be
	// guessed.
	workspacePath      = "/workspace"
	credentialFilePath = "/etc/obot/credential"
	poolDefaultsName   = "default"
)

// DesiredBuilder converts Obot resources into the runtime-ready backend
// contract. Keeping this boundary injectable lets resource and credential
// resolution evolve without coupling a backend adapter to Obot storage types.
type DesiredBuilder interface {
	Build(context.Context, BuildInput) (agentbackend.DesiredInstance, error)
}

// BuildInput is everything needed to render a sandbox's runtime contract.
//
// Client is here because the sandbox config resolves models and skills into
// complete endpoints and files, which means reading them; Credential is here
// because those endpoints carry it.
type BuildInput struct {
	Client     kclient.Client
	Namespace  string
	Instance   *v1.HostedAgentInstance
	Agent      *v1.HostedAgent
	Harness    *v1.Harness
	Credential string
}

// CredentialIssuer mints and revokes the credential a sandbox authenticates
// with. It is an interface so the controller can be tested without a database,
// and so the credential store stays replaceable.
type CredentialIssuer interface {
	// EnsureInstanceCredential returns the sandbox credential and an opaque
	// version that changes whenever the value does. It must be idempotent:
	// reconciles are frequent, and reissuing on every pass would restart the
	// sandbox on every pass.
	EnsureInstanceCredential(ctx context.Context, instanceID, ownerUserID string) (value string, version string, err error)
	// RevokeInstanceCredential is called during deletion and must tolerate a
	// credential that is already gone.
	RevokeInstanceCredential(ctx context.Context, instanceID string) error
}

type Handler struct {
	backend     agentbackend.InstanceBackend
	builder     DesiredBuilder
	credentials CredentialIssuer
	now         func() time.Time
}

func New(backend agentbackend.InstanceBackend, credentials CredentialIssuer, serverURL, internalURL string) *Handler {
	return NewWithBuilder(backend, credentials, defaultDesiredBuilder{
		ServerURL:   serverURL,
		InternalURL: internalURL,
		Skills:      newSkillFetcher(),
	})
}

func NewWithBuilder(backend agentbackend.InstanceBackend, credentials CredentialIssuer, builder DesiredBuilder) *Handler {
	return &Handler{
		backend:     backend,
		builder:     builder,
		credentials: credentials,
		now:         time.Now,
	}
}

// EnsurePool resolves the instance's pool before backend
// reconciliation. The deterministic names make concurrent first-instance
// reconciles for one user converge on the same pool and assignment.
func (h *Handler) EnsurePool(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.HostedAgentInstance)
	if instance.Spec.UserID == "" {
		return fmt.Errorf("hosted agent instance %q has no user ID", instance.Name)
	}

	if instance.Spec.PoolID != "" {
		assignments, err := assignmentsForUser(req, instance.Spec.UserID)
		if err != nil {
			return err
		}
		if isAssigned(assignments, instance.Spec.PoolID) {
			return nil
		}
		return fmt.Errorf("user %q is not assigned to hosted agent pool %q", instance.Spec.UserID, instance.Spec.PoolID)
	}

	assignments, err := assignmentsForUser(req, instance.Spec.UserID)
	if err != nil {
		return err
	}
	if poolID := defaultPool(assignments); poolID != "" {
		instance.Spec.PoolID = poolID
		return req.Client.Update(req.Ctx, instance)
	}

	var defaults v1.HostedAgentPoolDefaults
	if err := req.Get(&defaults, req.Namespace, poolDefaultsName); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("hosted agent pool defaults %q are not configured: %w", poolDefaultsName, err)
		}
		return err
	}
	if err := defaults.Spec.Manifest.Validate(); err != nil {
		return fmt.Errorf("hosted agent pool defaults %q are invalid: %w", poolDefaultsName, err)
	}

	poolID, assignmentID := initialPoolNames(instance.Spec.UserID)
	pool := &v1.HostedAgentPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      poolID,
			Namespace: req.Namespace,
		},
		Spec: v1.HostedAgentPoolSpec{
			Manifest: types.HostedAgentPoolManifest{
				Capacity:     defaults.Spec.Manifest.Capacity,
				MaxSandboxes: defaults.Spec.Manifest.MaxSandboxes,
				Suspended:    defaults.Spec.Manifest.Suspended,
			},
		},
	}
	if err := req.Client.Create(req.Ctx, pool); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create hosted agent pool: %w", err)
	}

	assignment := &v1.HostedAgentPoolAssignment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      assignmentID,
			Namespace: req.Namespace,
		},
		Spec: v1.HostedAgentPoolAssignmentSpec{
			Manifest: types.HostedAgentPoolAssignmentManifest{
				UserID:  instance.Spec.UserID,
				PoolID:  poolID,
				Default: true,
			},
		},
	}
	err = req.Client.Create(req.Ctx, assignment)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create hosted agent pool assignment: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		var persistedAssignment v1.HostedAgentPoolAssignment
		if err := req.Client.Get(req.Ctx, router.Key(req.Namespace, assignmentID), &persistedAssignment); err != nil {
			return fmt.Errorf("read hosted agent pool assignment after conflict: %w", err)
		}
		if persistedAssignment.Spec.Manifest.UserID != instance.Spec.UserID ||
			persistedAssignment.Spec.Manifest.PoolID != poolID ||
			!persistedAssignment.Spec.Manifest.Default {
			return fmt.Errorf("hosted agent pool assignment %q conflicts with the default assignment for user %q", assignmentID, instance.Spec.UserID)
		}
	}

	instance.Spec.PoolID = poolID
	return req.Client.Update(req.Ctx, instance)
}

func isAssigned(assignments []v1.HostedAgentPoolAssignment, poolID string) bool {
	for _, assignment := range assignments {
		if assignment.Spec.Manifest.PoolID == poolID {
			return true
		}
	}
	return false
}

func defaultPool(assignments []v1.HostedAgentPoolAssignment) string {
	for _, assignment := range assignments {
		if assignment.Spec.Manifest.Default {
			return assignment.Spec.Manifest.PoolID
		}
	}
	return ""
}

func initialPoolNames(userID string) (string, string) {
	suffix := hash.String(userID)[:16]
	return "hp-" + suffix, "hps-" + suffix
}

func assignmentsForUser(req router.Request, userID string) ([]v1.HostedAgentPoolAssignment, error) {
	var assignments v1.HostedAgentPoolAssignmentList
	if err := req.List(&assignments, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: kfields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return nil, fmt.Errorf("list hosted agent pool assignments: %w", err)
	}
	result := make([]v1.HostedAgentPoolAssignment, 0, len(assignments.Items))
	for _, assignment := range assignments.Items {
		if assignment.Spec.Manifest.UserID == userID {
			result = append(result, assignment)
		}
	}
	return result, nil
}

// OrchestrateInstance converges one HostedAgentInstance and periodically
// observes it so out-of-band backend changes are reflected in Obot.
func (h *Handler) OrchestrateInstance(req router.Request, resp router.Response) error {
	instance := req.Object.(*v1.HostedAgentInstance)
	if h.backend == nil {
		return fmt.Errorf("hosted agent backend is not configured")
	}

	ref := instanceRef(instance)
	// EnsurePool is registered immediately before this handler. nah can
	// aggregate errors from handlers in one reconciliation pass, so do not try
	// to construct backend desired state when pool resolution failed.
	// The pool handler's actionable error is sufficient and a later
	// reconcile will continue once defaults or an assignment are configured.
	if instance.Spec.PoolID == "" {
		return nil
	}

	var agent v1.HostedAgent
	if err := req.Get(&agent, req.Namespace, instance.Spec.HostedAgentName); err != nil {
		return err
	}

	var harness v1.Harness
	if err := req.Get(&harness, req.Namespace, agent.Spec.Manifest.HarnessID); err != nil {
		return err
	}

	// The credential is resolved before the config rather than attached after
	// it, because the config embeds it: every MCP header and model key in there
	// is this credential, so it has to exist before the config is rendered.
	credential, credentialVersion, err := h.instanceCredential(req.Ctx, instance)
	if err != nil {
		return err
	}

	desired, err := h.builder.Build(req.Ctx, BuildInput{
		Client:     req.Client,
		Namespace:  req.Namespace,
		Instance:   instance,
		Agent:      &agent,
		Harness:    &harness,
		Credential: credential,
	})
	if err != nil {
		return err
	}
	desired.Ref = ref
	desired.Pool.ID = instance.Spec.PoolID
	attachCredential(&desired, instance.Name, credential, credentialVersion)

	// A sandbox has no size of its own: it takes an equal share of its pool.
	// Carrying the result in desired state means resizing a pool changes every
	// sandbox's revision and restarts it, which is the only way a running
	// sandbox picks up new limits.
	var pool v1.HostedAgentPool
	if err := req.Get(&pool, req.Namespace, instance.Spec.PoolID); err != nil {
		return err
	}
	desired.Requests, desired.Limits, _ = agentbackend.SandboxShare(agentbackend.ResourceQuantity{
		CPUVCPUs:     pool.Spec.Manifest.Capacity.CPUVCPUs,
		MemoryBytes:  pool.Spec.Manifest.Capacity.MemoryBytes,
		StorageBytes: pool.Spec.Manifest.Capacity.StorageBytes,
	}, pool.Spec.Manifest.MaxSandboxes)

	var observation agentbackend.InstanceObservation
	if instance.Status.ObservedRevision == desired.Revision {
		observation, err = h.backend.ObserveInstance(req.Ctx, ref)
	} else {
		observation, err = h.backend.ReconcileInstance(req.Ctx, desired)
	}
	if err != nil {
		return err
	}
	previous := instance.Status
	h.applyObservation(instance, desired.Revision, observation)
	interval := pollInterval(instance.Status.State)
	resp.RetryAfter(interval)

	// A status write emits a change event, which reconciles this instance again
	// straight away. Writing unconditionally therefore means the controller
	// triggers its own next pass and RetryAfter never applies, spinning as fast
	// as storage allows. Only write when something actually moved, refreshing
	// the heartbeat at most once per poll interval so the timestamp stays
	// meaningful without driving the loop.
	if statusUnchanged(previous, instance.Status) && !heartbeatDue(previous.LastObservedTime, h.now(), interval) {
		instance.Status.LastObservedTime = previous.LastObservedTime
		return nil
	}
	return req.Client.Status().Update(req.Ctx, instance)
}

// statusUnchanged compares everything an observation can set except the
// heartbeat, which moves on every pass and so cannot be part of the decision to
// write.
func statusUnchanged(a, b v1.HostedAgentInstanceStatus) bool {
	return a.State == b.State &&
		a.URL == b.URL &&
		a.Error == b.Error &&
		a.Reason == b.Reason &&
		a.Message == b.Message &&
		a.ObservedRevision == b.ObservedRevision &&
		a.BackendID == b.BackendID &&
		a.BackendGeneration == b.BackendGeneration
}

func heartbeatDue(last *metav1.Time, now time.Time, interval time.Duration) bool {
	return last == nil || !now.Before(last.Add(interval))
}

// RemoveInstance asks the backend to remove an instance. The router retains
// the finalizer while this handler requests a retry.
func (h *Handler) RemoveInstance(req router.Request, resp router.Response) error {
	instance := req.Object.(*v1.HostedAgentInstance)
	if h.backend == nil {
		return fmt.Errorf("hosted agent backend is not configured")
	}
	ref := instanceRef(instance)
	result, err := h.backend.DeleteInstance(req.Ctx, ref)
	if err != nil {
		return err
	}
	if !result.Complete {
		resp.RetryAfter(transitionalPollInterval)
		return nil
	}

	// Revoke last. The credential outliving a failed teardown is recoverable;
	// revoking before the sandbox is gone would leave a live agent holding a
	// credential that no longer authenticates, which looks like a broken agent
	// rather than a deletion in progress.
	if h.credentials != nil {
		if err := h.credentials.RevokeInstanceCredential(req.Ctx, instance.Name); err != nil {
			return fmt.Errorf("revoke hosted agent credential: %w", err)
		}
	}

	return nil
}

// instanceCredential returns the sandbox's own credential and an opaque version
// that changes when it is rotated.
func (h *Handler) instanceCredential(ctx context.Context, instance *v1.HostedAgentInstance) (string, string, error) {
	if h.credentials == nil {
		return "", "", nil
	}
	value, version, err := h.credentials.EnsureInstanceCredential(ctx, instance.Name, instance.Spec.UserID)
	if err != nil {
		return "", "", fmt.Errorf("issue hosted agent credential: %w", err)
	}
	return value, version, nil
}

// attachCredential also places the raw credential in the sandbox.
//
// The config already carries it inside each endpoint, but an agent that needs
// to call Obot for something the config does not describe would otherwise have
// no token at all. It is a file rather than an environment variable: agents run
// model-directed commands, and every subprocess inherits the environment.
func attachCredential(desired *agentbackend.DesiredInstance, instanceName, value, version string) {
	if value == "" {
		return
	}
	desired.Secrets = append(desired.Secrets, agentbackend.SecretRef{
		ID:       instanceName + "-credential",
		Version:  version,
		Value:    value,
		FilePath: credentialFilePath,
	})
}

func (h *Handler) applyObservation(instance *v1.HostedAgentInstance, desiredRevision string, observation agentbackend.InstanceObservation) {
	instance.Status.ObservedRevision = observation.ObservedRevision
	instance.Status.Reason = observation.Reason
	instance.Status.Message = observation.Message
	instance.Status.Error = ""
	instance.Status.BackendGeneration = observation.BackendGeneration
	now := metav1.NewTime(h.now())
	instance.Status.LastObservedTime = &now

	if observation.Ref.BackendID != "" {
		instance.Status.BackendID = observation.Ref.BackendID
	}
	// The URL is intentionally sticky after the backend first advertises it.
	if observation.URL != "" {
		instance.Status.URL = observation.URL
	}

	switch {
	case observation.ObservedRevision != desiredRevision:
		instance.Status.State = types.HostedAgentStatePending
	case observation.State == agentbackend.StateError:
		instance.Status.State = types.HostedAgentStateError
		instance.Status.Error = observation.Message
	case observation.Exists &&
		observation.State == agentbackend.StateReady &&
		observation.ObservedRevision == desiredRevision:
		instance.Status.State = types.HostedAgentStateReady
	default:
		instance.Status.State = types.HostedAgentStatePending
	}
}

func instanceRef(instance *v1.HostedAgentInstance) agentbackend.InstanceRef {
	id := string(instance.UID)
	if id == "" {
		// Objects normally have a UID before a controller sees them. The name is
		// a deterministic fallback for storage implementations that assign it
		// after the initial controller pass.
		id = instance.Name
	}
	return agentbackend.InstanceRef{
		ID:        id,
		Namespace: instance.Namespace,
		UserID:    instance.Spec.UserID,
		BackendID: instance.Status.BackendID,
	}
}

func pollInterval(state types.HostedAgentState) time.Duration {
	if state == types.HostedAgentStateReady {
		return readyPollInterval
	}
	return transitionalPollInterval
}

// defaultDesiredBuilder renders the sandbox's runtime contract.
//
// It holds the server URL because every endpoint it writes into the config is
// absolute: the sandbox is told where to connect, never how to work it out.
//
// Two addresses, because the sandbox and the browser do not reach Obot the same
// way. Writing one into both roles breaks whichever it is not: the public
// address is commonly unroutable from inside the cluster, and the internal one
// is meaningless to a browser.
type defaultDesiredBuilder struct {
	// ServerURL is Obot's public address, and so the only one a published
	// agent can build links from.
	ServerURL string
	// InternalURL is Obot's address as a sandbox reaches it -- its models, its
	// MCP servers, its own API. Empty means the two are the same.
	InternalURL string
	Skills      *skillFetcher
}

// internal is the address the sandbox calls back on, falling back to the public
// one so a caller that knows of only a single address still builds a usable
// config.
func (b defaultDesiredBuilder) internal() string {
	if b.InternalURL != "" {
		return b.InternalURL
	}
	return b.ServerURL
}

// contentVersion identifies a config by its content, so that any change to it
// -- a new MCP server, a rotated credential, an edited skill -- changes the
// desired revision and restarts the sandbox, without the content itself ever
// entering the revision.
func contentVersion(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (b defaultDesiredBuilder) Build(ctx context.Context, in BuildInput) (agentbackend.DesiredInstance, error) {
	instance, agent, harness := in.Instance, in.Agent, in.Harness
	manifest := agent.Spec.Manifest
	instanceManifest := instance.Spec.Manifest

	answers := make(map[string]string, len(instanceManifest.Answers))
	for key, value := range instanceManifest.Answers {
		if !sensitiveQuestion(manifest.Questions, key) {
			answers[key] = value
		}
	}

	source := agentSource(manifest, instanceManifest)

	// A template names MCP servers and skills portably, by their source and key,
	// because the IDs an installation generates differ. Resolution happens here
	// rather than being rewritten into the template at sync, so a reference that
	// names nothing today resolves on its own once it is installed.
	//
	// Anything unresolved is dropped rather than failing the sandbox: the API
	// reports the agent unavailable for the same reason, and a sandbox that
	// starts with what it does have beats one that will not start at all.
	mcpIDs := append(slices.Clone(manifest.MCPServers), instanceManifest.MCPServers...)
	skillIDs := append(slices.Clone(manifest.Skills), instanceManifest.Skills...)
	if in.Client != nil {
		resolver := hostedagentrefs.New(in.Client, in.Namespace)
		var unresolvedMCP, unresolvedSkills []string
		mcpIDs, unresolvedMCP = resolver.MCPServers(ctx, mcpIDs)
		skillIDs, unresolvedSkills = resolver.Skills(ctx, skillIDs)
		for _, ref := range append(unresolvedMCP, unresolvedSkills...) {
			log.Warnf("hosted agent %s: %q names nothing installed here; leaving it out", instance.Name, ref)
		}
	}

	// Only an agent that serves HTTP is published, and only it needs to know
	// where. The path uses the instance's resource ID because that is what the
	// route is keyed on, not the backend identifier the config reports.
	var publicPath, publicURL string
	if manifest.Port > 0 {
		publicPath = "/agent-connect/" + instance.Name
		publicURL = strings.TrimSuffix(b.ServerURL, "/") + publicPath
	}

	config := agentConfig{
		Version: "v1",
		Instance: agentConfigMeta{
			ID:     instanceRef(instance).ID,
			UserID: instance.Spec.UserID,
			Name:   instanceManifest.Name,
		},
		SecretsFile: agentSecretsPath,
		Workspace:   workspacePath,
		ListenPort:  manifest.Port,
		PublicPath:  publicPath,
		PublicURL:   publicURL,
		ObotURL:     b.internal(),
		MCPServers:  mcpServerConfigs(b.internal(), mcpIDs),
		Answers:     answers,
	}
	if source.URL != "" {
		config.Source = &agentConfigSource{URL: source.URL, Ref: source.Revision, Subdir: source.Subdir}
	}

	if in.Client != nil {
		models, err := modelConfigs(ctx, in.Client, in.Namespace, b.internal(),
			append(slices.Clone(manifest.Models), instanceManifest.Models...),
			manifest.ModelProviders)
		if err != nil {
			return agentbackend.DesiredInstance{}, err
		}
		config.Models = models
	}

	// Skill files are placed in the sandbox rather than described to it: an
	// agent should find its skills on disk, not go and fetch them.
	var skillFiles []agentbackend.File
	if in.Client != nil && b.Skills != nil {
		skillFiles, config.Skills = b.Skills.skillFiles(ctx, in.Client, in.Namespace, skillIDs)
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return agentbackend.DesiredInstance{}, fmt.Errorf("render agent config: %w", err)
	}

	secretsBytes, err := json.Marshal(buildSecrets(config, in.Credential))
	if err != nil {
		return agentbackend.DesiredInstance{}, fmt.Errorf("render agent secrets: %w", err)
	}

	env := make(map[string]string)
	for _, value := range manifest.Env {
		if !value.Sensitive {
			env[value.Key] = value.Value
		}
	}

	desired := agentbackend.DesiredInstance{
		Ref:         instanceRef(instance),
		Pool:        agentbackend.PoolRef{ID: instance.Spec.PoolID},
		Name:        instanceManifest.Name,
		Description: instanceManifest.Description,
		Harness: agentbackend.Harness{
			ID:          manifest.HarnessID,
			Interactive: harness.Spec.Manifest.Interactive,
		},
		Image:    harness.Spec.Manifest.Image,
		Source:   source,
		Port:     manifest.Port,
		Terminal: manifest.Terminal,
		Env:      env,
		// The configuration itself carries no credential, so it is an ordinary
		// world-readable file: it can be inspected, logged or copied without
		// leaking anything.
		Files: append(skillFiles, agentbackend.File{
			Path:    agentConfigPath,
			Content: configBytes,
			Mode:    0o444,
		}),
		// The credentials travel separately as a secret, which keeps them out
		// of the desired revision. The version is the content hash, so a
		// rotated credential still restarts the sandbox.
		Secrets: []agentbackend.SecretRef{{
			ID:       instance.Name + "-secrets",
			Version:  contentVersion(secretsBytes),
			Value:    string(secretsBytes),
			FilePath: agentSecretsPath,
		}},
	}

	revision, err := desiredRevision(desired)
	if err != nil {
		return agentbackend.DesiredInstance{}, err
	}
	desired.Revision = revision
	return desired, nil
}

func desiredRevision(desired agentbackend.DesiredInstance) (string, error) {
	// Secret values must never reach the revision. Their version markers still
	// do, so a rotation changes the revision and restarts the sandbox without
	// the revision ever containing a credential.
	desired = desired.Redacted()
	// Backend IDs are observations and must not perturb desired configuration.
	desired.Ref.BackendID = ""
	desired.Revision = ""
	content, err := json.Marshal(desired)
	if err != nil {
		return "", fmt.Errorf("calculate hosted agent revision: %w", err)
	}
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func sensitiveQuestion(questions []types.HostedAgentQuestion, key string) bool {
	for _, question := range questions {
		if question.Key == key {
			return question.Sensitive
		}
	}
	return false
}

// agentSource picks the repository the sandbox clones.
//
// The ref travels with its own repository rather than being resolved
// separately: a user who supplies their own repository picks the ref for that
// repository, and the agent's ref -- which names a revision of a different
// repository -- must not be applied to it.
func agentSource(agent types.HostedAgentManifest, instance types.HostedAgentInstanceManifest) agentbackend.Source {
	if instance.GitRepo != "" {
		return agentbackend.Source{URL: instance.GitRepo, Revision: instance.GitRef}
	}
	return agentbackend.Source{URL: agent.GitRepo, Revision: agent.GitRef}
}
