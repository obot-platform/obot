package client

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"gorm.io/gorm"
)

var (
	// hostedMCPServerRuntimes are the runtimes Obot pays to run. A remote server
	// runs outside Obot and a vMCP server runs nothing of its own, so neither
	// counts.
	hostedMCPServerRuntimes = []apitypes.Runtime{
		apitypes.RuntimeUVX,
		apitypes.RuntimeNPX,
		apitypes.RuntimeContainerized,
	}
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

// IsHostedRuntime reports whether Obot runs a server with this runtime.
func IsHostedRuntime(runtime apitypes.Runtime) bool {
	return slices.Contains(hostedMCPServerRuntimes, runtime)
}

// Enforced reports whether the limit bounds the number of hosted MCP servers.
func (l HostedMCPServerLimit) Enforced() bool {
	return !l.Unlimited && l.Maximum > 0
}

// SetHostedMCPServerLimitProvider supplies the hosted MCP server limit. The
// gateway client is constructed before anything that can resolve entitlements.
func (c *Client) SetHostedMCPServerLimitProvider(provider HostedMCPServerLimitProvider) {
	c.hostedMCPServerLimitProvider.Store(&provider)
}

// HostedMCPServerLimit returns the limit currently in force, which is unbounded
// until a provider is supplied.
func (c *Client) HostedMCPServerLimit(ctx context.Context) (HostedMCPServerLimit, error) {
	if c == nil {
		return HostedMCPServerLimit{}, nil
	}

	provider := c.hostedMCPServerLimitProvider.Load()
	if provider == nil {
		return HostedMCPServerLimit{}, nil
	}
	return (*provider).HostedMCPServerLimit(ctx)
}

// HostedMCPServerCount returns the number of MCP servers this installation runs.
// Servers are counted as they are created rather than as they are deployed,
// because a server deploys lazily and shuts down when it goes idle.
func (c *Client) HostedMCPServerCount(ctx context.Context) (int64, error) {
	if c == nil || c.storageClient == nil {
		return 0, nil
	}

	// Listed and filtered here rather than through a field selector, so the count
	// does not depend on an index being registered for the runtime field.
	var servers v1.MCPServerList
	if err := c.storageClient.List(ctx, &servers); err != nil {
		return 0, fmt.Errorf("failed to list MCP servers: %w", err)
	}

	var count int64
	for _, server := range servers.Items {
		if IsHostedRuntime(server.Spec.Manifest.Runtime) {
			count++
		}
	}
	return count, nil
}

// CreateHostedMCPServer runs create unless the installation is already at its
// hosted MCP server limit.
//
// Counting and creating are serialized, in this process and across PostgreSQL
// replicas, by the same advisory lock user seat allocation takes, so two replicas
// cannot both claim the last server.
func (c *Client) CreateHostedMCPServer(ctx context.Context, runtime apitypes.Runtime, create func(context.Context) error) error {
	// A server we do not run costs nothing, so there is nothing to count it
	// against. A caller without a gateway client cannot count either, which only
	// happens in tests; the request path always has one.
	if c == nil || !IsHostedRuntime(runtime) {
		return create(ctx)
	}

	limit, err := c.HostedMCPServerLimit(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve hosted MCP server limit: %w", err)
	}
	if !limit.Enforced() {
		// Nothing bounds hosted servers here. Skip the count and the lock.
		return create(ctx)
	}

	// The in-process lock covers SQLite, where the advisory lock below is a
	// no-op because there is only ever one replica.
	c.hostedMCPServerCreationLock.Lock()
	defer c.hostedMCPServerCreationLock.Unlock()

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUserCreation(tx); err != nil {
			return err
		}

		// Counted inside the lock, so another replica cannot create a server
		// between this count and the create below.
		count, err := c.HostedMCPServerCount(ctx)
		if err != nil {
			return err
		}
		if count >= limit.Maximum {
			return newHostedMCPServerLimitError()
		}

		return create(ctx)
	})
}

func newHostedMCPServerLimitError() error {
	return apitypes.NewErrHTTP(
		http.StatusForbidden,
		"This installation is already running as many MCP servers as it is licensed for. Please contact your administrator.",
	)
}
