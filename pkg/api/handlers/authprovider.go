package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/auth"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CookieSecretEnvVar = "OBOT_AUTH_PROVIDER_COOKIE_SECRET"
)

type AuthProviderHandler struct {
	dispatcher  *dispatcher.Dispatcher
	postgresDSN string
	license     *license.Provider
}

func NewAuthProviderHandler(dispatcher *dispatcher.Dispatcher, postgresDSN string, licenseProvider *license.Provider) *AuthProviderHandler {
	return &AuthProviderHandler{
		dispatcher:  dispatcher,
		postgresDSN: postgresDSN,
		license:     licenseProvider,
	}
}

func (ap *AuthProviderHandler) ByID(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	authProviderStatus, err := providers.AuthProviderStatus(req.Context(), authProvider, nil, ap.license)
	if err != nil {
		return err
	}

	return req.Write(ap.convertAuthProvider(authProvider, *authProviderStatus))
}

func (ap *AuthProviderHandler) List(req api.Context) error {
	var authProviders v1.AuthProviderList
	if err := req.List(&authProviders, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	// This list is readable anonymously so the sign-in page can render, so a pending switch is
	// disclosed only to the administrators who can act on it.
	var staged, verifiedEmail string
	if req.UserIsAdmin() {
		var err error
		if staged, err = ap.dispatcher.GetStagedAuthProvider(req.Context()); err != nil {
			return err
		}
		if staged != "" {
			if cached := req.GatewayClient.GetTempUserCache(req.Context()); cached != nil && cached.AuthProviderName == staged {
				verifiedEmail = cached.Email
			}
		}
	}

	resp := make([]types.AuthProvider, 0, len(authProviders.Items))
	for _, a := range authProviders.Items {
		authProviderStatus, err := providers.AuthProviderStatus(req.Context(), a, nil, ap.license)
		if err != nil {
			return err
		}
		authProviderStatus.Staged = staged != "" && a.Name == staged
		if authProviderStatus.Staged {
			authProviderStatus.VerifiedEmail = verifiedEmail
		}

		resp = append(resp, ap.convertAuthProvider(a, *authProviderStatus))
	}

	return req.Write(types.AuthProviderList{Items: resp})
}

func (ap *AuthProviderHandler) Configure(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	if err := ap.license.RequireEntitlements(req.Context(), authProvider.Spec.RequiredEntitlements); err != nil {
		return err
	}

	if err := ensureNoPendingAuthProviderCleanup(req, authProvider); err != nil {
		return err
	}

	configuredProvider, err := ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configuredProvider != "" && configuredProvider != authProvider.Name {
		return types.NewErrBadRequest(
			"only one authentication provider can be configured at a time. Please deconfigure %q first",
			configuredProvider,
		)
	}
	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	} else if envVars == nil {
		envVars = make(map[string]string, 1)
	}

	envVars[CookieSecretEnvVar], err = generateCookieSecret()
	if err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	stagedName, err := stageProviderCredential(req, envVars)
	if err != nil {
		return err
	}

	return submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: authProvider.Namespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         authProvider.Name,
			DesiredState:         v1.ProviderDesiredStateConfigured,
			StagedCredentialName: stagedName,
		},
	})
}

func ensureNoPendingAuthProviderCleanup(req api.Context, authProvider v1.AuthProvider) error {
	var cleanups v1.AuthProviderCleanupList
	if err := req.List(&cleanups); err != nil {
		return fmt.Errorf("list pending auth provider cleanups: %w", err)
	}
	for _, cleanup := range cleanups.Items {
		sameProvider := cleanup.Spec.AuthProviderName == authProvider.Name
		samePrefix := authProvider.Spec.GroupIDPrefix != "" && cleanup.Spec.GroupIDPrefix == authProvider.Spec.GroupIDPrefix
		if sameProvider || samePrefix {
			return types.NewErrBadRequest("authentication provider %q is still being deconfigured; wait for cleanup to finish before configuring it again", authProvider.Name)
		}
	}
	return nil
}

func (ap *AuthProviderHandler) Deconfigure(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	// Turning off the provider serving logins leaves nobody able to sign in, and no session left to
	// undo it with. Switching is the supported way out, since it keeps this provider serving until
	// the replacement has been proven.
	configured, err := ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configured == authProvider.Name {
		return types.NewErrBadRequest(
			"deconfiguring %q would leave no way to sign in. Configure a replacement and complete the switch instead",
			authProvider.Name,
		)
	}

	return submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: authProvider.Namespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeAuth,
			ProviderName: authProvider.Name,
			DesiredState: v1.ProviderDesiredStateDeconfigured,
		},
	})
}

// Stage saves a replacement provider's settings without touching the active one. The settings live
// in their own credential context, so nothing here changes who serves logins.
func (ap *AuthProviderHandler) Stage(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	if err := ensureNoPendingAuthProviderCleanup(req, authProvider); err != nil {
		return err
	}

	// The controller checks whether anything is configured and whether something else is already
	// staged, so those run under the same serialization as the configure path.
	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	} else if envVars == nil {
		envVars = make(map[string]string, 1)
	}

	var err error
	envVars[CookieSecretEnvVar], err = generateCookieSecret()
	if err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	status, err := providers.AuthProviderStatus(req.Context(), authProvider, envVars, ap.license)
	if err != nil {
		return err
	}
	if len(status.MissingEntitlements) > 0 {
		return types.NewErrHTTP(http.StatusPaymentRequired,
			fmt.Sprintf("missing required license entitlements: %v", status.MissingEntitlements))
	}
	if !status.Configured {
		return types.NewErrBadRequest("missing required configuration parameters: %s",
			strings.Join(status.MissingConfigurationParameters, ", "))
	}

	stagedName, err := stageProviderCredential(req, envVars)
	if err != nil {
		return err
	}

	if err := submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: authProvider.Namespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         authProvider.Name,
			DesiredState:         v1.ProviderDesiredStateStaged,
			StagedCredentialName: stagedName,
		},
	}); err != nil {
		return err
	}

	// These settings are not the ones any earlier verification ran against, so its result no longer
	// describes what activation would promote.
	if err := req.GatewayClient.ClearTempUserCache(req.Context()); err != nil {
		return fmt.Errorf("failed to clear the verification for the previous settings: %w", err)
	}

	return nil
}

// Unstage discards a staged replacement provider, leaving the active provider untouched.
func (ap *AuthProviderHandler) Unstage(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	if err := submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: authProvider.Namespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeAuth,
			ProviderName: authProvider.Name,
			DesiredState: v1.ProviderDesiredStateUnstaged,
		},
	}); err != nil {
		return err
	}

	// Discarding the replacement retires its verification too, so a later staging of the same
	// provider cannot activate on proof of settings nobody kept.
	if err := req.GatewayClient.ClearTempUserCache(req.Context()); err != nil {
		return fmt.Errorf("failed to clear the verification for the discarded provider: %w", err)
	}

	clearAuthProviderVerifyCookie(req)
	return nil
}

// clearAuthProviderVerifyCookie retires the verification pin once the staged provider has been
// activated or discarded, so the window cannot be reused.
func clearAuthProviderVerifyCookie(req api.Context) {
	http.SetCookie(req.ResponseWriter, &http.Cookie{
		Name:     auth.AuthProviderVerifyCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// Verify starts a one-time login through the staged provider, authorized only for the owner
// requesting it. The identity it returns is granted the Owner role by the OAuth callback.
func (ap *AuthProviderHandler) Verify(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	staged, err := ap.dispatcher.GetStagedAuthProvider(req.Context())
	if err != nil {
		return err
	}
	if staged != authProvider.Name {
		return types.NewErrBadRequest("auth provider %q is not staged", authProvider.Name)
	}

	// A previous result describes settings that may since have been re-staged, and would make
	// caching this one fail.
	if err := req.GatewayClient.ClearTempUserCache(req.Context()); err != nil {
		return fmt.Errorf("failed to clear the previous verification: %w", err)
	}

	tokenID := uuid.New().String()
	// The verification belongs to the owner starting the switch, so only they can open the login
	// it authorizes.
	ownerUserID := req.UserID()
	if err := req.GatewayClient.CreateTokenRequest(req.Context(), &gatewaytypes.TokenRequest{
		ID:                    tokenID,
		Purpose:               gatewaytypes.TokenRequestPurposeAuthProviderVerify,
		CompletionRedirectURL: "/admin/auth-providers",
		RequestExpiresAt:      time.Now().Add(auth.AuthProviderVerifyWindow),
		OwnerUserID:           &ownerUserID,
	}); err != nil {
		return fmt.Errorf("failed to create verification token request: %w", err)
	}

	return req.Write(map[string]string{
		"redirectURL": fmt.Sprintf("%s/oauth/start/%s/%s/%s", req.APIBaseURL, tokenID, authProvider.Namespace, authProvider.Name),
	})
}

// Activate promotes the staged provider and deconfigures the outgoing one. It requires a recorded
// verification for the staged provider, which only a successful Verify produces.
func (ap *AuthProviderHandler) Activate(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	staged, err := ap.dispatcher.GetStagedAuthProvider(req.Context())
	if err != nil {
		return err
	}
	if staged != authProvider.Name {
		return types.NewErrBadRequest("auth provider %q is not staged", authProvider.Name)
	}

	cached := req.GatewayClient.GetTempUserCache(req.Context())
	if cached == nil || cached.AuthProviderName != authProvider.Name {
		return types.NewErrBadRequest("sign in through %q to verify it before completing the switch", authProvider.Name)
	}

	outgoing, err := ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}

	// One change carries the whole switch. The controller promotes the staged settings before
	// deconfiguring the outgoing provider, so a partial failure leaves a provider configured.
	if err := submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      system.ProviderChangeAuthName,
		Namespace: authProvider.Namespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeAuth,
			ProviderName:         authProvider.Name,
			DesiredState:         v1.ProviderDesiredStateSwitched,
			ReplacesProviderName: outgoing,
		},
	}); err != nil {
		return err
	}

	// Cleared only once the switch is submitted, so a rejected change does not cost the owner the
	// verification they already completed.
	if err := req.GatewayClient.ClearTempUserCache(req.Context()); err != nil {
		return fmt.Errorf("failed to clear the verification after the switch: %w", err)
	}

	clearAuthProviderVerifyCookie(req)
	return nil
}

func (ap *AuthProviderHandler) Reveal(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	// The replacement context comes last so a configured provider reveals the settings it is
	// running with, and reopening a staged one shows what was staged rather than an empty form.
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{
		authProvider.Name,
		system.GenericAuthProviderCredentialContext,
		system.ReplacementAuthProviderCredentialContext,
	}, authProvider.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal credential for auth provider %q: %w", authProvider.Name, err)
	} else if err == nil {
		return req.Write(cred.Secrets)
	}

	return types.NewErrNotFound("no credential found for %q", authProvider.Name)
}

func (ap *AuthProviderHandler) convertAuthProvider(authProvider v1.AuthProvider, authProviderStatus types.AuthProviderStatus) types.AuthProvider {
	return types.AuthProvider{
		Metadata:             MetadataFrom(&authProvider),
		AuthProviderManifest: authProvider.Spec.AuthProviderManifest,
		AuthProviderStatus:   authProviderStatus,
	}
}

func generateCookieSecret() (string, error) {
	const length = 32

	// Generate a random token. Repeat until we have one that is 32 bytes long after trimming.
	// This only takes one try in the vast majority of circumstances, but could occasionally take a second.
	var b = make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	for len(bytes.TrimSpace(b)) != length {
		_, err := rand.Read(b)
		if err != nil {
			return "", fmt.Errorf("failed to generate random token: %w", err)
		}
	}

	return base64.StdEncoding.EncodeToString(b), nil
}
