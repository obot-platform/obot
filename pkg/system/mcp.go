package system

import (
	"fmt"
)

func MCPConnectURL(serverURL, id string) string {
	return fmt.Sprintf("%s/mcp-connect/%s", serverURL, id)
}
