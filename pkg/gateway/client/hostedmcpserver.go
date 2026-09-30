package client

import (
	"context"
)

// HostedMCPServerLimit describes the maximum number of MCP servers an installation
// may host. Maximum is ignored when Unlimited is true, and a zero Maximum means no
// entitlement defines a limit.
type HostedMCPServerLimit struct {
	Maximum   int64
	Unlimited bool
}

// HostedMCPServerLimitProvider resolves the current entitlement-derived hosted MCP
// server limit.
type HostedMCPServerLimitProvider interface {
	HostedMCPServerLimit(context.Context) (HostedMCPServerLimit, error)
}

// Enforced reports whether the limit bounds the number of hosted MCP servers.
func (l HostedMCPServerLimit) Enforced() bool {
	return !l.Unlimited && l.Maximum > 0
}
