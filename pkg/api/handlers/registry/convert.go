package registry

import (
	"cmp"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// ConvertVMCPToRegistry publishes a definition's canonical endpoint. Authentication,
// user configuration, and instance creation happen in the connection's consent flow.
func ConvertVMCPToRegistry(
	definition v1.VMCP,
	serverURL string,
	reverseDNS string,
) types.RegistryServerResponse {
	manifest := definition.Spec.Manifest
	// The registry schema limits descriptions to 100 characters. Keep the full
	// description in the publisher-provided README, including its Markdown.
	description := []rune(cmp.Or(strings.TrimSpace(manifest.Description), manifest.DisplayName, definition.Name))
	detail := types.RegistryServerDetail{
		Name:        FormatRegistryServerName(reverseDNS, definition.Name),
		Description: string(description[:min(len(description), 100)]),
		Title:       cmp.Or(manifest.DisplayName, definition.Name),
		Version:     "latest",
		Schema:      "https://static.modelcontextprotocol.io/schemas/2025-09-29/server.schema.json",
		Remotes: []types.RegistryServerRemote{
			{
				Type: "streamable-http",
				URL:  system.MCPConnectURL(serverURL, definition.Name),
			},
		},
		Meta: types.RegistryServerMeta{
			PublisherProvided: &types.RegistryPublisherProvidedMeta{
				GitHub: &types.RegistryGitHubMeta{
					Readme: manifest.Description,
				},
			},
		},
	}
	if manifest.Icon != "" {
		detail.Icons = []types.RegistryServerIcon{
			{
				Src:      manifest.Icon,
				MimeType: guessMimeType(manifest.Icon),
			},
		}
	}
	return types.RegistryServerResponse{
		Server: detail,
		Meta: types.RegistryMeta{
			Official: types.RegistryOfficialMeta{
				IsLatest:  true,
				Status:    "active",
				CreatedAt: definition.CreationTimestamp.Format(time.RFC3339),
			},
		},
		CreatedAtUnix: definition.CreationTimestamp.Unix(),
	}
}
