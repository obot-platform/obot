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

	ModeConsent Mode = "consent"
	ModeOn      Mode = "on"
	ModeOff     Mode = "off"
)

var (
	errConsentForced = errors.New("product telemetry consent is operator-managed")
)

// Mode controls whether consent is user-managed or set by the operator.
type Mode string

// Consent persists and resolves the installation-wide product telemetry consent state.
type Consent struct {
	gatewayClient *client.Client
	mode          Mode
}

func (mode Mode) Validate() error {
	switch mode {
	case "", ModeConsent, ModeOn, ModeOff:
		return nil
	default:
		return fmt.Errorf("invalid product analytics mode %q: expected consent, on, or off", mode)
	}
}

func NewConsent(gatewayClient *client.Client, mode Mode) *Consent {
	return &Consent{
		gatewayClient: gatewayClient,
		mode:          mode,
	}
}

func (c *Consent) UserConfigurable() bool {
	return c.mode == "" || c.mode == ModeConsent
}

func (c *Consent) DisabledByOperator() bool {
	return c.mode == ModeOff
}

// Get returns effective consent. A nil value means consent is undecided. When
// analytics is on or off, Get returns the operator's choice without consulting persistence.
func (c *Consent) Get(ctx context.Context) (*bool, error) {
	switch c.mode {
	case ModeOn:
		return new(true), nil
	case ModeOff:
		return new(false), nil
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
	if !c.UserConfigurable() {
		return errConsentForced
	}

	if _, err := c.gatewayClient.SetProperty(ctx, consentPropertyKey, strconv.FormatBool(value)); err != nil {
		return fmt.Errorf("set product telemetry consent: %w", err)
	}
	return nil
}
