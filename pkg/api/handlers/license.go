package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/create"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/upgrade"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	licenseKeyMask          = "****"
	licenseKeyVisibleSuffix = 8
	manualCheckCoolDown     = 5 * time.Minute
)

type LicenseProvider interface {
	LicenseKey(context.Context) (string, error)
	LicenseKeyViaConfiguration() bool
	SetLicenseKey(context.Context, string) error
	RemoveLicenseKey(context.Context) error
	Validate(context.Context) error
	HasValidLicense(context.Context) (bool, error)
	Entitlements(context.Context) ([]string, error)
}

type LicenseHandler struct {
	licenseProvider        LicenseProvider
	communityIssuer        upgrade.CommunityLicenseIssuer
	communityLock          sync.Mutex
	communityInFlight      bool
	manualCheckLock        sync.RWMutex
	lastManualLicenseCheck time.Time
}

type LicenseStatus struct {
	LicenseKey             string     `json:"licenseKey"`
	Source                 string     `json:"source"`
	Locked                 bool       `json:"locked"`
	Enterprise             bool       `json:"enterprise"`
	Entitlements           []string   `json:"entitlements"`
	ManualCheckAvailableAt *time.Time `json:"manualCheckAvailableAt,omitempty"`
}

type LicenseUpdate struct {
	LicenseKey string `json:"licenseKey"`
}

func NewLicenseHandler(licenseProvider LicenseProvider, communityIssuer upgrade.CommunityLicenseIssuer) *LicenseHandler {
	return &LicenseHandler{
		licenseProvider: licenseProvider,
		communityIssuer: communityIssuer,
	}
}

func (h *LicenseHandler) Get(req api.Context) error {
	status, err := h.status(req)
	if err != nil {
		return err
	}
	return req.Write(status)
}

func (h *LicenseHandler) Update(req api.Context) error {
	var input LicenseUpdate
	if err := req.Read(&input); err != nil {
		return err
	}
	input.LicenseKey = strings.TrimSpace(input.LicenseKey)
	if input.LicenseKey == "" {
		return apitypes.NewErrBadRequest("licenseKey is required")
	}

	if err := h.licenseProvider.SetLicenseKey(req.Context(), input.LicenseKey); err != nil {
		if errors.Is(err, license.ErrLicenseKeyViaConfiguration) {
			return apitypes.NewErrBadRequest("license key is configured at startup and cannot be updated via the API")
		}
		if errors.Is(err, license.ErrInvalidLicense) {
			return apitypes.NewErrBadRequest("license key is invalid")
		}
		return err
	}

	status, err := h.status(req)
	if err != nil {
		return err
	}
	return req.Write(status)
}

func (h *LicenseHandler) CheckLicense(req api.Context) error {
	if availableAt, ok := h.reserveManualLicenseCheck(); !ok {
		remaining := time.Until(availableAt)
		retryAfter := max(int((remaining+time.Second-1)/time.Second), 1)
		req.ResponseWriter.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		return apitypes.NewErrHTTP(http.StatusTooManyRequests, "license can only be manually checked once every 5 minutes")
	}

	if err := h.licenseProvider.Validate(req.Context()); err != nil {
		if !errors.Is(err, license.ErrNotConfigured) {
			return err
		}
	} else if err := signalLicenseRefresh(req.Context(), req.Storage); err != nil {
		return err
	}

	status, err := h.status(req)
	if err != nil {
		return err
	}
	return req.Write(status)
}

func signalLicenseRefresh(ctx context.Context, client kclient.Client) error {
	providerSync := &v1.ProviderSync{
		Name:      system.ProviderSyncName,
		Namespace: system.DefaultNamespace,
	}
	if err := create.OrGet(ctx, client, providerSync); err != nil {
		return fmt.Errorf("ensure provider sync for license refresh: %w", err)
	}

	originalRevision := providerSync.Spec.Revisions[string(v1.ProviderTypeLicense)].Revision
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(ctx, kclient.ObjectKeyFromObject(providerSync), providerSync); err != nil {
			return err
		}

		if providerSync.Spec.Revisions == nil {
			providerSync.Spec.Revisions = make(map[string]v1.ProviderRevision, 1)
		}

		if providerSync.Spec.Revisions[string(v1.ProviderTypeLicense)].Revision != originalRevision {
			// The revision was already bumped, so we don't need to do it again.
			return nil
		}

		providerSync.Spec.Revisions[string(v1.ProviderTypeLicense)] = v1.ProviderRevision{
			ProviderType: v1.ProviderTypeLicense,
			ProviderName: "LicenseProvider",
			Revision:     originalRevision + 1,
		}
		return client.Update(ctx, providerSync)
	}); err != nil {
		return fmt.Errorf("signal license refresh: %w", err)
	}
	return nil
}

// reserveManualLicenseCheck reserves a manual license check, returning the time at which it can be checked again and whether the reservation was successful.
// This isn't HA-safe, but is sufficient for the current use case.
func (h *LicenseHandler) reserveManualLicenseCheck() (time.Time, bool) {
	h.manualCheckLock.RLock()
	last := h.lastManualLicenseCheck
	h.manualCheckLock.RUnlock()

	now := time.Now()
	availableAt := last.Add(manualCheckCoolDown)
	if !last.IsZero() && now.Before(availableAt) {
		return availableAt, false
	}

	h.manualCheckLock.Lock()
	defer h.manualCheckLock.Unlock()

	// Check again with the write lock to avoid race conditions.
	availableAt = last.Add(manualCheckCoolDown)
	if !last.IsZero() && now.Before(availableAt) {
		return availableAt, false
	}

	h.lastManualLicenseCheck = now
	return now.Add(manualCheckCoolDown), true
}

func (h *LicenseHandler) Delete(req api.Context) error {
	if err := h.licenseProvider.RemoveLicenseKey(req.Context()); err != nil {
		if errors.Is(err, license.ErrLicenseKeyViaConfiguration) {
			return apitypes.NewErrBadRequest("license key is configured via configuration and cannot be deleted via the API")
		}
		return err
	}

	status, err := h.status(req)
	if err != nil {
		return err
	}
	return req.Write(status)
}

func (h *LicenseHandler) status(req api.Context) (LicenseStatus, error) {
	licenseKey, err := h.licenseProvider.LicenseKey(req.Context())
	if err != nil {
		return LicenseStatus{}, err
	}
	enterprise, err := h.licenseProvider.HasValidLicense(req.Context())
	if err != nil {
		return LicenseStatus{}, err
	}
	entitlements, err := h.licenseProvider.Entitlements(req.Context())
	if err != nil {
		return LicenseStatus{}, err
	}
	status := LicenseStatus{
		LicenseKey:             displayLicenseKey(licenseKey, req.UserIsAdmin()),
		Locked:                 h.licenseProvider.LicenseKeyViaConfiguration(),
		Enterprise:             enterprise,
		Entitlements:           entitlements,
		ManualCheckAvailableAt: h.manualLicenseCheckAvailableAt(),
	}

	if status.Locked {
		status.Source = "config"
	} else if licenseKey != "" {
		status.Source = "database"
	}

	return status, nil
}

func (h *LicenseHandler) manualLicenseCheckAvailableAt() *time.Time {
	h.manualCheckLock.RLock()
	defer h.manualCheckLock.RUnlock()

	if h.lastManualLicenseCheck.IsZero() {
		return nil
	}

	availableAt := h.lastManualLicenseCheck.Add(manualCheckCoolDown)
	if !time.Now().Before(availableAt) {
		return nil
	}

	availableAt = availableAt.UTC()
	return &availableAt
}

func displayLicenseKey(licenseKey string, canViewPartial bool) string {
	licenseKey = strings.TrimSpace(licenseKey)
	if licenseKey == "" {
		return ""
	}
	if !canViewPartial || len(licenseKey) <= licenseKeyVisibleSuffix {
		return licenseKeyMask
	}
	return licenseKeyMask + licenseKey[len(licenseKey)-licenseKeyVisibleSuffix:]
}
