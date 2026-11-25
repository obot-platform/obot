package license

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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

	// CommunityLicenseKeyPropertyKey is the database property key used to persist the Obot Community license key issued
	// at registration, so deleting a license key that replaced it falls back to it.
	CommunityLicenseKeyPropertyKey = "obot-community-license-key"

	// LicenseFilePropertyKey is the database property key used to persist an uploaded license file.
	// It's separate from LicenseKeyPropertyKey so releases without license file support never read a file as a key,
	// and so a stored key stays underneath to fall back to.
	LicenseFilePropertyKey = "obot-license-file"

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
	keygenAPIURL        = "https://license.obot.ai"
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

	// ErrLicenseFileAsKey indicates a license file was given as a license key.
	ErrLicenseFileAsKey = errors.New("license key is a license file")

	// ErrAirgap indicates a license key was given to an air-gapped installation, which never contacts Keygen.
	ErrAirgap = errors.New("air-gapped installations are licensed with a license file, not a license key")

	// ErrNotAirgap indicates a license file was given to an installation that isn't air-gapped, which is licensed with a
	// license key.
	ErrNotAirgap = errors.New("license files are only used by air-gapped installations; set OBOT_SERVER_AIRGAP=true")

	// errLicenseFileExpired refuses the upload of a license file that verifies but has already expired.
	errLicenseFileExpired = &FileError{Message: "the license file has expired; ask Obot for a new one"}
)

// Config contains the settings needed to license an Obot installation.
type Config struct {
	LicenseKey  string `usage:"Keygen license key for this Obot installation"`
	LicenseFile string `usage:"Contents of a Keygen license file for this air-gapped Obot installation"`

	// airgap is set with WithAirgap rather than a flag of its own, since the server's air-gap setting decides it.
	airgap bool
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

	// airgap means the installation never contacts Keygen: only license files license it, and license keys, stored
	// or new, are ignored or refused. An installation that isn't air-gapped never uses a license file.
	airgap bool

	// configuredLicenseFile is the license file from configuration, set once at startup; file is set when there is one.
	configuredLicenseFile licenseKeySnapshot

	// licenseFile is the verified license file in effect, cached alongside licenseKeySnapshot.
	// It's nil while a license key is in effect, and when the license file in effect couldn't be read or verified.
	// licenseFileErr then says why, in words that are safe to show an administrator.
	licenseFile    *licenseFile
	licenseFileErr string

	// licenseFilePublicKey is the key license files must be signed with; tests swap in their own.
	licenseFilePublicKey ed25519.PublicKey

	// now is the clock license files are checked against on every read; tests swap in their own.
	now func() time.Time
}

// licenseKeySnapshot identifies the license in effect: a license key, or a license file's contents when file is set.
type licenseKeySnapshot struct {
	key              string
	file             bool
	updatedAt        time.Time
	propertyKey      string
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
		s.file == other.file &&
		s.viaConfiguration == other.viaConfiguration &&
		s.updatedAt.Equal(other.updatedAt)
}

// WithAirgap returns the config for an air-gapped installation, which is licensed only by license files and never
// contacts Keygen.
func (c Config) WithAirgap(airgap bool) Config {
	c.airgap = airgap
	return c
}

// NewProvider creates a Keygen-backed license provider. An air-gapped installation is licensed only by license files,
// verified offline, and any other only by license keys.
// It refuses to start only for settings that contradict each other:
// - A license file is configured for an installation that isn't air-gapped
// - The configured license key is a license file
// - A license key is configured for an air-gapped installation
// A configured license file that can't be verified starts the installation unlicensed instead, and LicenseFileError
// says why, rather than a damaged copy or a rotated key keeping Obot from starting.
func NewProvider(ctx context.Context, gatewayClient *client.Client, config Config) (*Provider, error) {
	return newProvider(ctx, gatewayClient, config, keygenAPIURL, productionLicenseFilePublicKey())
}

func newProvider(ctx context.Context, gatewayClient *client.Client, config Config, apiURL string, licenseFilePublicKey ed25519.PublicKey) (*Provider, error) {
	configuredLicenseKey, configuredLicenseFile := strings.TrimSpace(config.LicenseKey), strings.TrimSpace(config.LicenseFile)
	switch {
	case configuredLicenseFile != "" && !config.airgap:
		return nil, fmt.Errorf("a license file is configured: %w", ErrNotAirgap)
	case isLicenseFile(configuredLicenseKey):
		// Never send a license file to Keygen as a key.
		return nil, fmt.Errorf("%w, configure it as the license file instead", ErrLicenseFileAsKey)
	case configuredLicenseKey != "" && config.airgap:
		return nil, fmt.Errorf("a license key is configured: %w", ErrAirgap)
	}

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
		configuredLicenseKey: configuredLicenseKey,
		keygenAPIURL:         apiURL,
		airgap:               config.airgap,
		licenseFilePublicKey: licenseFilePublicKey,
		now:                  time.Now,
	}

	if configuredLicenseFile != "" {
		// The first refresh verifies it. A file that doesn't verify stays the configured license and grants nothing.
		k.configuredLicenseFile = licenseKeySnapshot{
			key:              normalizeLicenseFile(configuredLicenseFile),
			file:             true,
			viaConfiguration: true,
		}
	}

	if err := k.refresh(ctx, true); err != nil {
		slog.Warn("initial license refresh failed", "error", err)
	}

	if entitlements := k.currentEntitlements(); entitlements != nil {
		slog.Info("license provider initialized", "entitlements", entitlements)
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

// LicenseKey returns the license key in effect.
// It returns nothing while a license file is in effect, so callers that send the key to Keygen or Obot services have
// nothing to send.
func (p *Provider) LicenseKey(ctx context.Context) (string, error) {
	snapshot, err := p.loadLicense(ctx)
	if err != nil || snapshot.file {
		return "", err
	}
	return snapshot.key, nil
}

// LicenseFile reports whether a license file is in effect and, when it verified, its effective expiry.
// A license file that couldn't be read or verified is still in effect, it just grants nothing; see LicenseFileError.
func (p *Provider) LicenseFile(ctx context.Context) (bool, *time.Time, error) {
	if err := p.refresh(ctx, false); err != nil {
		return false, nil, err
	}
	p.lock.RLock()
	defer p.lock.RUnlock()

	if p.licenseFile == nil {
		return p.licenseKeySnapshot.file, nil, nil
	}
	return true, p.licenseFile.Expiry, nil
}

// LicenseFileError returns why the license file in effect grants nothing, when it couldn't be read or verified.
// It goes by the state LicenseFile last refreshed, and is safe to show an administrator.
func (p *Provider) LicenseFileError() string {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.licenseFileErr
}

// Airgap reports whether the installation is air-gapped, so only license files license it.
func (p *Provider) Airgap() bool {
	return p.airgap
}

// MachineFingerprint returns the existing persisted installation identity.
func (p *Provider) MachineFingerprint() string {
	return p.machineFingerprint
}

// LicenseKeyViaConfiguration reports whether configuration supplies the license, as a key or a file, so it can't be
// changed through the API.
func (p *Provider) LicenseKeyViaConfiguration() bool {
	return p.configuredLicenseKey != "" || p.configuredLicenseFile.file
}

// loadLicense returns the license in effect. An air-gapped installation is licensed by the configured license file, else
// the stored one. Any other by the license key in effect; see loadLicenseKey.
func (p *Provider) loadLicense(ctx context.Context) (licenseKeySnapshot, error) {
	switch {
	case !p.airgap:
		return p.loadLicenseKey(ctx)
	case p.configuredLicenseFile.file:
		return p.configuredLicenseFile, nil
	case p.gatewayClient == nil:
		return licenseKeySnapshot{}, nil
	}

	property, err := p.gatewayClient.GetProperty(ctx, LicenseFilePropertyKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return licenseKeySnapshot{}, nil
	}
	if err != nil {
		return licenseKeySnapshot{}, fmt.Errorf("failed to get %s property: %w", LicenseFilePropertyKey, err)
	}
	contents := strings.TrimSpace(property.Value)
	if contents == "" {
		return licenseKeySnapshot{}, nil
	}
	return licenseKeySnapshot{
		key:         contents,
		file:        true,
		updatedAt:   property.UpdatedAt,
		propertyKey: LicenseFilePropertyKey,
	}, nil
}

// loadLicenseKey returns the license key in effect: the configured key, else the key in LicenseKeyPropertyKey, else the
// registered Community key.
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
			return licenseKeySnapshot{}, fmt.Errorf("failed to get %s property: %w", propertyKey, err)
		}
		if key := strings.TrimSpace(property.Value); key != "" {
			return licenseKeySnapshot{
				key:         key,
				updatedAt:   property.UpdatedAt,
				propertyKey: propertyKey,
			}, nil
		}
	}
	return licenseKeySnapshot{}, nil
}

// SetLicenseKey stores the license key after Keygen accepts it.
// It returns ErrInvalidLicense when Keygen rejects the key and ErrLicenseCheckFailed when Keygen didn't give a verdict.
func (p *Provider) SetLicenseKey(ctx context.Context, licenseKey string) error {
	return p.setLicenseKey(ctx, licenseKey, false)
}

// SetCommunityLicenseKey stores the Obot Community license key issued at registration like SetLicenseKey, and keeps it
// to fall back to when a license key added later is deleted.
func (p *Provider) SetCommunityLicenseKey(ctx context.Context, licenseKey string) error {
	return p.setLicenseKey(ctx, licenseKey, true)
}

func (p *Provider) setLicenseKey(ctx context.Context, licenseKey string, community bool) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	if p.airgap {
		return ErrAirgap
	}
	licenseKey = strings.TrimSpace(licenseKey)
	if isLicenseFile(licenseKey) {
		// Never send a license file to Keygen as a key, it goes through SetLicenseFile.
		return ErrLicenseFileAsKey
	}

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
	if community {
		// Keep the Community key to fall back to.
		_, err = p.gatewayClient.SetProperty(ctx, CommunityLicenseKeyPropertyKey, licenseKey)
	} else {
		err = p.rescueCommunityKey(ctx)
	}
	if err != nil {
		return err
	}
	// Store the key in effect. A Community key goes here too, so Obot versions that only read LicenseKeyPropertyKey still
	// find it.
	if _, err := p.gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, licenseKey); err != nil {
		return err
	}

	// Cache the snapshot as read back, since the database stores updatedAt with less precision than it was written with.
	// If that fails, or another replica has replaced the key since, the next refresh catches up.
	if snapshot, err := p.loadLicenseKey(ctx); err == nil && snapshot.key == licenseKey {
		p.setCachedState(snapshot, state.Entitlements, nil, "")
	}
	return nil
}

// SetLicenseFile stores a license file after verifying it, which licenses an air-gapped installation.
// It returns ErrNotAirgap for any other installation, and a *FileError when the file fails verification or has expired;
// then nothing is stored. It never contacts Keygen.
func (p *Provider) SetLicenseFile(ctx context.Context, contents string) error {
	if !p.airgap {
		return ErrNotAirgap
	}
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}
	contents = normalizeLicenseFile(contents)

	file, err := verifyLicenseFile(contents, p.licenseFilePublicKey)
	if err != nil {
		return err
	}
	// Refuse a file that would grant nothing as soon as it's stored.
	if file.expired(p.now()) {
		return errLicenseFileExpired
	}

	if p.gatewayClient == nil {
		return fmt.Errorf("failed to persist license file: gateway client is not configured")
	}

	// Serialize the stored value and its cached state with refresh, the same as setLicenseKey.
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	if _, err := p.gatewayClient.SetProperty(ctx, LicenseFilePropertyKey, contents); err != nil {
		return err
	}

	// Cache the file as read back, so this replica doesn't verify it again. Other replicas verify it on their next read.
	if snapshot, err := p.loadLicense(ctx); err == nil && snapshot.key == contents {
		p.setCachedState(snapshot, nil, &file, "")
	}
	return nil
}

// rescueCommunityKey keeps the key in LicenseKeyPropertyKey as the registered Community key before a license key
// replaces it, if it's an Obot Community license. Registrations stored their key there before it had its own property.
// It goes by the key's saved entitlements, and checks the key with Keygen when none are saved. When it can't tell, it
// returns the error so the key isn't replaced.
func (p *Provider) rescueCommunityKey(ctx context.Context) error {
	_, err := p.gatewayClient.GetProperty(ctx, CommunityLicenseKeyPropertyKey)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		// A registered Community key is already kept, or the lookup failed.
		return err
	}
	snapshot, err := p.loadLicenseKey(ctx)
	if err != nil || snapshot.propertyKey != LicenseKeyPropertyKey {
		// There's no key in LicenseKeyPropertyKey to replace.
		return err
	}

	state, err := p.loadLicenseState(ctx, snapshot.key)
	if err == nil && state.Entitlements == nil {
		state, err = p.validate(ctx, snapshot.key)
	}
	if err != nil || !isCommunityLicense(state.Entitlements) {
		return err
	}
	_, err = p.gatewayClient.SetProperty(ctx, CommunityLicenseKeyPropertyKey, snapshot.key)
	return err
}

// isCommunityLicense reports whether the entitlements are an Obot Community license's.
// Enterprise and Cloud licenses can include CommunityEntitlement too.
func isCommunityLicense(entitlements map[keygen.EntitlementCode]struct{}) bool {
	_, community := entitlements[CommunityEntitlement]
	_, enterprise := entitlements[EnterpriseEntitlement]
	_, cloud := entitlements[CloudEntitlement]
	return community && !enterprise && !cloud
}

func (p *Provider) Validate(ctx context.Context) error {
	return p.refresh(ctx, true)
}

// RemoveLicenseKey removes the license in effect: the stored license file of an air-gapped installation, else the license
// key. Removing a license key that replaced a registered Community key falls back to the Community key, and removing the
// Community key itself leaves the installation unregistered.
func (p *Provider) RemoveLicenseKey(ctx context.Context) error {
	if p.LicenseKeyViaConfiguration() {
		return ErrLicenseKeyViaConfiguration
	}

	// Keep deletion and cache invalidation ordered with refresh so an in-flight
	// validation cannot restore the state for the removed key.
	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	if err := p.deleteLicenseKey(ctx); err != nil {
		return err
	}

	p.setCachedState(licenseKeySnapshot{}, nil, nil, "")
	return nil
}

// deleteLicenseKey deletes the license in effect: an air-gapped installation's stored license file, or a key from every
// property that holds it.
func (p *Provider) deleteLicenseKey(ctx context.Context) error {
	removed, err := p.loadLicense(ctx)
	if err != nil || removed.propertyKey == "" {
		return err
	}
	if err := p.gatewayClient.DeleteProperty(ctx, removed.propertyKey); err != nil || removed.file {
		return err
	}

	// A registered Community key is stored in both properties, delete the other copy too. A different key underneath is
	// the Community key to fall back to.
	next, err := p.loadLicenseKey(ctx)
	if err != nil || next.key != removed.key {
		return err
	}
	return p.gatewayClient.DeleteProperty(ctx, next.propertyKey)
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
	return p.currentEntitlements() != nil, nil
}

// currentEntitlements returns the cached entitlements of the license in effect.
// A license file's entitlements count only until its expiry at p.now(), so a file expires without a restart.
func (p *Provider) currentEntitlements() map[keygen.EntitlementCode]struct{} {
	p.lock.RLock()
	defer p.lock.RUnlock()

	if !p.licenseKeySnapshot.file {
		return p.entitlements
	}
	if p.licenseFile == nil || p.licenseFile.expired(p.now()) {
		return nil
	}
	return p.licenseFile.Entitlements
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
				slog.Info("license update finished", "entitlements", p.currentEntitlements())
			}
		}
	}
}

func (p *Provider) cachedSnapshotMatches(snapshot licenseKeySnapshot) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.licenseKeySnapshot.equal(snapshot)
}

func (p *Provider) setCachedState(snapshot licenseKeySnapshot, entitlements map[keygen.EntitlementCode]struct{}, file *licenseFile, fileErr string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.licenseKeySnapshot = snapshot
	p.entitlements = entitlements
	p.licenseFile = file
	p.licenseFileErr = fileErr
}

// refresh checks the license key with Keygen and caches its entitlements, unless force is false and the key hasn't
// changed since the last check.
// A license file is verified locally instead, and only when it changes, even when forced. It never contacts Keygen.
// When Keygen doesn't give a verdict, it falls back to the entitlements Keygen last accepted for the key, unless the
// license has expired since, and a forced check also returns the ErrLicenseCheckFailed error.
func (p *Provider) refresh(ctx context.Context, force bool) error {
	snapshot, err := p.loadLicense(ctx)
	if err != nil {
		return err
	}
	// A license file can't change without the stored value changing, so there's nothing to recheck.
	force = force && !snapshot.file
	if !force && p.cachedSnapshotMatches(snapshot) {
		return nil
	}

	p.refreshLock.Lock()
	defer p.refreshLock.Unlock()

	// Another request may have refreshed the provider while this request waited.
	snapshot, err = p.loadLicense(ctx)
	if err != nil {
		return err
	}
	if !force && p.cachedSnapshotMatches(snapshot) {
		return nil
	}

	if snapshot.file {
		// A file that fails verification leaves the replica unlicensed, with no fallback to a key or Keygen.
		file, err := p.loadLicenseFile(snapshot.key)
		p.setCachedState(snapshot, nil, file, licenseFileErrorMessage(err))
		return nil
	}
	if snapshot.key == "" {
		p.setCachedState(snapshot, nil, nil, "")
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

	p.setCachedState(snapshot, state.Entitlements, nil, "")
	if force {
		// The license keeps working, but an explicit check should report that Keygen didn't answer.
		return checkErr
	}
	return nil
}

// loadLicenseFile verifies a license file against the provider's public key and logs the outcome.
// The log carries the file's id, expiry, status, and entitlements, never its contents, since it embeds the license key.
func (p *Provider) loadLicenseFile(contents string) (*licenseFile, error) {
	file, err := verifyLicenseFile(contents, p.licenseFilePublicKey)
	if err != nil {
		slog.Error("license file rejected", "error", err)
		return nil, err
	}
	expiry := "never"
	if file.Expiry != nil {
		expiry = file.Expiry.Format(time.RFC3339)
	}
	slog.Info("license file verified",
		"id", file.ID,
		"expiry", expiry,
		"expired", file.expired(p.now()),
		"entitlements", slices.Sorted(maps.Keys(file.Entitlements)),
	)
	return &file, nil
}

// licenseFileErrorMessage returns the message of a license file error, which is safe to show an administrator.
func licenseFileErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
