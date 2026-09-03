package producttelemetry

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"gorm.io/gorm"
)

const (
	consentPropertyKey = "product_telemetry_consent"
)

var (
	errConsentForceEnabled = errors.New("product telemetry consent is force-enabled")
)

// Consent persists and resolves the installation-wide product telemetry consent state.
type Consent struct {
	gatewayClient *client.Client
	forceEnabled  bool
}

func NewConsent(gatewayClient *client.Client, forceEnabled bool) *Consent {
	return &Consent{
		gatewayClient: gatewayClient,
		forceEnabled:  forceEnabled,
	}
}

func (c *Consent) ForceEnabled() bool {
	return c.forceEnabled
}

// Get returns effective consent. A nil value means consent is undecided. When
// consent is force-enabled, Get returns true without consulting persistence.
func (c *Consent) Get(ctx context.Context) (*bool, error) {
	if c.forceEnabled {
		return new(true), nil
	}

	property, err := c.gatewayClient.GetProperty(ctx, consentPropertyKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get product telemetry consent: %w", err)
	}

	value, err := strconv.ParseBool(property.Value)
	if err != nil {
		return nil, fmt.Errorf("parse product telemetry consent: %w", err)
	}
	return &value, nil
}

func (c *Consent) Set(ctx context.Context, value bool) error {
	if c.forceEnabled {
		return errConsentForceEnabled
	}

	if _, err := c.gatewayClient.SetProperty(ctx, consentPropertyKey, strconv.FormatBool(value)); err != nil {
		return fmt.Errorf("set product telemetry consent: %w", err)
	}
	return nil
}
