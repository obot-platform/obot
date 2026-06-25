package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/license"
)

type LicenseHandler struct {
	licenseProvider        *license.Provider
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

const (
	licenseKeyMask          = "****"
	licenseKeyVisibleSuffix = 8
	manualCheckCoolDown     = 5 * time.Minute
)

func NewLicenseHandler(licenseProvider *license.Provider) *LicenseHandler {
	return &LicenseHandler{licenseProvider: licenseProvider}
}

func (h *LicenseHandler) Get(req api.Context) error {
	return req.Write(h.status(req))
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

	return req.Write(h.status(req))
}

func (h *LicenseHandler) CheckLicense(req api.Context) error {
	if availableAt, ok := h.reserveManualLicenseCheck(); !ok {
		remaining := time.Until(availableAt)
		retryAfter := max(int((remaining+time.Second-1)/time.Second), 1)
		req.ResponseWriter.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		return apitypes.NewErrHTTP(http.StatusTooManyRequests, "license can only be manually checked once every 5 minutes")
	}

	if err := h.licenseProvider.Validate(req.Context()); err != nil && !errors.Is(err, license.ErrNotConfigured) {
		return err
	}

	return req.Write(h.status(req))
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

	return req.Write(h.status(req))
}

func (h *LicenseHandler) status(req api.Context) LicenseStatus {
	licenseKey := h.licenseProvider.LicenseKey()
	status := LicenseStatus{
		LicenseKey:             displayLicenseKey(licenseKey, req.UserIsAdmin()),
		Locked:                 h.licenseProvider.LicenseKeyViaConfiguration(),
		Enterprise:             h.licenseProvider.HasValidLicense(),
		Entitlements:           h.licenseProvider.Entitlements(),
		ManualCheckAvailableAt: h.manualLicenseCheckAvailableAt(),
	}

	if status.Locked {
		status.Source = "config"
	} else if licenseKey != "" {
		status.Source = "database"
	}

	return status
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
