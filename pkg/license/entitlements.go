package license

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

const (
	enterpriseLimitEntitlementPrefix    = "OBOT_ENTERPRISE_"
	userLimitEntitlementUsersSuffix     = "_USERS"
	deviceLimitEntitlementDevicesSuffix = "_DEVICES"
)

var (
	// features maps each license code that grants a feature to that feature.
	features = []struct {
		code    string
		feature func(*types.Entitlements) *bool
	}{
		{CloudEntitlement, func(e *types.Entitlements) *bool { return &e.Cloud }},
		{CommunityEntitlement, func(e *types.Entitlements) *bool { return &e.Community }},
		{EnterpriseEntitlement, func(e *types.Entitlements) *bool { return &e.Enterprise }},
		{EnterpriseAuthProvidersEntitlement, func(e *types.Entitlements) *bool { return &e.EnterpriseAuthProviders }},
		{EnterpriseModelProvidersEntitlement, func(e *types.Entitlements) *bool { return &e.EnterpriseModelProviders }},
	}
)

// Features returns the features the codes grant. Codes that don't grant a feature, like limits, are ignored.
func Features(codes []string) types.Entitlements {
	var entitlements types.Entitlements
	for _, feature := range features {
		if slices.Contains(codes, feature.code) {
			*feature.feature(&entitlements) = true
		}
	}
	return entitlements
}

// Codes returns the codes of the features the entitlements grant.
func Codes(entitlements types.Entitlements) []string {
	var codes []string
	for _, feature := range features {
		if *feature.feature(&entitlements) {
			codes = append(codes, feature.code)
		}
	}
	return codes
}

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

// Entitlements returns the license's features and its user and device limits. A license limits nothing else.
func (p *Provider) Entitlements(ctx context.Context) (types.Entitlements, error) {
	if err := p.refresh(ctx, false); err != nil {
		return types.Entitlements{}, err
	}

	p.lock.RLock()
	defer p.lock.RUnlock()

	codes := make([]string, 0, len(p.entitlements))
	for code := range p.entitlements {
		codes = append(codes, string(code))
	}
	entitlements := Features(codes)
	entitlements.Users = p.resourceLimit(userLimitEntitlementUsersSuffix, gatewayclient.DefaultUserLimit)
	entitlements.Devices = p.resourceLimit(deviceLimitEntitlementDevicesSuffix, gatewayclient.DefaultDeviceLimit)
	return entitlements, nil
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
