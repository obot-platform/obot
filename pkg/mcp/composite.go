package mcp

import (
	"context"
	"fmt"
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

// ComponentUpstream is the resource a component reference points at.
type ComponentUpstream struct {
	// Manifest is zero when Missing is true.
	Manifest types.MCPServerCatalogEntryManifest
	Missing  bool
}

// runtimeIdentity excludes presentation and resource fields.
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

// CompositeNoHealthyComponentsError reports a composite with no deployed enabled children.
type CompositeNoHealthyComponentsError struct {
	Composite string
	Errors    map[string]string
}

// ResolveComponentUpstream reports dangling references as Missing.
func ResolveComponentUpstream(ctx context.Context, c kclient.Client, ref types.ComponentRef) (ComponentUpstream, error) {
	switch {
	case ref.MCPServerID != "":
		var server v1.MCPServer
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: ref.MCPServerID}, &server); err != nil {
			if apierrors.IsNotFound(err) {
				return ComponentUpstream{Missing: true}, nil
			}
			return ComponentUpstream{}, fmt.Errorf("failed to get multi-user server %s: %w", ref.MCPServerID, err)
		}

		return ComponentUpstream{Manifest: server.Spec.Manifest.ConvertToCatalogEntry()}, nil
	case ref.CatalogEntryID != "":
		var entry v1.MCPServerCatalogEntry
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: ref.CatalogEntryID}, &entry); err != nil {
			if apierrors.IsNotFound(err) {
				return ComponentUpstream{Missing: true}, nil
			}
			return ComponentUpstream{}, fmt.Errorf("failed to get component catalog entry %s: %w", ref.CatalogEntryID, err)
		}

		return ComponentUpstream{Manifest: entry.Spec.Manifest}, nil
	}

	return ComponentUpstream{Missing: true}, nil
}

// ComponentUpstreamResolver returns a client-backed component resolver.
func ComponentUpstreamResolver(c kclient.Client) ResolveComponentFunc {
	return func(ctx context.Context, ref types.ComponentRef) (ComponentUpstream, error) {
		return ResolveComponentUpstream(ctx, c, ref)
	}
}

// RuntimeIdentityDigest identifies the runtime configuration used to generate tool overrides.
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

// ComponentToolOverridesStale reports whether saved overrides match the current runtime identity.
func ComponentToolOverridesStale(component types.CatalogComponentServer, upstream ComponentUpstream) bool {
	if len(component.ToolOverrides) == 0 || component.SourceDigest == "" || upstream.Missing {
		return false
	}

	return component.SourceDigest != RuntimeIdentityDigest(upstream.Manifest)
}

// WaitForCompositeReady waits for the current generation and explicit sync request.
func WaitForCompositeReady(ctx context.Context, c kclient.WithWatch, compositeServer v1.MCPServer, timeout time.Duration) (v1.MCPServer, error) {
	latest, err := wait.For(
		ctx,
		c,
		&compositeServer,
		func(cs *v1.MCPServer) (bool, error) {
			return compositeReady(compositeServer, *cs), nil
		},
		wait.Option{
			Timeout: timeout,
		},
	)
	if err != nil {
		return compositeServer, fmt.Errorf("wait for composite server %s to be ready: %w", compositeServer.Name, err)
	}

	return *latest, nil
}

// compositeReady reports that the controller acted on this generation. It does not require every
// component to be materialized: composites are best effort.
func compositeReady(requested, observed v1.MCPServer) bool {
	syncRequest := requested.Annotations[v1.MCPServerCompositeSyncRequestedAtAnnotation]
	return observed.Status.ObservedCompositeGeneration >= requested.Generation &&
		(syncRequest == "" || observed.Status.ObservedCompositeSyncRequest == syncRequest)
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
