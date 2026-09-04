package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/setup"
	"github.com/obot-platform/obot/pkg/auth"
	kcontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// oauth handles the initial oauth request, redirecting based on the "service" path parameter.
func (s *Server) oauth(apiContext api.Context) error {
	namespace := apiContext.PathValue("namespace")
	if namespace == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "no namespace path parameter provided")
	}

	name := apiContext.PathValue("name")
	if name == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "no name path parameter provided")
	}

	// This is the only entry point that knows the verification ID, so it authorizes a staged
	// provider from the path rather than from the cookie it goes on to issue.
	id := apiContext.PathValue("id")
	if err := s.authorizeAuthProviderLogin(apiContext, name, id); err != nil {
		return err
	}

	state, err := apiContext.GatewayClient.CreateTokenRequestState(apiContext.Context(), id)
	if err != nil {
		return fmt.Errorf("could not create state: %w", err)
	}
	slog.Info("Starting OAuth flow for token request", "tokenRequestID", apiContext.PathValue("id"), "providerNamespace", namespace, "providerName", name)

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

// authorizeAuthProviderLogin authorizes an OAuth start against the requested provider. A staged
// provider is allowed only for the verification it was issued for, and that verification is then
// pinned to this browser for the rest of the OAuth round trip.
func (s *Server) authorizeAuthProviderLogin(apiContext api.Context, name, id string) error {
	configured, err := s.dispatcher.GetConfiguredAuthProvider(apiContext.Context())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get configured auth provider: %v", err))
	}
	if configured == "" || configured == name {
		return nil
	}

	staged, err := s.dispatcher.GetStagedAuthProvider(apiContext.Context())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get staged auth provider: %v", err))
	}
	if staged != name {
		slog.Info("Rejected OAuth start for unconfigured auth provider", "requestedProvider", name, "tokenRequestID", id)
		return types2.NewErrHTTP(http.StatusNotFound, "auth provider not found")
	}

	// Bound to the owner who started the switch, not merely to whoever holds the ID.
	startable, err := apiContext.GatewayClient.AuthProviderVerificationStartableBy(apiContext.Context(), id, apiContext.UserID())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to check for auth provider verification: %v", err))
	}
	if !startable {
		slog.Info("Rejected OAuth start for staged auth provider without an open verification for this user", "requestedProvider", name, "tokenRequestID", id, "userID", apiContext.UserID())
		return types2.NewErrHTTP(http.StatusNotFound, "auth provider not found")
	}

	http.SetCookie(apiContext.ResponseWriter, &http.Cookie{
		Name:     auth.AuthProviderVerifyCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   int(auth.AuthProviderVerifyWindow.Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.baseURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// redirect handles the OAuth redirect for each service.
func (s *Server) redirect(apiContext api.Context) error {
	namespace := apiContext.PathValue("namespace")
	if namespace == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "no namespace path parameter provided")
	}

	name := apiContext.PathValue("name")
	if name == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "no name path parameter provided")
	}

	// Check to make sure this auth provider exists.
	loginable, err := s.dispatcher.LoginableAuthProvider(apiContext.Context(), apiContext.Request, name)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get configured auth provider: %v", err))
	}
	if !loginable {
		slog.Info("Rejected OAuth redirect for unconfigured auth provider", "requestedProvider", name)
		return types2.NewErrHTTP(http.StatusNotFound, "auth provider not found")
	}

	tr, err := apiContext.GatewayClient.VerifyTokenRequestState(apiContext.Context(), apiContext.FormValue("state"))
	if err != nil {
		return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("invalid state: %v", err))
	}

	// A verification only has to record who signed in. It must not run the setup-only API key flow,
	// which rejects every purpose but "setup" and would mint a token nothing here reads.
	if tr.Purpose == types.TokenRequestPurposeAuthProviderVerify {
		staged, err := s.dispatcher.GetStagedAuthProvider(apiContext.Context())
		if err != nil {
			return s.errorToken(apiContext.Context(), tr, http.StatusInternalServerError, err)
		}
		identityName, identityNamespace := apiContext.AuthProviderNameAndNamespace()
		if name != staged || identityName != name || identityNamespace != namespace {
			return s.errorToken(apiContext.Context(), tr, http.StatusBadRequest,
				fmt.Errorf("verification must complete through the staged auth provider"))
		}

		user, err := apiContext.GatewayClient.UserByID(apiContext.Context(), fmt.Sprintf("%d", apiContext.UserID()))
		if err != nil {
			return s.errorToken(apiContext.Context(), tr, http.StatusInternalServerError, err)
		}

		if err := setup.PromoteToOwner(apiContext, user); err != nil {
			return s.errorToken(apiContext.Context(), tr, http.StatusForbidden, err)
		}

		// Caching the identity makes the verification outlive the browser that produced it, so
		// activation can require proof of a working login rather than trusting its caller. Recorded
		// only after the promotion succeeds, so a refused promotion leaves nothing to activate on.
		if err := apiContext.GatewayClient.SetTempUserCache(apiContext.Context(), user, name, namespace); err != nil {
			return s.errorToken(apiContext.Context(), tr, http.StatusConflict, err)
		}
		slog.Info("Completed staged auth provider verification", "tokenRequestID", tr.ID, "providerNamespace", namespace, "providerName", name, "userID", apiContext.UserID())
	} else {
		if _, err = apiContext.GatewayClient.CreateAPIKeyFromSetupTokenRequest(apiContext.Context(), apiContext.UserID(), tr); err != nil {
			return s.errorToken(apiContext.Context(), tr, http.StatusInternalServerError, err)
		}
		slog.Info("Completed OAuth redirect and issued auth token", "tokenRequestID", tr.ID, "providerNamespace", namespace, "providerName", name, "userID", apiContext.UserID())
	}

	if tr.CompletionRedirectURL == "" {
		tr.CompletionRedirectURL = s.authCompleteURL()
	}

	http.Redirect(apiContext.ResponseWriter, apiContext.Request, tr.CompletionRedirectURL, http.StatusFound)
	return nil
}

func (s *Server) errorToken(ctx context.Context, tr *types.TokenRequest, code int, err error) error {
	if tr != nil {
		tr.Error = err.Error()
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Updates(tr).Error
		}); err != nil {
			kcontext.GetLogger(ctx).Error("failed to update token", "id", tr.ID, "error", err)
		}
		slog.Info("Stored OAuth token flow error on token request", "tokenRequestID", tr.ID, "status", code)
	}

	return types2.NewErrHTTP(code, err.Error())
}
