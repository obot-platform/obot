package system

import (
	"fmt"
)

const (
	// MCPOAuthCredentialContext is the credential context prefix for MCP OAuth credentials
	MCPOAuthCredentialContext = "mcp-oauth"
)

// MCPOAuthCredentialName returns the credential name for an MCP server's OAuth credentials
func MCPOAuthCredentialName(mcpServerName string) string {
	return fmt.Sprintf("%s-%s", MCPOAuthCredentialContext, mcpServerName)
}

func MCPConnectURL(serverURL, id string) string {
	return fmt.Sprintf("%s/mcp-connect/%s", serverURL, id)
}
