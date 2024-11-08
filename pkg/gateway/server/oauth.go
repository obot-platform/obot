package server

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	types2 "github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
	"gorm.io/gorm"
)

// oauth handles the initial oauth request, redirecting based on the "service" path parameter.
func (s *Server) oauth(apiContext api.Context) error {
	service := apiContext.PathValue("service")
	if service == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "no service path parameter provided")
	}

	oauthProvider := new(types.AuthProvider)
	if err := s.db.WithContext(apiContext.Context()).Where("slug = ?", service).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		return types2.NewErrHttp(status, fmt.Sprintf("failed to find oauth provider: %v", err))
	}

	state, err := s.createState(apiContext.Context(), apiContext.PathValue("id"))
	if err != nil {
		return fmt.Errorf("could not create state: %w", err)
	}

	// Redirect the user through the oauth proxy flow so that everything is consistent.
	// The rd query parameter is used to redirect the user back through this oauth flow so a token can be generated.
	http.Redirect(apiContext.ResponseWriter, apiContext.Request, fmt.Sprintf("%s/oauth2/start?rd=%s", s.baseURL, url.QueryEscape(fmt.Sprintf("/api/oauth/redirect/%s?state=%s", oauthProvider.Slug, state))), http.StatusFound)
	return nil
}

// redirect handles the OAuth redirect for each service.
func (s *Server) redirect(apiContext api.Context) error {
	service := apiContext.PathValue("service")
	if service == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "no service path parameter provided")
	}

	oauthProvider := new(types.AuthProvider)
	if err := s.db.WithContext(apiContext.Context()).Where("slug = ?", service).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		return types2.NewErrHttp(status, fmt.Sprintf("failed to find oauth provider: %v", err))
	}

	tr, err := s.verifyState(apiContext.Context(), apiContext.FormValue("state"))
	if err != nil {
		return types2.NewErrHttp(http.StatusBadRequest, fmt.Sprintf("invalid state: %v", err))
	}

	randBytes := make([]byte, tokenIDLength+randomTokenLength)
	if _, err := rand.Read(randBytes); err != nil {
		return types2.NewErrHttp(http.StatusInternalServerError, fmt.Sprintf("could not generate token id: %v", err))
	}

	id := randBytes[:tokenIDLength]
	token := randBytes[tokenIDLength:]
	tr.Token = publicToken(id, token[:])
	tr.ExpiresAt = time.Now().Add(oauthProvider.ExpirationDur)

	tkn := &types.AuthToken{
		ID: fmt.Sprintf("%x", id),
		// Hash the token again for long-term storage
		HashedToken:    hashToken(fmt.Sprintf("%x", token)),
		ExpiresAt:      tr.ExpiresAt,
		AuthProviderID: oauthProvider.ID,
	}
	if err = s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Updates(tr).Error; err != nil {
			return err
		}

		tkn.UserID = apiContext.UserID()

		return tx.Create(tkn).Error
	}); err != nil {
		return s.errorToken(apiContext.Context(), tr, http.StatusInternalServerError, err)
	}

	if tr.CompletionRedirectURL == "" {
		tr.CompletionRedirectURL = s.authCompleteURL()
	}

	http.Redirect(apiContext.ResponseWriter, apiContext.Request, tr.CompletionRedirectURL, http.StatusFound)
	return nil
}

func hashToken(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

func publicToken(id, token []byte) string {
	return fmt.Sprintf("%x:%x", id, token)
}
