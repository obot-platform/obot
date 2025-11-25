package system

import (
	"context"
	"fmt"
)

func MCPConnectURL(serverURL, id string) string {
	return fmt.Sprintf("%s/mcp-connect/%s", serverURL, id)
}

type JWKS func(context.Context) ([]byte, error)

type EncodedJWKS func(context.Context) (string, error)
