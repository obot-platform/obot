package catalogversion

import (
	"context"
	"fmt"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Resolved struct {
	Entry   v1.MCPServerCatalogEntry
	Version v1.MCPServerCatalogEntryVersion
}

func ResolveDefault(ctx context.Context, reader client.Reader, namespace, entryName string) (Resolved, error) {
	var entry v1.MCPServerCatalogEntry
	if err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: entryName}, &entry); err != nil {
		return Resolved{}, err
	}
	return resolve(ctx, reader, entry, entry.Spec.DefaultVersion, false, true)
}

func ResolveExact(ctx context.Context, reader client.Reader, namespace, entryName string, version int, requireActive bool) (Resolved, error) {
	var entry v1.MCPServerCatalogEntry
	if err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: entryName}, &entry); err != nil {
		return Resolved{}, err
	}
	return resolve(ctx, reader, entry, version, requireActive, version == 0)
}

func resolve(ctx context.Context, reader client.Reader, entry v1.MCPServerCatalogEntry, version int, requireActive, allowCompatibilityFallback bool) (Resolved, error) {
	var child v1.MCPServerCatalogEntryVersion
	err := reader.Get(ctx, client.ObjectKey{
		Namespace: entry.Namespace,
		Name:      v1.MCPServerCatalogEntryVersionName(entry.Name, version),
	}, &child)
	if apierrors.IsNotFound(err) && allowCompatibilityFallback {
		child = v1.MCPServerCatalogEntryVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1.MCPServerCatalogEntryVersionName(entry.Name, version),
				Namespace: entry.Namespace,
			},
			Spec: v1.MCPServerCatalogEntryVersionSpec{
				MCPServerCatalogEntryName: entry.Name,
				Version:                   version,
				Manifest:                  entry.Spec.Manifest,
				UnsupportedTools:          entry.Spec.UnsupportedTools,
				SourceURL:                 entry.Spec.SourceURL,
				Active:                    true,
			},
		}
	} else if err != nil {
		return Resolved{}, fmt.Errorf("failed to resolve version %d for MCP catalog entry %s: %w", version, entry.Name, err)
	}
	if child.Spec.MCPServerCatalogEntryName != entry.Name || child.Spec.Version != version {
		return Resolved{}, fmt.Errorf("MCP catalog entry version %s does not match entry %s version %d", child.Name, entry.Name, version)
	}
	if requireActive && !child.Spec.Active {
		return Resolved{}, fmt.Errorf("version %d for MCP catalog entry %s is not active", version, entry.Name)
	}
	return Resolved{Entry: entry, Version: child}, nil
}
