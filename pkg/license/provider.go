package license

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"gorm.io/gorm"
)

const (
	// LicenseKeyPropertyKey is the database property key used to persist the Keygen license key.
	LicenseKeyPropertyKey = "obot-license-key"

	// CommunityLicenseKeyPropertyKey is the database property key used to preserve a Community license
	// while a separately managed license is active.
	CommunityLicenseKeyPropertyKey = "obot-community-license-key"

	// LicenseMachineIDPropertyKey is the database property key used to persist the Keygen machine fingerprint.
	LicenseMachineIDPropertyKey = "obot-license-machine-id"

	// EnterpriseAuthProvidersEntitlement is required to enable enterprise auth providers.
	EnterpriseAuthProvidersEntitlement = "OBOT_ENTERPRISE_AUTH_PROVIDERS"

	// EnterpriseEntitlement is required to enable enterprise edition.
	EnterpriseEntitlement = "OBOT_ENTERPRISE"

	// CommunityEntitlement is required to enable community edition.
	CommunityEntitlement = "OBOT_COMMUNITY"

	// CloudEntitlement identifies Obot Cloud deployments.
	CloudEntitlement = "OBOT_CLOUD"

	// EnterpriseModelProvidersEntitlement is required to enable enterprise model providers.
	EnterpriseModelProvidersEntitlement = "OBOT_ENTERPRISE_MODEL_PROVIDERS"

	defaultPollInterval = 24 * time.Hour
	keygenProduct       = "18a762f2-5281-45cf-93fc-e45e2d932094"
	keygenAccount       = "7565373b-6069-4a0b-9495-9777d9db3fd9"
	keygenAPIURL        = "https://api.keygen.sh"
	keygenAPIPrefix     = "v1"
	keygenAPIVersion    = "1.8"
)

var (
	// ErrNotConfigured indicates license validation was requested without enough Keygen configuration.
	ErrNotConfigured = errors.New("license provider is not configured")

	// ErrLicenseKeyViaConfiguration indicates the license key is managed by startup configuration.
	ErrLicenseKeyViaConfiguration = errors.New("license key is configured at startup")

	// ErrInvalidLicense indicates the provided license key could not be validated.
	ErrInvalidLicense = errors.New("license key is invalid")

	errLicenseKeyChanged = errors.New("license key changed while it was being updated")
)

// Config contains the Keygen settings needed to validate an Obot license.
type Config struct {
	LicenseKey string `usage:"Keygen license key for this Obot installation"`
}

type Provider struct {
	lock                 sync.RWMutex
	refreshLock          sync.Mutex
	entitlements         map[keygen.EntitlementCode]struct{}
	licenseKeySnapshot   licenseKeySnapshot
	machineFingerprint   string
	gatewayClient        *client.Client
	configuredLicenseKey string
	keygenAPIURL         string
}

type licenseKeySnapshot struct {
	key              string
	updatedAt        time.Time
	propertyKey      string
	viaConfiguration bool
}

type validationRequest struct {
	fingerprint string
}

type keygenValidationResponse struct {
	License keygen.License
	Result  keygen.ValidationResult
}

// equal includes the backing property because moving the same key between the
// primary and Community properties must invalidate the cached license state.
func (s licenseKeySnapshot) equal(other licenseKeySnapshot) bool {
	return s.key == other.key &&
		s.propertyKey == other.propertyKey &&
		s.viaConfiguration == other.viaConfiguration &&
		s.updatedAt.Equal(other.updatedAt)
}

// NewProvider creates a Keygen-backed license provider.
func NewProvider(ctx context.Context, gatewayClient *client.Client, config Config) (*Provider, error) {
	return newProvider(ctx, gatewayClient, config, keygenAPIURL)
}

func newProvider(ctx context.Context, gatewayClient *client.Client, config Config, apiURL string) (*Provider, error) {
	machineFingerprint, err := ensureMachineFingerprint(ctx, gatewayClient)
	if err != nil {
		return nil, err
	}

	if apiURL == "" {
		apiURL = keygenAPIURL
	}

	k := &Provider{
		machineFingerprint:   machineFingerprint,
		gatewayClient:        gatewayClient,
		configuredLicenseKey: strings.TrimSpace(config.LicenseKey),
		keygenAPIURL:         apiURL,
	}

	if err := k.refresh(ctx, true); err != nil {
		slog.Warn("initial license refresh failed", "error", err)
	}

	if k.entitlements != nil {
		slog.Info("license provider initialized", "entitlements", k.entitlements)
	}

	go k.poll(ctx)

	return k, nil
}

func ensureMachineFingerprint(ctx context.Context, gatewayClient *client.Client) (string, error) {
	if gatewayClient == nil {
		return uuid.New().String(), nil
	}

	property, err := gatewayClient.GetOrCreateProperty(ctx, LicenseMachineIDPropertyKey, uuid.New().String())
	if err != nil {
		return "", fmt.Errorf("failed to ensure license machine ID: %w", err)
	}
	return property.Value, nil
}

func (p *Provider) LicenseKey(ctx context.Context) (string, error) {
	snapshot, err := p.loadLicenseKey(ctx)
	if err != nil {
		return "", err
	}
	return snapshot.key, nil
}

// MachineFingerprint returns the existing persisted installation identity.
func (p *Provider) MachineFingerprint() string {
	return p.machineFingerprint
}

// PrimaryLicenseKeyExists reports whether a database-managed primary license
// prevents Community enrollment, regardless of whether that key is valid.
func (p *Provider) PrimaryLicenseKeyExists(ctx context.Context) (bool, error) {
	snapshot, err := p.loadPrimaryLicenseKey(ctx)
	return snapshot.propertyKey != "", err
}

func (p *Provider) loadPrimaryLicenseKey(ctx context.Context) (licenseKeySnapshot, error) {
	if p.gatewayClient == nil {
		return licenseKeySnapshot{}, nil
	}
	property, err := p.gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return licenseKeySnapshot{}, nil
		}
		return licenseKeySnapshot{}, fmt.Errorf("failed to get primary license key property: %w", err)
	}
	return licenseKeySnapshot{
		key:         strings.TrimSpace(property.Value),
		updatedAt:   property.UpdatedAt,
		propertyKey: LicenseKeyPropertyKey,
	}, nil
}

func (p *Provider) LicenseKeyViaConfiguration() bool {
	return p.configuredLicenseKey != ""
}

// loadLicenseKey returns the effective license key. A configured key has the
// highest priority, followed by the primary database key. The Community key is
// kept in a separate property so it can remain dormant while a primary license
// is active and become effective again when that primary key is removed.
func (p *Provider) loadLicenseKey(ctx context.Context) (licenseKeySnapshot, error) {
	if p.configuredLicenseKey != "" {
		return licenseKeySnapshot{
			key:              p.configuredLicenseKey,
			viaConfiguration: true,
		}, nil
	}
	if p.gatewayClient == nil {
		return licenseKeySnapshot{}, nil
	}

	for _, propertyKey := range []string{LicenseKeyPropertyKey, CommunityLicenseKeyPropertyKey} {
		property, err := p.gatewayClient.GetProperty(ctx, propertyKey)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return licenseKeySnapshot{}, fmt.Errorf("failed to get license key property %q: %w", propertyKey, err)
		}

		return licenseKeySnapshot{
			key:         strings.TrimSpace(property.Value),
			updatedAt:   property.UpdatedAt,
			propertyKey: propertyKey,
		}, nil
	}

	return licenseKeySnapshot{}, nil
}

// SetLicenseKey validates and stores the primary database license. If it
// replaces a legacy Community key in the primary property, that key is copied
// to the Community property first so removing the new license can restore it.
func (p *Provider) SetLicenseKey(ctx context.Context, licenseKey string) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	licenseKey = strings.TrimSpace(licenseKey)

	entitlements, err := p.validate(ctx, licenseKey)
	if err != nil {
		return err
	}
	if entitlements == nil {
		return ErrInvalidLicense
	}

	if p.gatewayClient == nil {
		return fmt.Errorf("failed to persist license key: gateway client is not configured")
	}

	// Serialize the persisted value and its cached representation with refresh.
	// Otherwise, an in-flight refresh of the previous key could overwrite this
	// state after the new key has been committed.
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	// Community licenses historically lived in the primary property. Preserve
	// one there as a fallback before overwriting it so existing installations
	// gain the new fallback behavior without a separate data migration.
	currentSnapshot, err := p.loadLicenseKey(ctx)
	if err != nil {
		return err
	}
	preserveCommunity := false
	if currentSnapshot.propertyKey == LicenseKeyPropertyKey {
		community, err := p.isCommunityLicense(ctx, currentSnapshot)
		if err != nil {
			return err
		}
		preserveCommunity = community
	}

	var updatedAt time.Time
	err = p.gatewayClient.Transaction(ctx, func(tx *gorm.DB) error {
		var expectedPrimaryVersion *time.Time
		if currentSnapshot.propertyKey == LicenseKeyPropertyKey {
			expectedPrimaryVersion = &currentSnapshot.updatedAt
		}
		matches, err := p.gatewayClient.PropertyVersionMatchesTx(tx, LicenseKeyPropertyKey, expectedPrimaryVersion)
		if err != nil {
			return err
		}
		if !matches {
			return errLicenseKeyChanged
		}

		if preserveCommunity {
			if _, err := p.gatewayClient.SetPropertyTx(ctx, tx, CommunityLicenseKeyPropertyKey, currentSnapshot.key); err != nil {
				return fmt.Errorf("failed to preserve Community license key: %w", err)
			}
		}
		property, err := p.gatewayClient.SetPropertyTx(ctx, tx, LicenseKeyPropertyKey, licenseKey)
		if err != nil {
			return err
		}
		updatedAt = property.UpdatedAt
		return nil
	})
	if err != nil {
		return err
	}

	p.setCachedState(licenseKeySnapshot{
		key:         licenseKey,
		updatedAt:   updatedAt,
		propertyKey: LicenseKeyPropertyKey,
	}, entitlements)
	return nil
}

// SetCommunityLicenseKey validates and stores an issued Community key in its
// dedicated property. A concurrently installed primary remains effective and
// the Community key becomes its fallback.
func (p *Provider) SetCommunityLicenseKey(ctx context.Context, licenseKey string) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	licenseKey = strings.TrimSpace(licenseKey)

	entitlements, err := p.validate(ctx, licenseKey)
	if err != nil {
		return err
	}
	if !isCommunityEntitlements(entitlements) {
		return ErrInvalidLicense
	}
	if p.gatewayClient == nil {
		return fmt.Errorf("failed to persist license key: gateway client is not configured")
	}

	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	// Do not cache this key here because a primary license installed by another
	// replica may make the Community property dormant. The next normal refresh
	// resolves and caches whichever property is actually effective.
	if _, err := p.gatewayClient.SetProperty(ctx, CommunityLicenseKeyPropertyKey, licenseKey); err != nil {
		return err
	}
	return nil
}

func (p *Provider) Validate(ctx context.Context) error {
	return p.refresh(ctx, true)
}

// RemoveLicenseKey removes the primary database license. A preserved Community
// property becomes effective afterward and is not affected by repeated calls.
func (p *Provider) RemoveLicenseKey(ctx context.Context) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}

	// Keep deletion and cache invalidation ordered with refresh so an in-flight
	// validation cannot restore the state for the removed key.
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	if err := p.gatewayClient.DeleteProperty(ctx, LicenseKeyPropertyKey); err != nil {
		return err
	}
	p.setCachedState(licenseKeySnapshot{}, nil)
	return nil
}

// isCommunityLicense determines whether a primary key should be retained in
// the Community fallback property. It uses cached entitlements only when they
// belong to the same property snapshot, which matters when another replica has
// changed the effective database key.
func (p *Provider) isCommunityLicense(ctx context.Context, snapshot licenseKeySnapshot) (bool, error) {
	p.lock.RLock()
	if p.licenseKeySnapshot.equal(snapshot) && p.entitlements != nil {
		community := isCommunityEntitlements(p.entitlements)
		p.lock.RUnlock()
		return community, nil
	}
	p.lock.RUnlock()

	entitlements, err := p.validate(ctx, snapshot.key)
	if err != nil {
		return false, err
	}
	return isCommunityEntitlements(entitlements), nil
}

// isCommunityEntitlements excludes Enterprise and Cloud licenses because they
// may also carry the Community entitlement but must never replace the fallback.
func isCommunityEntitlements(entitlements map[keygen.EntitlementCode]struct{}) bool {
	_, community := entitlements[CommunityEntitlement]
	_, enterprise := entitlements[EnterpriseEntitlement]
	_, cloud := entitlements[CloudEntitlement]
	return community && !enterprise && !cloud
}

func (p *Provider) keygenClient(licenseKey string) *keygen.Client {
	keygenClient := keygen.NewClientWithOptions(&keygen.ClientOptions{
		Account:    keygenAccount,
		LicenseKey: licenseKey,
		APIVersion: keygenAPIVersion,
		APIPrefix:  keygenAPIPrefix,
		APIURL:     p.keygenAPIURL,
	})
	// Avoid Keygen's package-level HTTP client. The transport remains shared and
	// safe for concurrent use, while redirect policy is scoped to this client.
	keygenClient.HTTPClient = &http.Client{Transport: http.DefaultTransport}
	return keygenClient
}

func (v validationRequest) GetMeta() any {
	return struct {
		Scope struct {
			Fingerprint string `json:"fingerprint,omitempty"`
			Product     string `json:"product"`
		} `json:"scope"`
	}{
		Scope: struct {
			Fingerprint string `json:"fingerprint,omitempty"`
			Product     string `json:"product"`
		}{
			Fingerprint: v.fingerprint,
			Product:     keygenProduct,
		},
	}
}

func (v *keygenValidationResponse) SetData(to func(target any) error) error {
	return to(&v.License)
}

func (v *keygenValidationResponse) SetMeta(to func(target any) error) error {
	return to(&v.Result)
}

func (p *Provider) validate(ctx context.Context, licenseKey string) (map[keygen.EntitlementCode]struct{}, error) {
	if strings.TrimSpace(licenseKey) == "" {
		return nil, ErrNotConfigured
	}

	keygenClient := p.keygenClient(licenseKey)
	lic := &keygen.License{}
	if _, err := keygenClient.Get(ctx, "me", nil, lic); err != nil {
		slog.Warn("license lookup failed", "error", err)
		if isDefinitiveLicenseRejection(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("license lookup failed: %w", err)
	}

	validation, err := p.validateLicense(ctx, keygenClient, lic)
	if err != nil {
		return nil, err
	}
	if !validation.Result.Valid {
		if validation.Result.Code == keygen.ValidationCodeFingerprintScopeMismatch ||
			validation.Result.Code == keygen.ValidationCodeNoMachines ||
			validation.Result.Code == keygen.ValidationCodeNoMachine {
			machine := &keygen.Machine{
				Fingerprint: p.machineFingerprint,
				LicenseID:   lic.ID,
			}
			machine.Hostname, _ = os.Hostname()
			machine.Platform = runtime.GOOS + "/" + runtime.GOARCH
			machine.Cores = runtime.NumCPU()
			if _, activationErr := keygenClient.Post(ctx, "machines", machine, &keygen.Machine{}); activationErr != nil &&
				!errors.Is(activationErr, keygen.ErrMachineAlreadyActivated) {
				slog.Warn("license activation failed", "error", activationErr)
				if isDefinitiveLicenseRejection(activationErr) {
					return nil, nil
				}
				return nil, fmt.Errorf("license activation failed: %w", activationErr)
			}

			validation, err = p.validateLicense(ctx, keygenClient, lic)
			if err != nil {
				return nil, err
			}
		}
	}
	if !validation.Result.Valid {
		slog.Warn("license validation failed", "code", validation.Result.Code, "detail", validation.Result.Detail)
		return nil, nil
	}

	entitlements := keygen.Entitlements{}
	if _, err := keygenClient.Get(ctx, fmt.Sprintf("licenses/%s/entitlements?limit=100", lic.ID), nil, &entitlements); err != nil {
		return nil, fmt.Errorf("list license entitlements: %w", err)
	}

	entitlementSet := make(map[keygen.EntitlementCode]struct{}, len(entitlements))
	for _, entitlement := range entitlements {
		entitlementSet[entitlement.Code] = struct{}{}
	}

	return entitlementSet, nil
}

func isDefinitiveLicenseRejection(err error) bool {
	if errors.Is(err, keygen.ErrTokenNotAllowed) ||
		errors.Is(err, keygen.ErrTokenFormatInvalid) ||
		errors.Is(err, keygen.ErrTokenInvalid) ||
		errors.Is(err, keygen.ErrTokenExpired) ||
		errors.Is(err, keygen.ErrLicenseKeyMissing) ||
		errors.Is(err, keygen.ErrLicenseKeyNotGenuine) ||
		errors.Is(err, keygen.ErrLicenseInvalid) ||
		errors.Is(err, keygen.ErrLicenseNotAllowed) ||
		errors.Is(err, keygen.ErrLicenseExpired) ||
		errors.Is(err, keygen.ErrLicenseSuspended) ||
		errors.Is(err, keygen.ErrMachineLimitExceeded) {
		return true
	}

	var licenseKeyErr *keygen.LicenseKeyError
	var licenseTokenErr *keygen.LicenseTokenError
	var notAuthorizedErr *keygen.NotAuthorizedError
	var notFoundErr *keygen.NotFoundError
	return errors.As(err, &licenseKeyErr) ||
		errors.As(err, &licenseTokenErr) ||
		errors.As(err, &notAuthorizedErr) ||
		errors.As(err, &notFoundErr)
}

func (p *Provider) validateLicense(ctx context.Context, keygenClient *keygen.Client, lic *keygen.License) (*keygenValidationResponse, error) {
	validation := &keygenValidationResponse{}
	if _, err := keygenClient.Post(ctx, "licenses/"+lic.ID+"/actions/validate", validationRequest{
		fingerprint: p.machineFingerprint,
	}, validation); err != nil {
		return validation, fmt.Errorf("validate license failed: %w", err)
	}
	*lic = validation.License
	return validation, nil
}

func (p *Provider) HasValidLicense(ctx context.Context) (bool, error) {
	if err := p.refresh(ctx, false); err != nil {
		return false, err
	}
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.entitlements != nil, nil
}

func (p *Provider) Entitlements(ctx context.Context) ([]string, error) {
	if err := p.refresh(ctx, false); err != nil {
		return nil, err
	}
	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.entitlements == nil {
		return nil, nil
	}

	entitlements := make([]string, 0, len(p.entitlements))
	for entitlement := range p.entitlements {
		entitlements = append(entitlements, string(entitlement))
	}

	slices.Sort(entitlements)

	return entitlements, nil
}

func (p *Provider) hasEntitlement(key string) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, ok := p.entitlements[keygen.EntitlementCode(key)]
	return ok
}

func (p *Provider) poll(ctx context.Context) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.update(ctx); err != nil {
				slog.Warn("license update failed", "error", err)
			} else {
				p.lock.RLock()
				entitlements := p.entitlements
				p.lock.RUnlock()

				slog.Info("license updated successfully", "entitlements", entitlements)
			}
		}
	}
}

func (p *Provider) update(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return p.refresh(ctx, true)
}

func (p *Provider) cachedSnapshotMatches(snapshot licenseKeySnapshot) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.licenseKeySnapshot.equal(snapshot)
}

func (p *Provider) setCachedState(snapshot licenseKeySnapshot, entitlements map[keygen.EntitlementCode]struct{}) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.licenseKeySnapshot = snapshot
	p.entitlements = entitlements
}

func (p *Provider) refresh(ctx context.Context, force bool) error {
	snapshot, err := p.loadLicenseKey(ctx)
	if err != nil {
		return err
	}
	if !force && p.cachedSnapshotMatches(snapshot) {
		return nil
	}

	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	// Another request may have refreshed the provider while this request waited.
	snapshot, err = p.loadLicenseKey(ctx)
	if err != nil {
		return err
	}
	if !force && p.cachedSnapshotMatches(snapshot) {
		return nil
	}

	if snapshot.key == "" {
		p.setCachedState(snapshot, nil)
		return nil
	}

	entitlements, err := p.validate(ctx, snapshot.key)
	if err != nil {
		return err
	}
	p.setCachedState(snapshot, entitlements)
	return nil
}
