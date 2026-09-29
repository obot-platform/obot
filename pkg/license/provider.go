package license

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	// LicenseKeyPropertyKey is the database property key used to persist the Keygen license key.
	LicenseKeyPropertyKey = "obot-license-key"

	// LicenseMachineFingerprintPropertyKey is the database property key used to persist the Keygen
	// machine fingerprint. The stored key predates this name and must not change.
	LicenseMachineFingerprintPropertyKey = "obot-license-machine-id"

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

	// keygenRequestTimeout bounds license validation and machine lookups.
	keygenRequestTimeout = 15 * time.Second

	// machineLookupBackoff spaces out machine lookups after one fails, so model proxy
	// requests cannot repeatedly call Keygen while it is unavailable.
	machineLookupBackoff = 30 * time.Second
)

var (
	// ErrNotConfigured indicates license validation was requested without enough Keygen configuration.
	ErrNotConfigured = errors.New("license provider is not configured")

	// ErrLicenseKeyViaConfiguration indicates the license key is managed by startup configuration.
	ErrLicenseKeyViaConfiguration = errors.New("license key is configured at startup")

	// ErrInvalidLicense indicates the provided license key could not be validated.
	ErrInvalidLicense = errors.New("license key is invalid")

	errMachineLookupBackoff = errors.New("license machine lookup failed recently; try again soon")
)

// Config contains the Keygen settings needed to validate an Obot license.
type Config struct {
	LicenseKey string `usage:"Keygen license key for this Obot installation"`
}

type Provider struct {
	lock                 sync.RWMutex
	refreshLock          sync.Mutex
	machineLookups       singleflight.Group
	entitlements         map[keygen.EntitlementCode]struct{}
	machineID            string
	machineLookupAfter   time.Time
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

	property, err := gatewayClient.GetOrCreateProperty(ctx, LicenseMachineFingerprintPropertyKey, uuid.New().String())
	if err != nil {
		return "", fmt.Errorf("failed to ensure license machine fingerprint: %w", err)
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

// MachineID returns the Keygen machine activated for this installation's
// fingerprint, looking it up with the license key the first time it is needed.
// It is empty when there is no valid license. Concurrent callers share one
// lookup, which holds no provider lock, and a failed lookup is not retried for
// machineLookupBackoff.
func (p *Provider) MachineID(ctx context.Context) (string, error) {
	if err := p.refresh(ctx, false); err != nil {
		return "", err
	}

	p.lock.RLock()
	machineID, valid, snapshot, retryAt := p.machineID, p.entitlements != nil, p.licenseKeySnapshot, p.machineLookupAfter
	p.lock.RUnlock()

	if machineID != "" || !valid {
		return machineID, nil
	}

	if time.Now().Before(retryAt) {
		return "", errMachineLookupBackoff
	}

	// The shared lookup outlives any one caller, so a cancelled caller cannot fail the others.
	lookup := p.machineLookups.DoChan(snapshot.key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), keygenRequestTimeout)
		defer cancel()

		return p.resolveMachineID(ctx, snapshot)
	})

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-lookup:
		if result.Err != nil {
			return "", result.Err
		}

		return result.Val.(string), nil
	}
}

// resolveMachineID looks up the machine for snapshot's license and caches the
// result, unless the cached license changed while Keygen was being asked.
func (p *Provider) resolveMachineID(ctx context.Context, snapshot licenseKeySnapshot) (string, error) {
	machineID, lookupErr := p.lookupMachineID(ctx, p.keygenClient(snapshot.key))

	p.lock.Lock()
	defer p.lock.Unlock()
	if p.entitlements == nil || !p.licenseKeySnapshot.equal(snapshot) {
		return "", errors.New("license changed while resolving its machine ID")
	}

	if lookupErr != nil {
		p.machineLookupAfter = time.Now().Add(machineLookupBackoff)
		return "", lookupErr
	}

	// A refresh during the lookup may already have cached an activated machine.
	if p.machineID == "" {
		p.machineID = machineID
	}

	return p.machineID, nil
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

	entitlements, machineID, err := p.validate(ctx, licenseKey)
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

	property, err := p.gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, licenseKey)
	if err != nil {
		return err
	}
	p.setCachedState(licenseKeySnapshot{
		key:       licenseKey,
		updatedAt: property.UpdatedAt,
	}, entitlements, machineID)
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

	p.setCachedState(licenseKeySnapshot{}, nil, "")
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

// validate returns the license entitlements, or nil when the license is invalid, and the
// Keygen machine ID for this installation when validation just activated it. Otherwise
// MachineID looks the machine up only when the model proxy needs it.
func (p *Provider) validate(ctx context.Context, licenseKey string) (map[keygen.EntitlementCode]struct{}, string, error) {
	if strings.TrimSpace(licenseKey) == "" {
		return nil, "", ErrNotConfigured
	}

	keygenClient := p.keygenClient(licenseKey)
	lic := &keygen.License{}
	if _, err := keygenClient.Get(ctx, "me", nil, lic); err != nil {
		slog.Warn("license lookup failed", "error", err)
		return nil, "", nil
	}

	validation, err := p.validateLicense(ctx, keygenClient, lic)
	if err != nil {
		return nil, "", err
	}

	var machineID string
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
			activated := &keygen.Machine{}
			_, activationErr := keygenClient.Post(ctx, "machines", machine, activated)
			switch {
			case activationErr == nil:
				// MachineID looks the machine up if the activation response is unusable.
				machineID, _ = p.verifiedMachineID(activated)
			case !errors.Is(activationErr, keygen.ErrMachineAlreadyActivated):
				slog.Warn("license activation failed", "error", activationErr)
				return nil, "", nil
			}

			validation, err = p.validateLicense(ctx, keygenClient, lic)
			if err != nil {
				return nil, "", err
			}
		}
	}
	if !validation.Result.Valid {
		slog.Warn("license validation failed", "code", validation.Result.Code, "detail", validation.Result.Detail)
		return nil, "", nil
	}

	entitlements := keygen.Entitlements{}
	if _, err := keygenClient.Get(ctx, fmt.Sprintf("licenses/%s/entitlements?limit=100", lic.ID), nil, &entitlements); err != nil {
		return nil, "", fmt.Errorf("list license entitlements: %w", err)
	}

	entitlementSet := make(map[keygen.EntitlementCode]struct{}, len(entitlements))
	for _, entitlement := range entitlements {
		entitlementSet[entitlement.Code] = struct{}{}
	}

	return entitlementSet, machineID, nil
}

// lookupMachineID resolves this installation's machine by fingerprint, which
// Keygen accepts in place of the machine ID.
func (p *Provider) lookupMachineID(ctx context.Context, keygenClient *keygen.Client) (string, error) {
	machine := &keygen.Machine{}
	if _, err := keygenClient.Get(ctx, "machines/"+url.PathEscape(p.machineFingerprint), nil, machine); err != nil {
		return "", fmt.Errorf("look up license machine: %w", err)
	}

	return p.verifiedMachineID(machine)
}

func (p *Provider) verifiedMachineID(machine *keygen.Machine) (string, error) {
	if machine.Fingerprint != p.machineFingerprint {
		return "", errors.New("license machine does not match this installation's fingerprint")
	}

	id, err := uuid.Parse(machine.ID)
	if err != nil {
		return "", errors.New("license machine ID is not a UUID")
	}

	return id.String(), nil
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
	ctx, cancel := context.WithTimeout(ctx, keygenRequestTimeout)
	defer cancel()
	return p.refresh(ctx, true)
}

func (p *Provider) cachedSnapshotMatches(snapshot licenseKeySnapshot) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.licenseKeySnapshot.equal(snapshot)
}

func (p *Provider) setCachedState(snapshot licenseKeySnapshot, entitlements map[keygen.EntitlementCode]struct{}, machineID string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.licenseKeySnapshot = snapshot
	p.entitlements = entitlements
	p.machineID = machineID
	p.machineLookupAfter = time.Time{}
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
		p.setCachedState(snapshot, nil, "")
		return nil
	}

	entitlements, machineID, err := p.validate(ctx, snapshot.key)
	p.setCachedState(snapshot, entitlements, machineID)
	return err
}
