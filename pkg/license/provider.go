package license

import (
	"context"
	"encoding/json"
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
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

const (
	// LicenseKeyPropertyKey is the database property key used to persist the Keygen license key.
	LicenseKeyPropertyKey = "obot-license-key"

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

	// licenseStatePropertyPrefix is followed by the SHA-256 of a license key to make the property key of its state.
	licenseStatePropertyPrefix = "obot-license-state-"

	defaultPollInterval = 24 * time.Hour
	keygenProduct       = "18a762f2-5281-45cf-93fc-e45e2d932094"
	keygenAccount       = "7565373b-6069-4a0b-9495-9777d9db3fd9"
	keygenAPIURL        = "https://api.keygen.sh"
	keygenAPIPrefix     = "v1"
	keygenAPIVersion    = "1.8"
)

var (
	// keygenTimeout bounds one license check with Keygen, across all of its requests. Only the Keygen requests get
	// this deadline, so falling back to the stored state still has time when Keygen hangs.
	keygenTimeout = 15 * time.Second

	// ErrNotConfigured indicates license validation was requested without enough Keygen configuration.
	ErrNotConfigured = errors.New("license provider is not configured")

	// ErrLicenseKeyViaConfiguration indicates the license key is managed by startup configuration.
	ErrLicenseKeyViaConfiguration = errors.New("license key is configured at startup")

	// ErrInvalidLicense indicates the provided license key could not be validated.
	ErrInvalidLicense = errors.New("license key is invalid")

	// licenseRejectionCodes are the Keygen error codes that say the license itself is bad.
	licenseRejectionCodes = []keygen.ErrorCode{
		keygen.ErrorCodeLicenseInvalid,
		keygen.ErrorCodeLicenseExpired,
		keygen.ErrorCodeLicenseSuspended,
		keygen.ErrorCodeLicenseNotAllowed,
		keygen.ErrorCodeTokenInvalid,
		keygen.ErrorCodeTokenExpired,
		keygen.ErrorCodeTokenFormatInvalid,
		keygen.ErrorCodeTokenNotAllowed,
		keygen.ErrorCodeMachineLimitExceeded,
		keygen.ErrorCodeProcessLimitExceeded,
	}

	// licenseRejectionErrors are the rejections keygen-go returns as sentinels, without a *keygen.Error.
	licenseRejectionErrors = []error{
		ErrInvalidLicense,
		keygen.ErrLicenseExpired,
		keygen.ErrLicenseSuspended,
		keygen.ErrLicenseNotAllowed,
		keygen.ErrTokenInvalid,
		keygen.ErrTokenExpired,
		keygen.ErrTokenFormatInvalid,
		keygen.ErrTokenNotAllowed,
		keygen.ErrMachineLimitExceeded,
		keygen.ErrProcessLimitExceeded,
	}
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
	viaConfiguration bool
}

type validationRequest struct {
	fingerprint string
}

type keygenValidationResponse struct {
	License keygen.License
	Result  keygen.ValidationResult
}

func (s licenseKeySnapshot) equal(other licenseKeySnapshot) bool {
	return s.key == other.key &&
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

func (p *Provider) LicenseKeyViaConfiguration() bool {
	return p.configuredLicenseKey != ""
}

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

	property, err := p.gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return licenseKeySnapshot{}, nil
	}
	if err != nil {
		return licenseKeySnapshot{}, fmt.Errorf("failed to get license key property: %w", err)
	}

	return licenseKeySnapshot{
		key:       strings.TrimSpace(property.Value),
		updatedAt: property.UpdatedAt,
	}, nil
}

func (p *Provider) SetLicenseKey(ctx context.Context, licenseKey string) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	licenseKey = strings.TrimSpace(licenseKey)

	entitlements, err := p.validate(ctx, licenseKey)
	if isLicenseRejection(err) {
		return ErrInvalidLicense
	}
	if err != nil {
		return err
	}

	if p.gatewayClient == nil {
		return fmt.Errorf("failed to persist license key: gateway client is not configured")
	}

	// Serialize the persisted value and its cached representation with refresh.
	// Otherwise, an in-flight refresh of the previous key could overwrite this
	// state after the new key has been committed.
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	// Store the state first, so the key is never stored without it.
	if err := p.storeState(ctx, licenseKey, entitlements); err != nil {
		return err
	}
	property, err := p.gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, licenseKey)
	if err != nil {
		return err
	}
	p.setCachedState(licenseKeySnapshot{
		key:       licenseKey,
		updatedAt: property.UpdatedAt,
	}, entitlements)
	return nil
}

func (p *Provider) Validate(ctx context.Context) error {
	return p.refresh(ctx, true)
}

func (p *Provider) RemoveLicenseKey(ctx context.Context) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}

	// Keep deletion and cache invalidation ordered with refresh so an in-flight
	// validation cannot restore the state for the removed key.
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	if p.gatewayClient != nil {
		if err := p.gatewayClient.DeleteProperty(ctx, LicenseKeyPropertyKey); err != nil {
			return err
		}
	}

	p.setCachedState(licenseKeySnapshot{}, nil)
	return nil
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

// validate checks the license key with Keygen, activating this machine if needed, and returns its entitlements.
// Every failure is an error, and isLicenseRejection tells a rejected license apart from one that couldn't be checked.
func (p *Provider) validate(ctx context.Context, licenseKey string) (map[keygen.EntitlementCode]struct{}, error) {
	if strings.TrimSpace(licenseKey) == "" {
		return nil, ErrNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, keygenTimeout)
	defer cancel()

	keygenClient := p.keygenClient(licenseKey)
	lic := &keygen.License{}
	if _, err := keygenClient.Get(ctx, "me", nil, lic); err != nil {
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
				return nil, fmt.Errorf("license activation failed: %w", activationErr)
			}

			validation, err = p.validateLicense(ctx, keygenClient, lic)
			if err != nil {
				return nil, err
			}
		}
	}
	if !validation.Result.Valid {
		if code := string(validation.Result.Code); code == "" || strings.HasSuffix(code, "_SCOPE_REQUIRED") || strings.HasSuffix(code, "_SCOPE_EMPTY") {
			// The request was missing a scope or the response was empty, so Keygen didn't judge the license.
			return nil, fmt.Errorf("license validation was inconclusive: %q: %s", code, validation.Result.Detail)
		}
		return nil, fmt.Errorf("%w: %s: %s", ErrInvalidLicense, validation.Result.Code, validation.Result.Detail)
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

// isLicenseRejection reports whether Keygen answered that the license is bad.
// Anything else, like an outage, rate limit, or unexpected response, is an error checking the license.
func isLicenseRejection(err error) bool {
	if slices.ContainsFunc(licenseRejectionErrors, func(rejection error) bool {
		return errors.Is(err, rejection)
	}) {
		return true
	}

	// Other error documents only reject the license with a license code.
	keygenErr, ok := errors.AsType[*keygen.Error](err)
	return ok && slices.Contains(licenseRejectionCodes, keygen.ErrorCode(keygenErr.Code))
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
			if err := p.refresh(ctx, true); err != nil {
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

	var storeErr error
	entitlements, err := p.validate(ctx, snapshot.key)
	switch {
	case err == nil:
		storeErr = p.storeState(ctx, snapshot.key, entitlements)
	case isLicenseRejection(err):
		slog.Warn("license rejected", "error", err)
		storeErr = p.storeState(ctx, snapshot.key, nil)
	case ctx.Err() != nil:
		// The caller gave up, so nothing was learned about the license. Keep the cache, so the next refresh tries again.
		return ctx.Err()
	default:
		// Keygen didn't answer, so fall back to its latest stored answer for this key. If that can't be read, keep what
		// this pod last had for the key, if anything.
		stored, storedAt, ok := p.storedState(ctx, snapshot.key)
		if !ok {
			p.lock.RLock()
			if p.licenseKeySnapshot.key == snapshot.key {
				stored = p.entitlements
			}
			p.lock.RUnlock()
		}
		slog.Error("license check failed, using the last known result", "error", err, "storedAt", storedAt)
		entitlements = stored
	}
	if storeErr != nil {
		// Only the fallback for a later outage is lost, so keep the answer.
		slog.Error("failed to store license state", "error", storeErr)
	}
	p.setCachedState(snapshot, entitlements)
	return nil
}

func licenseStatePropertyKey(licenseKey string) string {
	return licenseStatePropertyPrefix + hash.String(licenseKey)
}

// storeState stores Keygen's latest answer for the license key, encrypted in the property table: its entitlements, or
// nil when Keygen rejected it. It's only read when Keygen can't be reached, so a pod that starts during an outage still
// has its license.
func (p *Provider) storeState(ctx context.Context, licenseKey string, entitlements map[keygen.EntitlementCode]struct{}) error {
	if p.gatewayClient == nil {
		return nil
	}

	value, err := json.Marshal(entitlements)
	if err != nil {
		return fmt.Errorf("failed to encode license state: %w", err)
	}
	if _, err := p.gatewayClient.SetProperty(ctx, licenseStatePropertyKey(licenseKey), string(value)); err != nil {
		return fmt.Errorf("failed to store license state: %w", err)
	}
	return nil
}

// storedState returns Keygen's latest stored answer for the license key, when it was stored, and whether one could be
// read. The entitlements are nil when Keygen rejected the key.
func (p *Provider) storedState(ctx context.Context, licenseKey string) (map[keygen.EntitlementCode]struct{}, time.Time, bool) {
	if p.gatewayClient == nil {
		return nil, time.Time{}, false
	}

	property, err := p.gatewayClient.GetProperty(ctx, licenseStatePropertyKey(licenseKey))
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("failed to load stored license state", "error", err)
		}
		return nil, time.Time{}, false
	}

	var entitlements map[keygen.EntitlementCode]struct{}
	if err := json.Unmarshal([]byte(property.Value), &entitlements); err != nil {
		slog.Error("ignoring unreadable stored license state", "error", err)
		return nil, time.Time{}, false
	}
	return entitlements, property.UpdatedAt, true
}
