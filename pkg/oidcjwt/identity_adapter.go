package oidcjwt

import (
	"context"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func NewGatewayIdentityResolver(c *gclient.Client) IdentityResolver {
	return &gatewayResolver{c: c}
}

type gatewayResolver struct {
	c *gclient.Client
}

func (g *gatewayResolver) ResolveOrCreate(ctx context.Context, id *gwtypes.Identity, timezone string) (*gwtypes.User, error) {
	return g.c.EnsureIdentity(ctx, id, timezone)
}
