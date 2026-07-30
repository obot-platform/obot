package types

import (
	"fmt"
	"regexp"
)

const maxTunnelNameLength = 63

var tunnelNameRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// ValidateTunnelName validates the MCPTunnel ID used to select a connection.
func ValidateTunnelName(name string) error {
	if name == "" {
		return fmt.Errorf("tunnel name is required")
	}
	if len(name) > maxTunnelNameLength {
		return fmt.Errorf("tunnel name must not exceed %d characters", maxTunnelNameLength)
	}
	if !tunnelNameRegex.MatchString(name) {
		return fmt.Errorf("tunnel name must contain only lowercase alphanumeric characters or hyphens, and must start and end with an alphanumeric character")
	}
	return nil
}

// TunnelConnection describes a tunnel that is currently connected to this
// Obot installation.
type TunnelConnection struct {
	Name string `json:"name"`
}

type TunnelConnectionList List[TunnelConnection]
