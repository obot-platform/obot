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

	// licenseStatePropertyPrefix is followed by a hash of a license key to make the property key that stores the license
	// state Keygen last accepted for it.
	licenseStatePropertyPrefix = "obot-license-state-"

	defaultPollInterval = 24 * time.Hour
	keygenCheckTimeout  = 15 * time.Second
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

	// ErrInvalidLicense indicates Keygen rejected the provided license key.
	ErrInvalidLicense = errors.New("license key is invalid")

	// ErrLicenseCheckFailed indicates Keygen didn't give a verdict on a license key.
	ErrLicenseCheckFailed = errors.New("failed to check license key")
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

type licenseState struct {
	Entitlements map[keygen.EntitlementCode]struct{} `json:"entitlements"`
	Expiry       *time.Time                          `json:"expiry,omitempty"`
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

// SetLicenseKey stores the license key after Keygen accepts it.
// It returns ErrInvalidLicense when Keygen rejects the key and ErrLicenseCheckFailed when Keygen didn't give a verdict.
func (p *Provider) SetLicenseKey(ctx context.Context, licenseKey string) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	licenseKey = strings.TrimSpace(licenseKey)

	state, err := p.validate(ctx, licenseKey)
	if err != nil {
		slog.Error("license check failed", "error", err)
		return err
	}
	if state.Entitlements == nil {
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

	// Save the state before the key, so any replica that sees the new key has it to fall back on.
	if err := p.saveLicenseState(ctx, licenseKey, state); err != nil {
		return err
	}
	property, err := p.gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, licenseKey)
	if err != nil {
		return err
	}
	p.setCachedState(licenseKeySnapshot{
		key:       licenseKey,
		updatedAt: property.UpdatedAt,
	}, state.Entitlements)
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

// validate checks the license key with Keygen, activating this machine if needed, and returns the license state.
// It returns a state without entitlements when Keygen rejects the license, and an ErrLicenseCheckFailed error when
// Keygen didn't give a verdict.
func (p *Provider) validate(ctx context.Context, licenseKey string) (state licenseState, err error) {
	if strings.TrimSpace(licenseKey) == "" {
		return licenseState{}, ErrNotConfigured
	}

	// Bound the Keygen calls, so a slow Keygen is a failure to check rather than holding up the caller.
	ctx, cancel := context.WithTimeout(ctx, keygenCheckTimeout)
	defer cancel()

	// Keygen rejecting the license is a verdict, so return no entitlements rather than an error. Anything else means
	// Keygen didn't give a verdict.
	defer func() {
		if isLicenseRejection(err) {
			slog.Warn("license rejected", "error", err)
			state, err = licenseState{}, nil
		} else if err != nil {
			err = fmt.Errorf("%w: %w", ErrLicenseCheckFailed, err)
		}
	}()

	keygenClient := p.keygenClient(licenseKey)
	lic := &keygen.License{}
	if _, err := keygenClient.Get(ctx, "me", nil, lic); err != nil {
		return licenseState{}, fmt.Errorf("look up license: %w", err)
	}

	validation, err := p.validateLicense(ctx, keygenClient, lic)
	if err != nil {
		return licenseState{}, err
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
				return licenseState{}, fmt.Errorf("activate license: %w", activationErr)
			}

			validation, err = p.validateLicense(ctx, keygenClient, lic)
			if err != nil {
				return licenseState{}, err
			}
		}
	}
	if !validation.Result.Valid {
		slog.Warn("license validation failed", "code", validation.Result.Code, "detail", validation.Result.Detail)
		return licenseState{}, nil
	}

	entitlements := keygen.Entitlements{}
	if _, err := keygenClient.Get(ctx, fmt.Sprintf("licenses/%s/entitlements?limit=100", lic.ID), nil, &entitlements); err != nil {
		return licenseState{}, fmt.Errorf("list license entitlements: %w", err)
	}

	state = licenseState{
		Entitlements: make(map[keygen.EntitlementCode]struct{}, len(entitlements)),
		Expiry:       lic.Expiry,
	}
	for _, entitlement := range entitlements {
		state.Entitlements[entitlement.Code] = struct{}{}
	}

	return state, nil
}

// loadLicenseState returns the license state Keygen last accepted for the license key, unless the license has expired
// since.
func (p *Provider) loadLicenseState(ctx context.Context, licenseKey string) (licenseState, error) {
	if p.gatewayClient == nil {
		return licenseState{}, nil
	}

	property, err := p.gatewayClient.GetProperty(ctx, licenseStatePropertyKey(licenseKey))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return licenseState{}, nil
	}
	if err != nil {
		return licenseState{}, fmt.Errorf("failed to load license state: %w", err)
	}

	var state licenseState
	if err := json.Unmarshal([]byte(property.Value), &state); err != nil {
		return licenseState{}, fmt.Errorf("failed to decode license state: %w", err)
	}
	if state.Expiry != nil && !time.Now().Before(*state.Expiry) {
		slog.Warn("saved license has expired", "expiry", state.Expiry)
		return licenseState{}, nil
	}
	return state, nil
}

// saveLicenseState saves the license state Keygen accepted for the license key, encrypted in the property table, or
// deletes it when Keygen rejected the key.
func (p *Provider) saveLicenseState(ctx context.Context, licenseKey string, state licenseState) error {
	if p.gatewayClient == nil {
		return nil
	}

	if state.Entitlements == nil {
		return p.gatewayClient.DeleteProperty(ctx, licenseStatePropertyKey(licenseKey))
	}

	value, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to encode license state: %w", err)
	}
	_, err = p.gatewayClient.SetProperty(ctx, licenseStatePropertyKey(licenseKey), string(value))
	return err
}

// licenseStatePropertyKey returns the property key for the license key's state.
// The license key is hashed since property keys aren't encrypted.
func licenseStatePropertyKey(licenseKey string) string {
	return licenseStatePropertyPrefix + hash.String(licenseKey)
}

// isLicenseRejection reports whether a Keygen error rejects the license:
//   - License key authentication, which every request goes through, failed with LICENSE_INVALID, LICENSE_BANNED,
//     LICENSE_SUSPENDED, LICENSE_EXPIRED, or LICENSE_NOT_ALLOWED
//   - Machine activation was refused because the license's machine, core, or group machine limit is used up
//
// Everything else is a failure to check. That includes Keygen API errors about Obot's Keygen account rather than the
// license, e.g. a 403 for an inactive subscription or a 402 for the daily request limit.
func isLicenseRejection(err error) bool {
	// keygen-go returns these as sentinels, without the underlying *keygen.Error.
	if errors.Is(err, keygen.ErrLicenseSuspended) ||
		errors.Is(err, keygen.ErrLicenseExpired) ||
		errors.Is(err, keygen.ErrLicenseNotAllowed) ||
		errors.Is(err, keygen.ErrMachineLimitExceeded) {
		return true
	}

	// The rest come back as a *keygen.Error, and keygen-go has no constants for most of their codes.
	keygenErr, ok := errors.AsType[*keygen.Error](err)
	if !ok {
		return false
	}

	switch keygen.ErrorCode(keygenErr.Code) {
	case keygen.ErrorCodeLicenseInvalid,
		"LICENSE_BANNED",
		"MACHINE_CORE_LIMIT_EXCEEDED",
		"GROUP_MACHINE_LIMIT_EXCEEDED":
		return true
	default:
		return false
	}
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

// poll checks the license key with Keygen once a day.
func (p *Provider) poll(ctx context.Context) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Validate(ctx); err != nil {
				slog.Warn("license update failed", "error", err)
			} else {
				p.lock.RLock()
				entitlements := p.entitlements
				p.lock.RUnlock()

				slog.Info("license update finished", "entitlements", entitlements)
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

// refresh checks the license key with Keygen and caches its entitlements, unless force is false and the key hasn't
// changed since the last check.
// When Keygen doesn't give a verdict, it falls back to the entitlements Keygen last accepted for the key, unless the
// license has expired since, and a forced check also returns the ErrLicenseCheckFailed error.
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

	state, err := p.validate(ctx, snapshot.key)
	checkErr := err
	if checkErr != nil {
		// Keygen didn't give a verdict, fall back to the state it last accepted for the key, if there is one.
		state, err = p.loadLicenseState(ctx, snapshot.key)
		slog.Error("license check failed", "error", checkErr, "usingSavedEntitlements", state.Entitlements != nil)
	} else if err := p.saveLicenseState(ctx, snapshot.key, state); err != nil {
		// Keygen gave a verdict, use it even if it can't be saved for later checks.
		slog.Error("failed to save license state", "error", err)
	}
	if err != nil {
		// Couldn't load the saved state, keep the cached state so the next check tries again.
		return err
	}

	p.setCachedState(snapshot, state.Entitlements)
	if force {
		// The license keeps working, but an explicit check should report that Keygen didn't answer.
		return checkErr
	}
	return nil
}
