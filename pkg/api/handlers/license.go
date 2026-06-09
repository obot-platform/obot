package handlers

import (
	"errors"
	"strings"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/license"
)

type LicenseHandler struct {
	licenseProvider *license.KeygenProvider
}

type LicenseStatus struct {
	LicenseKey   string   `json:"licenseKey"`
	Source       string   `json:"source"`
	Locked       bool     `json:"locked"`
	Enterprise   bool     `json:"enterprise"`
	Entitlements []string `json:"entitlements"`
}

type LicenseUpdate struct {
	LicenseKey string `json:"licenseKey"`
}

const (
	licenseKeyMask          = "****"
	licenseKeyVisibleSuffix = 8
)

func NewLicenseHandler(licenseProvider *license.KeygenProvider) *LicenseHandler {
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
		LicenseKey:   displayLicenseKey(licenseKey, req.UserIsAdmin()),
		Locked:       h.licenseProvider.LicenseKeyViaConfiguration(),
		Enterprise:   h.licenseProvider.HasValidLicense(),
		Entitlements: h.licenseProvider.Entitlements(),
	}

	if status.Locked {
		status.Source = "config"
	} else if licenseKey != "" {
		status.Source = "database"
	}

	return status
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
