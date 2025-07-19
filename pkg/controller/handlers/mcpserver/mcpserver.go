package mcpserver

import (
	"net/url"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
)

func DetectDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.MCPServerCatalogEntryName == "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
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

	if server.Status.NeedsUpdate != drifted {
		server.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

func configurationHasDrifted(serverManifest types.MCPServerManifest, entryManifest types.MCPServerCatalogEntryManifest) (bool, error) {
	// First, check on the URL or hostname.
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

	// Check the rest of the fields to see if anything has changed.
	drifted := serverManifest.Command != entryManifest.Command ||
		!slices.Equal(serverManifest.Args, entryManifest.Args) ||
		!utils.SlicesEqualIgnoreOrder(serverManifest.Env, entryManifest.Env) ||
		!utils.SlicesEqualIgnoreOrder(serverManifest.Headers, entryManifest.Headers)

	return drifted, nil
}
