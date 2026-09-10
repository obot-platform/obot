package mcptester

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

const (
	descriptorURLHeader     = "X-Obot-MCP-URL"
	descriptorCommandHeader = "X-Obot-MCP-Command"
	descriptorImageHeader   = "X-Obot-MCP-Image"
)

var (
	errDescriptor  = errors.New("MCP deployment has no safe model proxy identification")
	packagePattern = regexp.MustCompile(`^(?:@[a-zA-Z0-9._-]+/)?[a-zA-Z0-9_][a-zA-Z0-9._-]*(?:\[[a-zA-Z0-9_,.-]+\])?(?:@{1,2}[a-zA-Z0-9*._+^~=-]+|==[a-zA-Z0-9._+!-]+)?$`)
	imagePattern   = regexp.MustCompile(`^(?:[a-zA-Z0-9.-]+(?::[0-9]+)?/)?[a-z0-9]+(?:[._-]+[a-z0-9]+)*(?:/[a-z0-9]+(?:[._-]+[a-z0-9]+)*)*(?::[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127})?(?:@sha256:[a-fA-F0-9]{64})?$`)
)

type Descriptor struct {
	Header string
	Value  string
}

// DeploymentDescriptor uses the original runtime, even when npx/uvx runs in a
// container. Package descriptors include the runtime followed by the sanitized
// package identifier. No arguments, environment, headers, or wrapper URL are included.
func DeploymentDescriptor(manifest types.MCPServerManifest, resolved mcp.ServerConfig) (Descriptor, error) {
	var result Descriptor
	switch manifest.Runtime {
	case types.RuntimeNPX:
		if manifest.NPXConfig == nil {
			return result, errDescriptor
		}

		result = Descriptor{Header: descriptorCommandHeader, Value: manifest.NPXConfig.Package}
	case types.RuntimeUVX:
		if manifest.UVXConfig == nil {
			return result, errDescriptor
		}

		result = Descriptor{Header: descriptorCommandHeader, Value: manifest.UVXConfig.Package}
	case types.RuntimeContainerized:
		if manifest.ContainerizedConfig == nil {
			return result, errDescriptor
		}

		result = Descriptor{Header: descriptorImageHeader, Value: manifest.ContainerizedConfig.Image}
	case types.RuntimeRemote:
		if manifest.RemoteConfig == nil {
			return result, errDescriptor
		}

		value := resolved.URL
		if value == "" {
			value = manifest.RemoteConfig.URL
		}

		result = Descriptor{Header: descriptorURLHeader, Value: value}
	default:
		return result, errDescriptor
	}

	if len(result.Value) > 2048 || !utf8.ValidString(result.Value) {
		return Descriptor{}, errDescriptor
	}

	for _, char := range result.Value {
		if unicode.IsControl(char) || char == ',' {
			return Descriptor{}, errDescriptor
		}
	}

	result.Value = strings.TrimSpace(result.Value)

	var err error
	switch result.Header {
	case descriptorURLHeader:
		result.Value, err = sanitizeURL(result.Value)
	case descriptorCommandHeader:
		result.Value, err = sanitizePackageReference(result.Value)
	case descriptorImageHeader:
		if !imagePattern.MatchString(result.Value) {
			err = errDescriptor
		}
	}

	if err != nil {
		return Descriptor{}, err
	}

	if result.Header == descriptorCommandHeader {
		result.Value = string(manifest.Runtime) + " " + result.Value
		if len(result.Value) > 2048 {
			return Descriptor{}, errDescriptor
		}
	}

	return result, nil
}

// Remote MCP URLs have no allowlisted path structure and retain only the origin.
func sanitizeURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || !validServiceHost(parsed) || parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errDescriptor
	}

	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}
