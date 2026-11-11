package license

import (
	"context"
	"slices"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

func (p *Provider) hasEntitlement(key string) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, ok := p.entitlements[keygen.EntitlementCode(key)]
	return ok
}

func (p *Provider) codes(ctx context.Context) ([]string, error) {
	if err := p.refresh(ctx, false); err != nil {
		return nil, err
	}
	p.lock.RLock()
	defer p.lock.RUnlock()

	codes := make([]string, 0, len(p.entitlements))
	for code := range p.entitlements {
		codes = append(codes, string(code))
	}
	slices.Sort(codes)
	return codes, nil
}

func (p *Provider) userLimit(t *testing.T) (gatewayclient.UserLimit, error) {
	t.Helper()
	entitlements, err := p.Entitlements(t.Context())
	if err != nil || entitlements.Users == nil {
		return gatewayclient.UserLimit{Unlimited: err == nil}, err
	}
	return gatewayclient.UserLimit{Maximum: *entitlements.Users}, nil
}

func (p *Provider) deviceLimit(t *testing.T) (gatewayclient.DeviceLimit, error) {
	t.Helper()
	entitlements, err := p.Entitlements(t.Context())
	if err != nil || entitlements.Devices == nil {
		return gatewayclient.DeviceLimit{Unlimited: err == nil}, err
	}
	return gatewayclient.DeviceLimit{Maximum: *entitlements.Devices}, nil
}
