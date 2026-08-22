package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/obot-platform/obot/pkg/wait"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolvedComponent is the upstream one composite component reference points at.
type ResolvedComponent struct {
	// Manifest is the referenced catalog entry's manifest, or the referenced multi-user server's
	// manifest in catalog-entry form. Zero when Missing.
	Manifest types.MCPServerCatalogEntryManifest
	Missing  bool
}

// ResolveCompositeComponentRef resolves one composite component reference. A reference that does
// not resolve returns Missing rather than an error: a dangling reference is a reportable state,
// not a failure.
func ResolveCompositeComponentRef(ctx context.Context, c kclient.Client, catalogEntryID, mcpServerID string) (ResolvedComponent, error) {
	switch {
	case mcpServerID != "":
		var server v1.MCPServer
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: mcpServerID}, &server); err != nil {
			if apierrors.IsNotFound(err) {
				return ResolvedComponent{Missing: true}, nil
			}
			return ResolvedComponent{}, fmt.Errorf("failed to get multi-user server %s: %w", mcpServerID, err)
		}

		return ResolvedComponent{Manifest: server.Spec.Manifest.ConvertToCatalogEntry()}, nil
	case catalogEntryID != "":
		var entry v1.MCPServerCatalogEntry
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: catalogEntryID}, &entry); err != nil {
			if apierrors.IsNotFound(err) {
				return ResolvedComponent{Missing: true}, nil
			}
			return ResolvedComponent{}, fmt.Errorf("failed to get component catalog entry %s: %w", catalogEntryID, err)
		}

		return ResolvedComponent{Manifest: entry.Spec.Manifest}, nil
	}

	return ResolvedComponent{Missing: true}, nil
}

// ComponentResolver returns a ResolveComponentFunc backed by the given client, for validation
// options that need to check what a composite's components reference.
func ComponentResolver(c kclient.Client) ResolveComponentFunc {
	return func(ctx context.Context, ref types.ComponentRef) (ResolvedComponent, error) {
		return ResolveCompositeComponentRef(ctx, c, ref.CatalogEntryID, ref.MCPServerID)
	}
}

// runtimeIdentity is the subset of a manifest that can change which tools a server serves. Name,
// icon, description, tool preview, and resource limits are excluded, so editing one of those does
// not report a composite's tool overrides stale.
type runtimeIdentity struct {
	Runtime types.Runtime `json:"runtime"`

	Package string   `json:"package,omitempty"`
	Image   string   `json:"image,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Port    int      `json:"port,omitempty"`
	Path    string   `json:"path,omitempty"`

	FixedURL    string `json:"fixedURL,omitempty"`
	URLTemplate string `json:"urlTemplate,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	TunnelName  string `json:"tunnelName,omitempty"`

	EnvKeys    []string `json:"envKeys,omitempty"`
	HeaderKeys []string `json:"headerKeys,omitempty"`
}

// RuntimeIdentityDigest digests the populated runtime config block plus the keys, never the
// values, of env and remote headers. It is captured alongside a component's generated tool
// overrides and compared against the upstream's current digest to detect staleness.
func RuntimeIdentityDigest(manifest types.MCPServerCatalogEntryManifest) string {
	identity := runtimeIdentity{Runtime: manifest.Runtime}

	switch {
	case manifest.NPXConfig != nil:
		identity.Package = manifest.NPXConfig.Package
		identity.Args = manifest.NPXConfig.Args
	case manifest.UVXConfig != nil:
		identity.Package = manifest.UVXConfig.Package
		identity.Command = manifest.UVXConfig.Command
		identity.Args = manifest.UVXConfig.Args
	case manifest.ContainerizedConfig != nil:
		identity.Image = manifest.ContainerizedConfig.Image
		identity.Command = manifest.ContainerizedConfig.Command
		identity.Args = manifest.ContainerizedConfig.Args
		identity.Port = manifest.ContainerizedConfig.Port
		identity.Path = manifest.ContainerizedConfig.Path
	case manifest.RemoteConfig != nil:
		identity.FixedURL = manifest.RemoteConfig.FixedURL
		identity.URLTemplate = manifest.RemoteConfig.URLTemplate
		identity.Hostname = manifest.RemoteConfig.Hostname
		identity.TunnelName = manifest.RemoteConfig.TunnelName
		identity.HeaderKeys = headerKeys(manifest.RemoteConfig.Headers)
	}

	identity.EnvKeys = envKeys(manifest.Env)

	return utils.Digest(identity)
}

func envKeys(envs []types.MCPEnv) []string {
	if len(envs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(envs))
	for _, env := range envs {
		keys = append(keys, env.Key)
	}

	return keys
}

func headerKeys(headers []types.MCPHeader) []string {
	if len(headers) == 0 {
		return nil
	}

	keys := make([]string, 0, len(headers))
	for _, header := range headers {
		keys = append(keys, header.Key)
	}

	return keys
}

// ComponentToolOverridesStale reports whether a component's tool overrides were generated against
// a different upstream runtime identity than the upstream has now. A component with no overrides,
// or with no captured digest, is never stale.
func ComponentToolOverridesStale(component types.CatalogComponentServer, upstream ResolvedComponent) bool {
	if len(component.ToolOverrides) == 0 || component.SourceDigest == "" || upstream.Missing {
		return false
	}

	return component.SourceDigest != RuntimeIdentityDigest(upstream.Manifest)
}

// WaitForCompositeSettled waits for the composite controller to reconcile the composite server's
// current spec generation and reports whether it did. On timeout it returns the composite as it
// stands, so a caller that only reports state can carry on while the connect path can refuse.
// The comparison is >= so a stale cached generation cannot make the predicate unsatisfiable.
func WaitForCompositeSettled(ctx context.Context, c kclient.WithWatch, compositeServer v1.MCPServer, timeout time.Duration) (v1.MCPServer, bool) {
	latest, err := wait.For(
		ctx,
		c,
		&compositeServer,
		func(cs *v1.MCPServer) (bool, error) {
			return cs.Status.ObservedCompositeGeneration >= cs.Generation, nil
		},
		wait.Option{
			Timeout: timeout,
		},
	)
	if err != nil {
		slog.Debug("Composite server did not settle before the timeout, returning current state", "composite", compositeServer.Name, "generation", compositeServer.Generation, "observedCompositeGeneration", compositeServer.Status.ObservedCompositeGeneration)
		return compositeServer, false
	}

	return *latest, true
}

// CompositeNotSettledError is returned when a composite could not be reconciled in time.
type CompositeNotSettledError struct {
	Composite string
	Timeout   time.Duration
}

func (e CompositeNotSettledError) Error() string {
	return fmt.Sprintf("composite server %s was not ready after %s", e.Composite, e.Timeout)
}

// CompositeNoHealthyComponentsError is returned when a composite settled with none of its enabled
// components deployed. Errors holds the reason each one failed, keyed by component reference.
type CompositeNoHealthyComponentsError struct {
	Composite string
	Errors    map[string]string
}

func (e CompositeNoHealthyComponentsError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("composite server %s has no deployed components", e.Composite)
	}

	reasons := make([]string, 0, len(e.Errors))
	for _, component := range slices.Sorted(maps.Keys(e.Errors)) {
		reasons = append(reasons, fmt.Sprintf("%s: %s", component, e.Errors[component]))
	}

	return fmt.Sprintf("composite server %s has no deployed components: %s", e.Composite, strings.Join(reasons, "; "))
}

// EnabledComponentCount is how many of a composite's components it should have deployed.
func EnabledComponentCount(manifest types.MCPServerManifest) int {
	if manifest.CompositeConfig == nil {
		return 0
	}

	var enabled int
	for _, component := range manifest.CompositeConfig.ComponentServers {
		if !component.Disabled {
			enabled++
		}
	}

	return enabled
}
