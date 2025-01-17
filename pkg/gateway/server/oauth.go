package server

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const expirationDur = 7 * 24 * time.Hour

// oauth handles the initial oauth request, redirecting based on the "service" path parameter.
func (s *Server) oauth(apiContext api.Context) error {
	namespace := apiContext.PathValue("namespace")
	if namespace == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "no namespace path parameter provided")
	}

	name := apiContext.PathValue("name")
	if name == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "no name path parameter provided")
	}

	// Check to make sure this auth provider exists.
	list, err := s.dispatcher.ListConfiguredAuthProviders(apiContext.Context(), namespace)
	if err != nil {
		return fmt.Errorf("could not list configured auth providers: %w", err)
	} else if !slices.Contains(list, name) {
		return types2.NewErrHttp(http.StatusNotFound, "auth provider not found")
	}

	state, err := s.createState(apiContext.Context(), apiContext.PathValue("id"))
	if err != nil {
		return fmt.Errorf("could not create state: %w", err)
	}

	// Redirect the user through the oauth proxy flow so that everything is consistent.
	// The rd query parameter is used to redirect the user back through this oauth flow so a token can be generated.
	http.Redirect(
		apiContext.ResponseWriter,
		apiContext.Request,
		fmt.Sprintf("%s/oauth2/start?rd=%s&obot-auth-provider=%s",
			s.baseURL,
			url.QueryEscape(fmt.Sprintf("/api/oauth/redirect/%s/%s?state=%s", namespace, name, state)),
			url.QueryEscape(fmt.Sprintf("%s/%s", namespace, name)),
		),
		http.StatusFound,
	)

	return nil
}

// redirect handles the OAuth redirect for each service.
func (s *Server) redirect(apiContext api.Context) error {
	namespace := apiContext.PathValue("namespace")
	if namespace == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "no namespace path parameter provided")
	}

	name := apiContext.PathValue("name")
	if name == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "no name path parameter provided")
	}

	// Check to make sure this auth provider exists.
	list, err := s.dispatcher.ListConfiguredAuthProviders(apiContext.Context(), namespace)
	if err != nil {
		return fmt.Errorf("could not list configured auth providers: %w", err)
	} else if !slices.Contains(list, name) {
		return types2.NewErrHttp(http.StatusNotFound, "auth provider not found")
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
	tr.ExpiresAt = time.Now().Add(expirationDur) // TODO: make this configurable?

	tkn := &types.AuthToken{
		ID: fmt.Sprintf("%x", id),
		// Hash the token again for long-term storage
		HashedToken:           hashToken(fmt.Sprintf("%x", token)),
		ExpiresAt:             tr.ExpiresAt,
		AuthProviderNamespace: namespace,
		AuthProviderName:      name,
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
