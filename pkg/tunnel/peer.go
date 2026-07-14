package tunnel

import (
	"fmt"
	"strings"
)

// PeerConnectPath is the internal websocket endpoint used to connect Obot
// tunnel servers to one another.
const PeerConnectPath = "/tunnel/peer"

// PeerConfig configures authentication and discovery of the other Obot
// replicas that participate in the tunnel remotedialer mesh. An entirely empty
// config disables peering; otherwise every field is required.
type PeerConfig struct {
	ID               string
	Token            string
	ServiceName      string
	ServiceNamespace string
}

// Validate verifies that tunnel peering is either fully disabled or fully
// configured.
func (c PeerConfig) Validate() error {
	_, err := c.validate()
	return err
}

// Enabled reports whether a valid config enables tunnel peering. Call
// Validate first when an invalid partial config must be distinguished from a
// disabled config.
func (c PeerConfig) Enabled() bool {
	enabled, err := c.validate()
	return err == nil && enabled
}

func (c PeerConfig) validate() (bool, error) {
	c = c.normalized()

	if c.ID == "" && c.Token == "" && c.ServiceName == "" && c.ServiceNamespace == "" {
		return false, nil
	}

	missing := make([]string, 0, 4)
	if c.ID == "" {
		missing = append(missing, "ID")
	}
	if c.Token == "" {
		missing = append(missing, "Token")
	}
	if c.ServiceName == "" {
		missing = append(missing, "ServiceName")
	}
	if c.ServiceNamespace == "" {
		missing = append(missing, "ServiceNamespace")
	}
	if len(missing) > 0 {
		return false, fmt.Errorf("tunnel peer config is incomplete: missing %s", strings.Join(missing, ", "))
	}

	return true, nil
}

func (c PeerConfig) normalized() PeerConfig {
	c.ID = strings.TrimSpace(c.ID)
	c.Token = strings.TrimSpace(c.Token)
	c.ServiceName = strings.TrimSpace(c.ServiceName)
	c.ServiceNamespace = strings.TrimSpace(c.ServiceNamespace)
	return c
}
