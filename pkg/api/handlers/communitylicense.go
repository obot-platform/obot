package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"slices"
	"strings"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/upgrade"
)

type CommunityLicenseEnrollment struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Company string `json:"company,omitempty"`
}

func (h *LicenseHandler) CreateCommunityLicense(req api.Context) error {
	var input CommunityLicenseEnrollment
	if err := req.Read(&input); err != nil {
		return apitypes.NewErrBadRequest("invalid Obot Community enrollment request")
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	input.Company = strings.TrimSpace(input.Company)
	if input.Name == "" {
		return apitypes.NewErrBadRequest("name is required")
	}
	if input.Email == "" {
		return apitypes.NewErrBadRequest("email is required")
	}
	parsedEmail, err := mail.ParseAddress(input.Email)
	if err != nil {
		return apitypes.NewErrBadRequest("a valid email address is required")
	}
	if !hasEmailDomainSuffix(parsedEmail.Address) {
		return apitypes.NewErrBadRequest("a valid email address is required")
	}
	input.Email = parsedEmail.Address

	if err := h.communityEligibilityError(req.Context()); err != nil {
		return err
	}
	if !h.reserveCommunityEnrollment() {
		return apitypes.NewErrAlreadyExists("an Obot Community enrollment request is already in progress")
	}
	defer h.releaseCommunityEnrollment()

	if err := h.communityEligibilityError(req.Context()); err != nil {
		return err
	}

	issuedKey, err := h.communityIssuer.Issue(req.Context(), upgrade.CommunityLicenseRequest{
		Name:    input.Name,
		Email:   input.Email,
		Company: input.Company,
	})
	if err != nil {
		return apitypes.NewErrHTTP(http.StatusBadGateway, "failed to obtain an Obot Community license")
	}

	if err := h.licenseProvider.SetLicenseKey(req.Context(), issuedKey); err != nil {
		if errors.Is(err, license.ErrLicenseKeyViaConfiguration) {
			return apitypes.NewErrAlreadyExists("license key is configured at startup and cannot be updated via the API")
		}
		if errors.Is(err, license.ErrInvalidLicense) {
			return apitypes.NewErrHTTP(http.StatusBadGateway, "the issued Obot Community license is invalid")
		}
		return apitypes.NewErrHTTP(http.StatusInternalServerError, "failed to install the Obot Community license")
	}

	status, err := h.status(req)
	if err != nil {
		return err
	}
	return req.Write(status)
}

func hasEmailDomainSuffix(address string) bool {
	at := strings.LastIndexByte(address, '@')
	if at <= 0 || at == len(address)-1 {
		return false
	}

	labels := strings.Split(address[at+1:], ".")
	if len(labels) < 2 {
		return false
	}
	if slices.Contains(labels, "") {
		return false
	}
	return true
}

func (h *LicenseHandler) communityEligibilityError(ctx context.Context) error {
	if ok, err := h.licenseProvider.HasValidLicense(ctx); err != nil {
		return err
	} else if ok {
		return apitypes.NewErrAlreadyExists("a valid license is already active")
	}
	if h.licenseProvider.LicenseKeyViaConfiguration() {
		return apitypes.NewErrAlreadyExists("license key is configured at startup and cannot be updated via the API")
	}
	return nil
}

func (h *LicenseHandler) reserveCommunityEnrollment() bool {
	h.communityLock.Lock()
	defer h.communityLock.Unlock()
	if h.communityInFlight {
		return false
	}
	h.communityInFlight = true
	return true
}

func (h *LicenseHandler) releaseCommunityEnrollment() {
	h.communityLock.Lock()
	h.communityInFlight = false
	h.communityLock.Unlock()
}
