package mcpserverinstance

import (
	"maps"
	"net/url"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Migrate makes sure that all spec fields are set properly.
func Migrate(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	// Check to see if we need to update.
	// Pre-migration, if there is a catalog name, it points to a shared server, and we don't need to add any new information.
	if instance.Spec.MCPCatalogName != "" {
		return nil
	}

	var server v1.MCPServer
	if err := req.Client.Get(req.Ctx, client.ObjectKey{
		Namespace: instance.Namespace,
		Name:      instance.Spec.MCPServerName,
	}, &server); err != nil {
		return err
	}

	if server.Spec.MCPServerCatalogEntryName == "" {
		instance.Spec.MCPServerCatalogEntryName = server.Spec.MCPServerCatalogEntryName

		var entry v1.MCPServerCatalogEntry
		if err := req.Client.Get(req.Ctx, client.ObjectKey{
			Namespace: instance.Namespace,
			Name:      instance.Spec.MCPServerCatalogEntryName,
		}, &entry); err != nil {
			return err
		}

		instance.Spec.MCPCatalogName = entry.Spec.MCPCatalogName

		return req.Client.Update(req.Ctx, instance)
	}

	return nil
}

func UpdateStatus(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	if instance.Spec.MCPServerCatalogEntryName == "" {
		return nil
	}

	var server v1.MCPServer
	if err := req.Get(&server, instance.Namespace, instance.Spec.MCPServerName); err != nil {
		return err
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, instance.Namespace, instance.Spec.MCPServerCatalogEntryName); err != nil {
		return err
	}

	var (
		drifted bool
		err     error
	)
	if entry.Spec.CommandManifest.Name != "" {
		drifted, err = configurationHasDrifted(server.Spec.Manifest, entry.Spec.CommandManifest)
	} else {
		drifted, err = configurationHasDrifted(server.Spec.Manifest, entry.Spec.URLManifest)
	}
	if err != nil {
		return err
	}

	instance.Status.NeedsUpdate = drifted
	instance.Status.NeedsURL = entry.Spec.URLManifest.Hostname != "" && server.Spec.Manifest.URL == ""
	return req.Client.Status().Update(req.Ctx, instance)
}

func configurationHasDrifted(serverManifest types.MCPServerManifest, entryManifest types.MCPServerCatalogEntryManifest) (bool, error) {
	drifted := !maps.Equal(serverManifest.Metadata, entryManifest.Metadata) ||
		serverManifest.Name != entryManifest.Name ||
		serverManifest.Description != entryManifest.Description ||
		serverManifest.Icon != entryManifest.Icon ||
		!types.EqualMCPEnvs(serverManifest.Env, entryManifest.Env) ||
		serverManifest.Command != entryManifest.Command ||
		!slices.Equal(serverManifest.Args, entryManifest.Args) ||
		!types.EqualMCPHeaders(serverManifest.Headers, entryManifest.Headers)

	if drifted {
		return true, nil
	}

	// Now check on the URL.

	if entryManifest.FixedURL != "" && serverManifest.URL != entryManifest.FixedURL {
		return true, nil
	}

	if entryManifest.Hostname != "" {
		u, err := url.Parse(serverManifest.URL)
		if err != nil {
			// Shouldn't ever happen.
			return true, err
		}

		if u.Hostname() != entryManifest.Hostname {
			return true, nil
		}
	}

	return false, nil
}
