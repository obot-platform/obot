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
	types2 "github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/gateway/client"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
	"golang.org/x/oauth2"
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

	state, nonce, err := s.createState(apiContext.Context(), apiContext.PathValue("id"))
	if err != nil {
		return fmt.Errorf("could not create state: %w", err)
	}

	http.Redirect(apiContext.ResponseWriter, apiContext.Request, oauthProvider.AuthURL(s.baseURL, state, nonce), http.StatusFound)
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

	// First, we need to get the value of the `code` query param
	if err := apiContext.ParseForm(); err != nil {
		return types2.NewErrHttp(http.StatusBadRequest, fmt.Sprintf("could not parse query: %v", err))
	}

	tr, err := s.verifyState(apiContext.Context(), apiContext.FormValue("state"))
	if err != nil {
		return types2.NewErrHttp(http.StatusBadRequest, fmt.Sprintf("invalid state: %v", err))
	}

	var (
		identity *types.Identity
		status   int
	)
	switch oauthProvider.Type {
	case types.AuthTypeGitHub:
		identity, status, err = s.githubOauth(apiContext.Request, oauthProvider, tr)
		if err != nil {
			return s.errorToken(apiContext.Context(), tr, status, err)
		}
	case types.AuthTypeAzureAD, types.AuthTypeGoogle, types.AuthTypeGenericOIDC:
		identity, status, err = s.genericOIDC(apiContext.Request, oauthProvider, tr)
		if err != nil {
			return s.errorToken(apiContext.Context(), tr, status, err)
		}
	default:
		return types2.NewErrNotFound("unknown oauth provider type: %v", oauthProvider.Type)
	}

	identity.AuthProviderID = oauthProvider.ID

	role := types2.RoleBasic
	if _, isAdmin := s.adminEmails[identity.Email]; isAdmin {
		role = types2.RoleAdmin
	}

	id := make([]byte, tokenIDLength)
	if _, err := rand.Read(id); err != nil {
		return types2.NewErrHttp(http.StatusInternalServerError, fmt.Sprintf("could not generate token id: %v", err))
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
	if err = s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
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
		return s.errorToken(apiContext.Context(), tr, status, err)
	}

	if tr.CompletionRedirectURL == "" {
		tr.CompletionRedirectURL = s.authCompleteURL()
	}

	http.Redirect(apiContext.ResponseWriter, apiContext.Request, tr.CompletionRedirectURL, http.StatusFound)
	return nil
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
