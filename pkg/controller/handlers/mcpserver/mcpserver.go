package mcpserver

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/errors"
)

var log = logger.Package()

// DeleteOrphans deletes non-shared MCPServer that have no MCPServerInstances at least one hour after creation.
func DeleteOrphans(req router.Request, resp router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.ThreadName != "" || server.Spec.SharedWithinMCPCatalogName != "" {
		return nil
	} else if since := time.Since(server.CreationTimestamp.Time); since < time.Hour {
		resp.RetryAfter(time.Hour - since)
		return nil
	}

	var instance v1.MCPServerInstance
	if err := req.Get(&instance, server.Namespace, fmt.Sprintf("%s-%s-%s", system.MCPServerInstancePrefix, server.Spec.UserID, server.Name)); errors.IsNotFound(err) {
		log.Infof("Deleting orphaned MCP server %s/%s", req.Namespace, server.Name)
		return req.Delete(server)
	} else if err != nil {
		return err
	}

	return nil
}

func CheckForUpdates(req router.Request, resp router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.MCPServerCatalogEntryName == "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
		return err
	}

	drifted, err := configurationHasDrifted(server.Spec.Manifest, entry.Spec.CommandManifest)
	if err != nil {
		return err
	}

	server.Status.NeedsUpdate = drifted
	return req.Client.Status().Update(req.Ctx, server)
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

	if entryManifest.FixedURL != "" {
		if serverManifest.URL != entryManifest.FixedURL {
			return true, nil
		}
	} else if entryManifest.Hostname != "" {
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
