package license

import (
	"context"
	"math"
	"strconv"
	"strings"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

const (
	enterpriseLimitEntitlementPrefix    = "OBOT_ENTERPRISE_"
	userLimitEntitlementUsersSuffix     = "_USERS"
	deviceLimitEntitlementDevicesSuffix = "_DEVICES"
)

// GetDistributionFromEntitlements returns the product distribution represented by the entitlements.
func GetDistributionFromEntitlements(entitlements types.Entitlements) types.ProductTelemetryDistribution {
	switch {
	case entitlements.Cloud:
		return types.ProductTelemetryDistributionCloud
	case entitlements.Enterprise:
		return types.ProductTelemetryDistributionEnterprise
	case entitlements.Community:
		return types.ProductTelemetryDistributionRegistered
	default:
		return types.ProductTelemetryDistributionUnregistered
	}
}

// Entitlements returns the license's entitlements. A license only limits users and devices.
func (p *Provider) Entitlements(ctx context.Context) (types.Entitlements, error) {
	if err := p.refresh(ctx, false); err != nil {
		return types.Entitlements{}, err
	}

	p.lock.RLock()
	defer p.lock.RUnlock()

	has := func(code string) bool {
		_, ok := p.entitlements[keygen.EntitlementCode(code)]
		return ok
	}
	return types.Entitlements{
		Community:                has(CommunityEntitlement),
		Enterprise:               has(EnterpriseEntitlement),
		Cloud:                    has(CloudEntitlement),
		EnterpriseAuthProviders:  has(EnterpriseAuthProvidersEntitlement),
		EnterpriseModelProviders: has(EnterpriseModelProvidersEntitlement),
		Users:                    p.resourceLimit(userLimitEntitlementUsersSuffix, gatewayclient.DefaultUserLimit),
		Devices:                  p.resourceLimit(deviceLimitEntitlementDevicesSuffix, gatewayclient.DefaultDeviceLimit),
	}, nil
}

// Restricted is always false. Only billing restricts an environment.
func (p *Provider) Restricted(context.Context) (bool, error) {
	return false, nil
}

// Usage counts the users and devices the license limits.
func (p *Provider) Usage(ctx context.Context) (types.EntitlementsUsage, error) {
	users, err := p.gatewayClient.UserCount(ctx)
	if err != nil {
		return types.EntitlementsUsage{}, err
	}
	devices, err := p.gatewayClient.DeviceCount(ctx)
	if err != nil {
		return types.EntitlementsUsage{}, err
	}
	return types.EntitlementsUsage{
		Users:   users,
		Devices: devices,
	}, nil
}

// resourceLimit adds up the license's OBOT_ENTERPRISE_<number><entitlementSuffix> entitlements. OBOT_ENTERPRISE
// without any is unlimited, and a license with neither gets defaultMaximum. The caller must hold p.lock.
func (p *Provider) resourceLimit(entitlementSuffix string, defaultMaximum int64) *int64 {
	var maximum int64
	var isEnterpriseEdition bool
	for entitlement := range p.entitlements {
		code := string(entitlement)
		if code == EnterpriseEntitlement {
			isEnterpriseEdition = true
			continue
		}

		value, ok := strings.CutPrefix(code, enterpriseLimitEntitlementPrefix)
		if !ok {
			continue
		}
		value, ok = strings.CutSuffix(value, entitlementSuffix)
		if !ok {
			continue
		}
		if value == "" || strings.IndexFunc(value, func(r rune) bool {
			return r < '0' || r > '9'
		}) >= 0 {
			continue
		}

		entitlementMaximum, err := strconv.ParseInt(value, 10, 64)
		if err != nil || entitlementMaximum <= 0 {
			continue
		}

		if entitlementMaximum > math.MaxInt64-maximum {
			maximum = math.MaxInt64
		} else {
			maximum += entitlementMaximum
		}
	}

	if maximum == 0 {
		if isEnterpriseEdition {
			return nil
		}
		maximum = defaultMaximum
	}
	return &maximum
}
