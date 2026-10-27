package mcp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// OAuthTargetHash binds a challenge to its current owner and upstream configuration
// without persisting configured secrets in the challenge or OAuth transaction.
func OAuthTargetHash(server v1.MCPServer, cfg ServerConfig) string {
	data, err := json.Marshal(struct {
		UID                     string
		Owner                   string
		Instance                string
		Component               string
		URL                     string
		Tunnel                  string
		Manifest                any
		Headers                 []string
		PassthroughHeaderNames  []string
		PassthroughHeaderValues []string
		VMCPID                  string
		VMCPInstanceID          string
	}{
		UID:                     string(server.UID),
		Owner:                   server.Spec.UserID,
		Instance:                cfg.MCPServerInstanceID,
		Component:               server.Spec.VMCPComponentID,
		URL:                     cfg.URL,
		Tunnel:                  cfg.TunnelName,
		Manifest:                server.Spec.Manifest,
		Headers:                 cfg.Headers,
		PassthroughHeaderNames:  cfg.PassthroughHeaderNames,
		PassthroughHeaderValues: cfg.PassthroughHeaderValues,
		VMCPID:                  server.Spec.VMCPID,
		VMCPInstanceID:          server.Spec.VMCPInstanceID,
	})
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%x", sha256.Sum256(data))
}
