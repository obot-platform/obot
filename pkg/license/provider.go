package license

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"gorm.io/gorm"
)

const (
	// LicenseKeyPropertyKey is the database property key used to persist the Keygen license key.
	LicenseKeyPropertyKey = "obot-license-key"

	// LicenseMachineIDPropertyKey is the database property key used to persist the Keygen machine fingerprint.
	LicenseMachineIDPropertyKey = "obot-license-machine-id"

	// EnterpriseAuthProvidersEntitlement is required to enable enterprise auth providers.
	EnterpriseAuthProvidersEntitlement = "OBOT_ENTERPRISE_AUTH_PROVIDERS"

	// EnterpriseModelProvidersEntitlement is required to enable enterprise model providers.
	EnterpriseModelProvidersEntitlement = "OBOT_ENTERPRISE_MODEL_PROVIDERS"

	defaultPollInterval = time.Hour
	keygenProduct       = "18a762f2-5281-45cf-93fc-e45e2d932094"
	keygenAccount       = "7565373b-6069-4a0b-9495-9777d9db3fd9"
)

var (
	// ErrNotConfigured indicates license validation was requested without enough Keygen configuration.
	ErrNotConfigured = errors.New("license provider is not configured")

	// ErrLicenseKeyViaConfiguration indicates the license key is managed by startup configuration.
	ErrLicenseKeyViaConfiguration = errors.New("license key is configured at startup")

	// ErrInvalidLicense indicates the provided license key could not be validated.
	ErrInvalidLicense = errors.New("license key is invalid")

	log = logger.Package()
)

// Config contains the Keygen settings needed to validate an Obot license.
type Config struct {
	KeygenLicenseKey string `usage:"Keygen license key for this Obot installation"`
}

type KeygenProvider struct {
	lock                       sync.RWMutex
	entitlements               map[keygen.EntitlementCode]struct{}
	machineFingerprint         string
	gatewayClient              *client.Client
	licenseKeyViaConfiguration bool
}

// NewProvider creates a Keygen-backed license provider.
func NewProvider(ctx context.Context, gatewayClient *client.Client, config Config) (*KeygenProvider, error) {
	keygen.Account = keygenAccount
	keygen.Product = keygenProduct

	machineFingerprint, err := ensureMachineFingerprint(ctx, gatewayClient)
	if err != nil {
		return nil, err
	}

	k := &KeygenProvider{
		machineFingerprint: machineFingerprint,
		gatewayClient:      gatewayClient,
	}

	if licenseKey := strings.TrimSpace(config.KeygenLicenseKey); licenseKey != "" {
		if err := k.setLicenseKey(ctx, licenseKey, true, true); err != nil {
			return nil, err
		}
	} else if gatewayClient != nil {
		property, err := gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to get license key property: %w", err)
		}
		if err == nil {
			if err := k.setLicenseKey(ctx, property.Value, false, true); err != nil {
				return nil, err
			}
		} else {
			log.Infof("license provider is not configured, license key is empty")
		}
	} else {
		log.Infof("license provider is not configured, license key is empty")
	}

	go k.poll(ctx)

	return k, nil
}

func ensureMachineFingerprint(ctx context.Context, gatewayClient *client.Client) (string, error) {
	if gatewayClient == nil {
		return uuid.NewString(), nil
	}

	property, err := gatewayClient.GetOrCreateProperty(ctx, LicenseMachineIDPropertyKey, uuid.NewString())
	if err != nil {
		return "", fmt.Errorf("failed to ensure license machine ID: %w", err)
	}
	return property.Value, nil
}

func (p *KeygenProvider) LicenseKey() string {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return keygen.LicenseKey
}

func (p *KeygenProvider) LicenseKeyViaConfiguration() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.licenseKeyViaConfiguration
}

func (p *KeygenProvider) SetLicenseKey(ctx context.Context, licenseKey string) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	return p.setLicenseKey(ctx, licenseKey, false, false)
}

func (p *KeygenProvider) setLicenseKey(ctx context.Context, licenseKey string, viaConfiguration, allowInvalid bool) error {
	licenseKey = strings.TrimSpace(licenseKey)

	p.lock.Lock()
	previousLicenseKey := keygen.LicenseKey
	previousViaConfiguration := p.licenseKeyViaConfiguration
	previousEntitlements := p.entitlements

	keygen.LicenseKey = licenseKey
	p.licenseKeyViaConfiguration = viaConfiguration
	p.lock.Unlock()

	entitlements, err := p.validate(ctx)
	if err != nil && !errors.Is(err, ErrNotConfigured) {
		p.restoreLicenseState(previousLicenseKey, previousViaConfiguration, previousEntitlements)
		return err
	}
	if entitlements == nil && !allowInvalid {
		p.restoreLicenseState(previousLicenseKey, previousViaConfiguration, previousEntitlements)
		return ErrInvalidLicense
	}

	if !viaConfiguration && entitlements != nil && p.gatewayClient != nil {
		if _, err := p.gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, licenseKey); err != nil {
			p.restoreLicenseState(previousLicenseKey, previousViaConfiguration, previousEntitlements)
			return err
		}
	}

	p.lock.Lock()
	p.entitlements = entitlements
	p.lock.Unlock()

	return nil
}

func (p *KeygenProvider) restoreLicenseState(licenseKey string, viaConfiguration bool, entitlements map[keygen.EntitlementCode]struct{}) {
	p.lock.Lock()
	defer p.lock.Unlock()

	keygen.LicenseKey = licenseKey
	p.licenseKeyViaConfiguration = viaConfiguration
	p.entitlements = entitlements
}

func (p *KeygenProvider) RemoveLicenseKey(ctx context.Context) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	if p.gatewayClient != nil {
		if err := p.gatewayClient.DeleteProperty(ctx, LicenseKeyPropertyKey); err != nil {
			return err
		}
	}

	p.lock.Lock()
	keygen.LicenseKey = ""
	p.lock.Unlock()

	p.update(ctx)
	return nil
}

func (p *KeygenProvider) validate(ctx context.Context) (map[keygen.EntitlementCode]struct{}, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if err := validateConfig(); err != nil {
		return nil, err
	}

	lic, err := keygen.Validate(ctx, p.machineFingerprint)
	if err != nil {
		if lic != nil && lic.LastValidation != nil && lic.LastValidation.Code == keygen.ValidationCodeNoMachine || errors.Is(err, keygen.ErrLicenseNotActivated) {
			if _, activationErr := lic.Activate(ctx, p.machineFingerprint); activationErr != nil && !errors.Is(activationErr, keygen.ErrMachineAlreadyActivated) {
				log.Warnf("license activation failed: %v", activationErr)
				return nil, nil
			}

			lic, err = keygen.Validate(ctx, p.machineFingerprint)
		}
		if err != nil {
			log.Warnf("license validation failed: %v", err)
			return nil, nil
		}
	}

	entitlements, err := lic.Entitlements(ctx)
	if err != nil {
		return nil, fmt.Errorf("list license entitlements: %w", err)
	}

	entitlementSet := make(map[keygen.EntitlementCode]struct{}, len(entitlements))
	for _, entitlement := range entitlements {
		entitlementSet[entitlement.Code] = struct{}{}
	}

	return entitlementSet, nil
}

func (p *KeygenProvider) HasValidLicense() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.entitlements != nil
}

func (p *KeygenProvider) Entitlements() []string {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.entitlements == nil {
		return nil
	}

	entitlements := make([]string, 0, len(p.entitlements))
	for entitlement := range p.entitlements {
		entitlements = append(entitlements, string(entitlement))
	}

	slices.Sort(entitlements)

	return entitlements
}

func (p *KeygenProvider) hasEntitlement(key string) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, ok := p.entitlements[keygen.EntitlementCode(key)]
	return ok
}

func (p *KeygenProvider) poll(ctx context.Context) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.update(ctx)
		}
	}
}

func (p *KeygenProvider) update(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var (
		entitlements  map[keygen.EntitlementCode]struct{}
		hasLicenceKey bool
		err           error
	)

	p.lock.RLock()
	if keygen.LicenseKey != "" {
		hasLicenceKey = true
		entitlements, err = p.validate(ctx)
	}
	p.lock.RUnlock()

	p.lock.Lock()
	defer p.lock.Unlock()

	if err != nil || !hasLicenceKey {
		p.entitlements = nil
		return
	}

	p.entitlements = entitlements
}

func validateConfig() error {
	if strings.TrimSpace(keygen.Account) == "" {
		return fmt.Errorf("%w: missing Keygen account", ErrNotConfigured)
	}
	if strings.TrimSpace(keygen.Product) == "" {
		return fmt.Errorf("%w: missing Keygen product", ErrNotConfigured)
	}
	if strings.TrimSpace(keygen.LicenseKey) == "" {
		return fmt.Errorf("%w: missing license key or token", ErrNotConfigured)
	}
	return nil
}
