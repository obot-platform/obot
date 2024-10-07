package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v62/github"
	"github.com/gptscript-ai/otto/pkg/gateway/client"
	kcontext "github.com/gptscript-ai/otto/pkg/gateway/context"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// oauth handles the initial oauth request, redirecting based on the "service" path parameter.
func (s *Server) oauth(w http.ResponseWriter, r *http.Request) {
	logger := kcontext.GetLogger(r.Context())
	service := r.PathValue("service")
	if service == "" {
		writeError(r.Context(), logger, w, http.StatusBadRequest, fmt.Errorf("no service path parameter provided"))
		return
	}

	oauthProvider := new(types.AuthProvider)
	if err := s.db.WithContext(r.Context()).Where("slug = ?", service).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeError(r.Context(), logger, w, status, fmt.Errorf("failed to find oauth provider: %v", err))
		return
	}

	state, nonce, err := s.createState(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(r.Context(), logger, w, http.StatusInternalServerError, fmt.Errorf("could not create state: %w", err))
		return
	}

	http.Redirect(w, r, oauthProvider.AuthURL(s.baseURL, state, nonce), http.StatusFound)
}

// redirect handles the OAuth redirect for each service.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	logger := kcontext.GetLogger(r.Context())

	service := r.PathValue("service")
	if service == "" {
		writeError(r.Context(), logger, w, http.StatusBadRequest, fmt.Errorf("no service path parameter provided"))
		return
	}

	oauthProvider := new(types.AuthProvider)
	if err := s.db.WithContext(r.Context()).Where("slug = ?", service).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeError(r.Context(), logger, w, status, fmt.Errorf("failed to find oauth provider: %v", err))
		return
	}

	// First, we need to get the value of the `code` query param
	if err := r.ParseForm(); err != nil {
		writeError(r.Context(), logger, w, http.StatusBadRequest, fmt.Errorf("could not parse query: %w", err))
		return
	}

	tr, err := s.verifyState(r.Context(), r.FormValue("state"))
	if err != nil {
		writeError(r.Context(), logger, w, http.StatusBadRequest, fmt.Errorf("invalid state: %v", err))
		return
	}

	var (
		identity *types.Identity
		status   int
	)
	switch oauthProvider.Type {
	case types.AuthTypeGitHub:
		identity, status, err = s.githubOauth(r, oauthProvider, tr)
		if err != nil {
			s.errorToken(r.Context(), tr, logger, w, status, err)
			return
		}
	case types.AuthTypeAzureAD, types.AuthTypeGoogle, types.AuthTypeGenericOIDC:
		identity, status, err = s.genericOIDC(r, oauthProvider, tr)
		if err != nil {
			s.errorToken(r.Context(), tr, logger, w, status, err)
			return
		}
	default:
		writeError(r.Context(), logger, w, http.StatusNotFound, fmt.Errorf("unknown oauth provider type: %v", oauthProvider.Type))
		return
	}

	identity.AuthProviderID = oauthProvider.ID

	role := types.RoleBasic
	if _, isAdmin := s.adminEmails[identity.Email]; isAdmin {
		role = types.RoleAdmin
	}

	id := make([]byte, tokenIDLength)
	if _, err := rand.Read(id); err != nil {
		s.errorToken(r.Context(), tr, logger, w, http.StatusInternalServerError, fmt.Errorf("could not generate token id: %v", err))
		return
	}

	// Hash the provider token
	token := sha256.Sum256([]byte(tr.Token))
	tr.Token = publicToken(id, token[:])
	tr.ExpiresAt = time.Now().Add(oauthProvider.ExpirationDur)

	tkn := &types.AuthToken{
		ID: fmt.Sprintf("%x", id),
		// Hash the token again for long-term storage
		HashedToken:    hashToken(fmt.Sprintf("%x", token)),
		ExpiresAt:      tr.ExpiresAt,
		AuthProviderID: oauthProvider.ID,
	}
	if err = s.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Updates(tr).Error; err != nil {
			return err
		}

		_, err := client.EnsureIdentity(tx, identity, role)
		if err != nil {
			return err
		}

		tkn.UserID = identity.UserID

		return tx.Create(tkn).Error
	}); err != nil {
		s.errorToken(r.Context(), tr, logger, w, status, err)
		return
	}

	if tr.CompletionRedirectURL == "" {
		tr.CompletionRedirectURL = s.authCompleteURL()
	}

	http.Redirect(w, r, tr.CompletionRedirectURL, http.StatusFound)
}

func (s *Server) githubOauth(r *http.Request, oauthProvider *types.AuthProvider, tr *types.TokenRequest) (*types.Identity, int, error) {
	token, err := exchange(r.Context(), oauthProvider, r.FormValue("code"))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("could not exchange code for token: %w", err)
	}

	ghClient := github.NewClient(s.httpClient).WithAuthToken(token.AccessToken).Users
	ghUser, ghResp, err := ghClient.Get(r.Context(), "")
	if err != nil {
		statusCode := http.StatusInternalServerError
		if ghResp != nil {
			statusCode = ghResp.StatusCode
		}
		return nil, statusCode, fmt.Errorf("could not get GitHub user: %w", err)
	}

	if emails, _, err := ghClient.ListEmails(r.Context(), nil); err == nil {
		for _, email := range emails {
			if email.GetPrimary() {
				ghUser.Email = &[]string{email.GetEmail()}[0]
				break
			}
		}
	}

	tr.Token = token.AccessToken
	return &types.Identity{
		ProviderUsername: ghUser.GetLogin(),
		Email:            ghUser.GetEmail(),
	}, http.StatusOK, nil
}

func (s *Server) genericOIDC(r *http.Request, oauthProvider *types.AuthProvider, tr *types.TokenRequest) (*types.Identity, int, error) {
	authErr := r.FormValue("error")
	errDescription := r.FormValue("error_description")
	if authErr != "" {
		return nil, http.StatusBadRequest, fmt.Errorf("error (%s): %s", authErr, errDescription)
	}

	idToken := r.FormValue("id_token")
	if idToken == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("no id_token provided")
	}

	jwks, err := keyfunc.NewDefaultCtx(r.Context(), []string{oauthProvider.JWKSURL})
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("could not get jwks: %w", err)
	}

	parsedToken, err := jwt.Parse(idToken, jwks.Keyfunc, jwt.WithIssuedAt(), jwt.WithAudience(oauthProvider.ClientID))
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("could not parse id_token: %w", err)
	}

	if !parsedToken.Valid {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid id_token")
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, http.StatusBadRequest, fmt.Errorf("could not get claims")
	}

	tr.Token = idToken
	username, _ := claims[oauthProvider.UsernameClaim].(string)
	email, _ := claims[oauthProvider.EmailClaim].(string)
	return &types.Identity{
		ProviderUsername: username,
		Email:            email,
	}, http.StatusOK, nil
}

func exchange(ctx context.Context, oauthProvider *types.AuthProvider, code string) (*oauth2.Token, error) {
	oauthConf := &oauth2.Config{
		ClientID:     oauthProvider.ClientID,
		ClientSecret: oauthProvider.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  oauthProvider.OAuthURL,
			TokenURL: oauthProvider.TokenURL,
		},
	}

	token, err := oauthConf.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("could not exchange code for token: %w", err)
	}

	return token, nil
}

func hashToken(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

func publicToken(id, token []byte) string {
	return fmt.Sprintf("%x:%x", id, token)
}
