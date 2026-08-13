// Package kubernetes implements the agent runtime backend on top of a
// Kubernetes cluster.
//
// The mapping is:
//
//	HostedAgentPool -> a PriorityClass, a ResourceQuota scoped to it, and
//	                         one shared PersistentVolumeClaim
//	HostedAgentInstance   -> a Deployment, a Service, and a Secret, with the
//	                         pool volume mounted at a per-instance subPath
//
// ResourceQuota cannot select on labels, and PriorityClass is the only per-pod
// attribute its scopeSelector understands. Tagging every sandbox with its
// pool's PriorityClass is therefore what lets several pools share one
// namespace. The PriorityClass carries no scheduling intent: every pool uses
// the same value and never preempts.
//
// Sandboxes are Burstable. The quota's requests budget is the pool's
// capacity, and its limits budget is that capacity multiplied by the overcommit
// ratio. Co-location happens implicitly: the pool volume binds to whichever
// node the first sandbox lands on, and the scheduler then confines the rest of
// the pool to that node.
package kubernetes

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/pkg/agentbackend"
	"github.com/obot-platform/obot/pkg/hash"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// revisionAnnotation carries the Obot-supplied desired revision. It is
	// stored verbatim and reported back as the observed revision.
	revisionAnnotation = "obot.ai/agent-revision"
	// schedulingRevisionAnnotation records the deployment-wide scheduling
	// defaults used to build a pod template. Unlike the agent revision, these
	// settings can change without changing desired instance state.
	schedulingRevisionAnnotation = "obot.ai/hosted-agent-scheduling-revision"

	instanceLabel = "obot.ai/hosted-agent-instance"
	poolLabel     = "obot.ai/hosted-agent-pool"
	userLabel     = "obot.ai/hosted-agent-user"
	managedLabel  = "obot.ai/managed-by"
	managedValue  = "obot-agent-backend"

	// agentContainerName is the sandbox's only container. The terminal attaches
	// to it by name rather than by position.
	agentContainerName = "agent"

	workspaceMountPath  = "/workspace"
	workspaceVolumeName = "workspace"
	filesVolumePrefix   = "files"

	// poolPriorityValue is shared by every pool. Pools are accounting
	// boundaries, not scheduling tiers, so they must not preempt one another.
	poolPriorityValue = 1000

	defaultContainerPort = 8099
	defaultFSGroup       = 1000
)

// Options configures the backend. Everything here is deployment-wide; nothing
// is per-agent or per-user.
type Options struct {
	// Namespace holds every sandbox and pool object. One namespace serves all
	// pools; they are separated by PriorityClass, not by namespace.
	Namespace string
	// ClusterDomain is used to build in-cluster service URLs.
	ClusterDomain string
	// StorageClassName provisions pool volumes. It should use
	// volumeBindingMode: WaitForFirstConsumer, which is what confines a pool to
	// a single node.
	StorageClassName string
	// RuntimeClassName is the name of the runtime class to use for the sandbox pods.
	RuntimeClassName string
	// Affinity, Tolerations, and NodeSelector constrain every pod created by
	// the backend, including cleanup jobs that must reach the pool volume.
	Affinity     *corev1.Affinity
	Tolerations  []corev1.Toleration
	NodeSelector map[string]string
	// FSGroup owns the per-instance subdirectory on the shared volume.
	FSGroup int64
	// PodSecurityLevel must match the Pod Security Admission level enforced on
	// Namespace. A sandbox that does not satisfy it is refused at admission,
	// which surfaces as a Deployment whose pod never appears. Empty means
	// restricted, which is what the Helm chart labels the namespace with.
	PodSecurityLevel PodSecurityLevel
	// ImagePullSecrets are attached to every sandbox pod.
	ImagePullSecrets []string
	// CleanupImage runs the per-instance volume cleanup job. It needs a shell
	// and coreutils; any small base image will do.
	CleanupImage string
	// ImagePullPolicy controls whether a sandbox image is re-pulled on every
	// start. Always is right for a mutable tag such as :latest, but refuses an
	// image that was preloaded into the node rather than published to a
	// registry -- which is how an air-gapped install, and a developer testing a
	// locally built harness, supply one. Empty means Always.
	ImagePullPolicy string
	// RESTConfig enables live usage reporting through metrics.k8s.io. Without
	// it, utilization reports committed requests instead of measured usage.
	RESTConfig *rest.Config
}

type Backend struct {
	client       kclient.Client
	cachedClient kclient.Client
	opts         Options
	// usage measures live consumption. It is nil when no metrics source is
	// configured, in which case utilization falls back to committed requests.
	usage usageReader

	// attachClient and devTransport are derived from RESTConfig, which does not
	// change for the life of the backend. Building them per call meant a new
	// rate limiter for every terminal attach and a transport lookup on every
	// proxied development request.
	//
	// Both are nil when there is no Kubernetes connection, or when one could not
	// be built from it; the features that need them report that themselves
	// rather than the backend refusing to start without them.
	attachClient rest.Interface
	devTransport http.RoundTripper

	namespaceOnce sync.Once
	namespaceRef  *metav1.OwnerReference
	namespaceErr  error
}

var (
	_ agentbackend.Backend = (*Backend)(nil)
)

func New(client, cachedClient kclient.Client, opts Options) (*Backend, error) {
	if opts.Namespace == "" {
		return nil, fmt.Errorf("agent backend namespace is required")
	}
	if opts.ClusterDomain == "" {
		opts.ClusterDomain = "cluster.local"
	}
	if opts.FSGroup == 0 {
		opts.FSGroup = defaultFSGroup
	}
	if opts.CleanupImage == "" {
		opts.CleanupImage = "busybox:1.36"
	}
	opts.PodSecurityLevel = ParsePodSecurityLevel(string(opts.PodSecurityLevel))
	if cachedClient == nil {
		cachedClient = client
	}
	// A missing or unusable metrics source is not fatal: utilization degrades
	// to committed requests rather than the backend failing to start.
	usage, err := newMetricsReader(opts.RESTConfig)
	if err != nil {
		return nil, err
	}

	// Neither of these is required for the backend to run sandboxes, so a
	// failure to build one disables the feature that uses it rather than
	// stopping start-up -- the same treatment the metrics source gets above.
	var (
		attachClient rest.Interface
		devTransport http.RoundTripper
	)
	if opts.RESTConfig != nil {
		if attachClient, err = rest.RESTClientFor(attachConfig(opts.RESTConfig)); err != nil {
			attachClient = nil
		}
		if devTransport, err = rest.TransportFor(opts.RESTConfig); err != nil {
			devTransport = nil
		}
	}

	return &Backend{
		client:       client,
		cachedClient: cachedClient,
		opts:         opts,
		usage:        usage,
		attachClient: attachClient,
		devTransport: devTransport,
	}, nil
}

func (b *Backend) podSchedulingRevision() string {
	if b.opts.Affinity == nil && len(b.opts.Tolerations) == 0 && len(b.opts.NodeSelector) == 0 {
		return ""
	}

	return hash.String(struct {
		Affinity     *corev1.Affinity    `json:"affinity,omitempty"`
		Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
		NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	}{
		Affinity:     b.opts.Affinity,
		Tolerations:  b.opts.Tolerations,
		NodeSelector: b.opts.NodeSelector,
	})
}

// poolName is the shared name of a pool's PriorityClass, ResourceQuota,
// and volume. PriorityClasses are cluster-scoped, so this has to be unique
// across the cluster, not just the namespace.
func poolName(poolID string) string {
	return name.SafeConcatName("obot-pool", sanitize(poolID))
}

func instanceName(instanceID string) string {
	return name.SafeConcatName("obot-agent", sanitize(instanceID))
}

func cleanupJobName(instanceID string) string {
	return name.SafeConcatName("obot-agent-cleanup", sanitize(instanceID))
}

// sandboxSubdirPattern is the only shape a sandbox's directory on the pool
// volume may take: a single lowercase DNS label. It admits no dot, no slash and
// no empty string.
var sandboxSubdirPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// sandboxSubdir is the per-instance directory within the shared pool volume.
// It is the only supported way to derive that name, and every path built from
// an instance ID must come through here rather than calling sanitize directly.
//
// sanitize alone is not enough, because it can return an empty string: an ID of
// "", "..", "---" or anything else without an alphanumeric reduces to nothing.
// Both uses of this value make that catastrophic rather than merely wrong. As a
// mount subPath, empty mounts the pool root into the sandbox, handing one agent
// every other agent's workspace. As the argument to the cleanup job's rm, empty
// makes the target "/pool" itself, so deleting one sandbox would erase every
// sandbox in the pool.
//
// A rejected ID therefore fails the operation loudly. That leaves an
// undeletable instance for an operator to look at, which is the outcome we
// want when the alternative is destroying a shared volume.
func sandboxSubdir(instanceID string) (string, error) {
	subdir := sanitize(instanceID)
	if !sandboxSubdirPattern.MatchString(subdir) {
		return "", fmt.Errorf("instance %q does not reduce to a usable pool directory name (got %q): refusing to touch the pool volume", instanceID, subdir)
	}
	return subdir, nil
}

// sanitize reduces an Obot identity to something usable as a DNS label. Obot
// IDs are already UID- or name-shaped, so this normally only lowercases.
//
// It can return an empty string. Anything using the result as a path must go
// through sandboxSubdir instead.
func sanitize(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// namespaceOwner returns an owner reference to the sandbox namespace. A
// PriorityClass is cluster-scoped and so may only be owned by another
// cluster-scoped object -- which a Namespace is. This is a backstop for someone
// deleting the whole namespace; ordinary pool teardown goes through
// DeletePool.
func (b *Backend) namespaceOwner(ctx context.Context) (*metav1.OwnerReference, error) {
	b.namespaceOnce.Do(func() {
		var namespace corev1.Namespace
		err := b.client.Get(ctx, kclient.ObjectKey{Name: b.opts.Namespace}, &namespace)
		if apierrors.IsNotFound(err) {
			// The chart creates this namespace, and hosted agents cannot run on
			// Kubernetes without MCP doing so too, so normally it is already
			// here. Creating it is for an install that does not use the chart,
			// and mirrors what pkg/mcp does for the same namespace rather than
			// leaving hosted agents the only feature that cannot start without
			// one being made for it.
			created := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: b.opts.Namespace}}
			if createErr := b.client.Create(ctx, created); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
				b.namespaceErr = fmt.Errorf("failed to create namespace %s: %w", b.opts.Namespace, createErr)
				return
			}
			err = b.client.Get(ctx, kclient.ObjectKey{Name: b.opts.Namespace}, &namespace)
		}
		if err != nil {
			b.namespaceErr = fmt.Errorf("failed to read namespace %s: %w", b.opts.Namespace, err)
			return
		}
		b.namespaceRef = &metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "Namespace",
			Name:       namespace.Name,
			UID:        namespace.UID,
		}
	})
	if b.namespaceErr != nil {
		// Don't cache a transient failure for the life of the process.
		b.namespaceOnce = sync.Once{}
		return nil, b.namespaceErr
	}
	return b.namespaceRef, nil
}

func cpuQuantity(vcpus float64) resource.Quantity {
	return *resource.NewMilliQuantity(int64(vcpus*1000), resource.DecimalSI)
}

func memoryQuantity(bytes int64) resource.Quantity {
	return *resource.NewQuantity(bytes, resource.BinarySI)
}

func poolLabels(poolID string) map[string]string {
	return map[string]string{
		managedLabel: managedValue,
		poolLabel:    sanitize(poolID),
	}
}
