package mcpservercatalogentry

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func RemoveArgsOnRemoteEntries(req router.Request, _ router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	if entry.Spec.SourceURL != "" {
		return nil
	}

	// The URLManifest should never have args on it, but there was a bug where sometimes they were created with one empty string as an argument.
	if entry.Spec.URLManifest.Args != nil {
		entry.Spec.URLManifest.Args = nil
		return req.Client.Update(req.Ctx, entry)
	}

	return nil
}
