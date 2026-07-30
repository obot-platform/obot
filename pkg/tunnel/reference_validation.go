package tunnel

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateCatalogEntryTunnelReferences verifies every tunnel reference in a
// catalog manifest, including remote components embedded in a composite.
func ValidateCatalogEntryTunnelReferences(ctx context.Context, client kclient.Client, manifest types.MCPServerCatalogEntryManifest) error {
	switch manifest.Runtime {
	case types.RuntimeRemote:
		if manifest.RemoteConfig == nil || manifest.RemoteConfig.TunnelName == "" {
			return nil
		}
		if manifest.RemoteConfig.URLTemplate != "" {
			return fmt.Errorf("remoteConfig.tunnelName cannot be used with remoteConfig.urlTemplate")
		}

		target := manifest.RemoteConfig.FixedURL
		if target == "" {
			target = manifest.RemoteConfig.Hostname
		}
		if target == "" {
			return fmt.Errorf("remoteConfig.tunnelName requires a fixedURL or hostname")
		}
		return ValidateReference(ctx, client, manifest.RemoteConfig.TunnelName, target)
	case types.RuntimeComposite:
		if manifest.CompositeConfig == nil {
			return nil
		}
		for i, component := range manifest.CompositeConfig.ComponentServers {
			if err := ValidateCatalogEntryTunnelReferences(ctx, client, component.Manifest); err != nil {
				return fmt.Errorf("compositeConfig.componentServers[%d]: %w", i, err)
			}
		}
	}
	return nil
}

// ValidateServerTunnelReferences verifies every tunnel reference in a runtime
// server manifest, including remote components embedded in a composite.
func ValidateServerTunnelReferences(ctx context.Context, client kclient.Client, manifest types.MCPServerManifest) error {
	switch manifest.Runtime {
	case types.RuntimeRemote:
		if manifest.RemoteConfig == nil || manifest.RemoteConfig.TunnelName == "" {
			return nil
		}
		if manifest.RemoteConfig.IsTemplate || manifest.RemoteConfig.URLTemplate != "" {
			return fmt.Errorf("remoteConfig.tunnelName cannot be used with a URL template")
		}

		target := manifest.RemoteConfig.URL
		if target == "" {
			target = manifest.RemoteConfig.Hostname
		}
		if target == "" {
			return fmt.Errorf("remoteConfig.tunnelName requires a URL or hostname")
		}
		return ValidateReference(ctx, client, manifest.RemoteConfig.TunnelName, target)
	case types.RuntimeComposite:
		if manifest.CompositeConfig == nil {
			return nil
		}
		for i, component := range manifest.CompositeConfig.ComponentServers {
			if err := ValidateServerTunnelReferences(ctx, client, component.Manifest); err != nil {
				return fmt.Errorf("compositeConfig.componentServers[%d]: %w", i, err)
			}
		}
	}
	return nil
}

// ValidateReference checks that name identifies an MCPTunnel whose current
// AllowedURLs authorize target.
func ValidateReference(ctx context.Context, client kclient.Client, name, target string) error {
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("MCP tunnel name must not contain leading or trailing whitespace")
	}
	target = strings.TrimSpace(target)
	if err := types.ValidateTunnelName(name); err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("cannot validate MCP tunnel %q without storage", name)
	}

	var tunnel v1.MCPTunnel
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      name,
	}, &tunnel); err != nil {
		return fmt.Errorf("failed to get MCP tunnel %q: %w", name, err)
	}
	tunnelName := tunnel.Spec.Manifest.DisplayName
	if tunnelName == "" {
		tunnelName = name
	}
	if err := tunnel.Spec.Manifest.Validate(); err != nil {
		return fmt.Errorf("MCP tunnel %q is invalid: %w", tunnelName, err)
	}
	if !tunnel.Spec.Manifest.AllowsURL(target) {
		return fmt.Errorf("MCP tunnel %q does not allow target %q", tunnelName, target)
	}
	return nil
}
