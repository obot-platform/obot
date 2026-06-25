package system

import (
	"fmt"
	"strings"
)

const (
	// MCPOAuthCredentialContextPrefix is the credential context prefix for MCP OAuth credentials
	MCPOAuthCredentialContextPrefix = "mcp-oauth"
	OAuthClientIDMetadataPath       = "/oauth/client-metadata.json"
)

// MCPOAuthCredentialName returns the credential name for an MCP server's OAuth credentials
func MCPOAuthCredentialName(mcpServerName string) string {
	return fmt.Sprintf("%s-%s", MCPOAuthCredentialContextPrefix, mcpServerName)
}

func MCPConnectURL(serverURL, id string) string {
	return fmt.Sprintf("%s/mcp-connect/%s", serverURL, id)
}

func NanobotAgentConnectURL(serverURL, id string) string {
	return MCPConnectURL(serverURL, MCPServerPrefix+id)
}

func MCPOAuthCallbackURL(serverURL string) string {
	return fmt.Sprintf("%s/oauth/mcp/callback", serverURL)
}

func OAuthClientIDMetadataURL(serverURL string) string {
	return strings.TrimRight(serverURL, "/") + OAuthClientIDMetadataPath
}
