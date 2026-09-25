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
	errConsentForced = errors.New("product telemetry consent is operator-managed")
)

// Consent persists and resolves the installation-wide product telemetry consent state.
type Consent struct {
	gatewayClient *client.Client
	forcedValue   *bool
}

func NewConsent(gatewayClient *client.Client, forcedValue *bool) *Consent {
	return &Consent{
		gatewayClient: gatewayClient,
		forcedValue:   forcedValue,
	}
}

func (c *Consent) UserConfigurable() bool {
	return c.forcedValue == nil
}

// Get returns effective consent. A nil value means consent is undecided. When
// consent is forced, Get returns the operator's choice without consulting persistence.
func (c *Consent) Get(ctx context.Context) (*bool, error) {
	if c.forcedValue != nil {
		return new(*c.forcedValue), nil
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
	if c.forcedValue != nil {
		return errConsentForced
	}

	if _, err := c.gatewayClient.SetProperty(ctx, consentPropertyKey, strconv.FormatBool(value)); err != nil {
		return fmt.Errorf("set product telemetry consent: %w", err)
	}
	return nil
}
