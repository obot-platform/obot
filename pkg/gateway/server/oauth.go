package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
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

	// Check to make sure this auth provider exists.
	configuredProvider, err := s.dispatcher.GetConfiguredAuthProvider(apiContext.Context())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get configured auth provider: %v", err))
	}
	if configuredProvider != "" && configuredProvider != name {
		pkgLog.Infof("Rejected OAuth start for unconfigured auth provider: requestedProvider=%s configuredProvider=%s tokenRequestID=%s", name, configuredProvider, apiContext.PathValue("id"))
		return types2.NewErrHTTP(http.StatusNotFound, "auth provider not found")
	}

	state, err := s.createState(apiContext.Context(), apiContext.PathValue("id"))
	if err != nil {
		return fmt.Errorf("could not create state: %w", err)
	}
	pkgLog.Infof("Starting OAuth flow for token request: tokenRequestID=%s provider=%s/%s", apiContext.PathValue("id"), namespace, name)

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
		return types2.NewErrHTTP(http.StatusBadRequest, "no namespace path parameter provided")
	}

	name := apiContext.PathValue("name")
	if name == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "no name path parameter provided")
	}

	// Check to make sure this auth provider exists.
	configuredProvider, err := s.dispatcher.GetConfiguredAuthProvider(apiContext.Context())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get configured auth provider: %v", err))
	}
	if configuredProvider != "" && configuredProvider != name {
		pkgLog.Infof("Rejected OAuth redirect for unconfigured auth provider: requestedProvider=%s configuredProvider=%s", name, configuredProvider)
		return types2.NewErrHTTP(http.StatusNotFound, "auth provider not found")
	}

	tr, err := s.verifyState(apiContext.Context(), apiContext.FormValue("state"))
	if err != nil {
		return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("invalid state: %v", err))
	}

	if _, err = apiContext.GatewayClient.NewAuthToken(
		apiContext.Context(),
		namespace,
		name,
		apiContext.AuthProviderUserID(),
		apiContext.UserID(),
		tr,
	); err != nil {
		return s.errorToken(apiContext.Context(), tr, http.StatusInternalServerError, err)
	}
	pkgLog.Infof("Completed OAuth redirect and issued auth token: tokenRequestID=%s provider=%s/%s userID=%d", tr.ID, namespace, name, apiContext.UserID())

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
			kcontext.GetLogger(ctx).ErrorContext(ctx, "failed to update token", "id", tr.ID, "error", err)
		}
		pkgLog.Infof("Stored OAuth token flow error on token request: tokenRequestID=%s status=%d", tr.ID, code)
	}

	return types2.NewErrHTTP(code, err.Error())
}
