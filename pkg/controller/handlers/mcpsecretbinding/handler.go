// Package mcpsecretbinding watches Kubernetes Secrets in the obot
// namespace and triggers MCPServer reconciles when a referenced Secret
// changes (created / updated / deleted). Together with the read-time
// resolution in mcp.MergeBoundCreds this delivers rotation-aware
// secretBinding handling: a single Secret edit fans out to every
// MCPServer that references it, the deploy reconcile re-runs with the
// latest values, secretEnvData digests change, and the obot-revision
// annotation flips so the Deployment rolls.
package mcpsecretbinding

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

// rotationAnnotation is the spec-level sentinel we bump on referencing
// MCPServers when a watched Secret changes. Bumping any spec annotation
// makes the storage backend emit a watch event, which re-triggers the
// existing MCPServer reconcile pipeline. The deploy path then calls
// mcp.MergeBoundCreds again, picks up the latest secret content, and
// rolls if the resulting secretEnvData digest changed.
const rotationAnnotation = "obot.ai/secret-binding-rotation"

// Handler is the Secret-watch fan-in for secretBinding rotation.
//
// It is registered on the localK8sRouter against &corev1.Secret{} so
// req.Client is a kclient for the local Kubernetes cluster (not used
// here — we don't read or write Secrets from this handler). All
// MCPServer reads/writes go through h.storage, which is the storage
// backend (CRDs).
//
// Mirrors the deployment.UpdateMCPServerStatusFromPod pattern: a
// secondary-resource watch that fans into MCPServer reconciles by
// touching them in storage.
type Handler struct {
	storage           kclient.Client
	obotNamespace     string
	mcpSessionManager *mcp.SessionManager
}

// New constructs the handler. obotNamespace is the Kubernetes namespace
// where source Secrets for secretBindings live (the obot pod's release
// namespace). Empty obotNamespace disables the watch — events for
// other namespaces are no-ops anyway, but with no obot namespace we
// can't distinguish them. storage must be non-nil.
func New(storage kclient.Client, obotNamespace string, mcpSessionManager *mcp.SessionManager) *Handler {
	return &Handler{
		storage:           storage,
		obotNamespace:     obotNamespace,
		mcpSessionManager: mcpSessionManager,
	}
}

// SecretChanged is the canonical handler. Attached to localK8sRouter
// via &corev1.Secret{} with IncludeRemoved() — delete events MUST
// reach this handler so we can flip referencing servers' missing
// state.
func (h *Handler) SecretChanged(req router.Request, _ router.Response) error {
	if h.obotNamespace == "" {
		return nil
	}

	// req.Object can be nil on delete depending on router config;
	// req.Namespace + req.Name still identify the affected Secret.
	namespace := req.Namespace
	name := req.Name
	if secret, ok := req.Object.(*corev1.Secret); ok && secret != nil {
		namespace = secret.Namespace
		name = secret.Name
	}
	if namespace != h.obotNamespace || name == "" {
		return nil
	}

	var servers v1.MCPServerList
	if err := h.storage.List(req.Ctx, &servers, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return fmt.Errorf("list mcpservers: %w", err)
	}

	for i := range servers.Items {
		s := &servers.Items[i]
		if !mcp.ManifestReferencesSecret(s.Spec.Manifest, name) {
			continue
		}

		// Compute a hash of only the Secret keys actually referenced by
		// this MCPServer's bindings. Using a content hash (rather than
		// ResourceVersion) means that changes to other keys in the same
		// Secret — or metadata-only updates — do not trigger a spurious
		// redeploy. The annotation value is empty string when the Secret
		// is deleted, which will always differ from any non-empty hash.
		secret, _ := req.Object.(*corev1.Secret)
		referencedHash := mcp.HashReferencedKeys(s.Spec.Manifest, name, secret)
		if s.Annotations[rotationAnnotation] == referencedHash {
			continue
		}
		if s.Annotations == nil {
			s.Annotations = map[string]string{}
		}
		s.Annotations[rotationAnnotation] = referencedHash
		if err := h.storage.Update(req.Ctx, s); err != nil {
			return fmt.Errorf("trigger reconcile on mcpserver %s: %w", s.Name, err)
		}

		// Shut down the running deployment so it stops serving the stale
		// secret value immediately. The next user connection will trigger a
		// fresh deploy that picks up the updated Secret — matching the
		// behaviour of the configure UI flow. Errors are non-fatal: the
		// annotation bump above already ensures fresh values on the next deploy.
		//
		// Exception: skip shutdown when every binding referencing this Secret
		// is a dynamic file binding (file=true, dynamic=true). Those update
		// in-place via kubelet Secret volume propagation — the pod keeps
		// running and reads the new file content without restart.
		if h.mcpSessionManager != nil && !mcp.ManifestHasOnlyDynamicFileBindingsForSecret(s.Spec.Manifest, name) {
			if err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, s.Name); err != nil {
				log.Errorf("shutdown mcpserver %s after secret rotation: %v", s.Name, err)
			}
		}
	}

	return nil
}
