package openapi

import (
	"net/url"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/obot-platform/obot/apiclient/types"
)

// suggestedMetadata extracts optional catalog details without changing the
// snapshot or fetching icon URLs. Users decide whether to apply these values.
func suggestedMetadata(info *openapi3.Info, sourceURL string) types.OpenAPIMetadata {
	metadata := types.OpenAPIMetadata{
		Name:             strings.TrimSpace(info.Title),
		Description:      strings.TrimSpace(info.Description),
		ShortDescription: strings.TrimSpace(info.Summary),
	}

	logo, _ := info.Extensions["x-logo"].(map[string]any)
	logoURL, _ := logo["url"].(string)
	if strings.TrimSpace(logoURL) == "" {
		return metadata
	}

	icon, err := url.Parse(strings.TrimSpace(logoURL))
	if err != nil {
		return metadata
	}

	if source, err := url.Parse(sourceURL); err == nil {
		icon = source.ResolveReference(icon)
	}

	if icon.Scheme == "https" && icon.Hostname() != "" && icon.User == nil {
		metadata.Icon = icon.String()
	}

	return metadata
}
