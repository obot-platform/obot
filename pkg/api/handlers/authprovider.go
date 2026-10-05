package handlers

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
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
	"github.com/obot-platform/obot/pkg/scim/adapter"
	scimsetup "github.com/obot-platform/obot/pkg/scim/setup"
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

	var (
		configured bool
		conn       *gatewaytypes.SCIMConnection
	)
	if seesSCIM(req) {
		configuredProvider, err := ap.dispatcher.GetConfiguredAuthProvider(req.Context())
		if err != nil {
			return fmt.Errorf("failed to get configured auth provider: %w", err)
		}
		configured = authProvider.Name == configuredProvider

		if conn, err = req.GatewayClient.SCIMConnectionForAuthProvider(req.Context(), authProvider.Namespace, authProvider.Name); err != nil {
			return err
		}
	}

	authProviderStatus, err := providers.AuthProviderStatus(req.Context(), authProvider, nil, conn, ap.license)
	if err != nil {
		return err
	}
	setSCIMStatus(req, authProvider, conn, authProviderStatus)
	if err := setRequiresActivation(req, authProvider, authProviderStatus); err != nil {
		return err
	}

	params, err := servedAuthProviderParameters(req, authProvider, configured, conn)
	if err != nil {
		return err
	}

	return req.Write(ap.convertAuthProvider(authProvider, *authProviderStatus, params))
}

// effectiveAuthProviderParameters returns the auth provider's effective configuration parameters. configured is set
// for the configured auth provider. They depend on the provider's SCIM connection, and with one, on whether the
// stored credential still holds the parameters that only directory synchronization uses.
func effectiveAuthProviderParameters(req api.Context, authProvider v1.AuthProvider, configured bool, conn *gatewaytypes.SCIMConnection) (adapter.Parameters, error) {
	var stored map[string]string
	if conn != nil {
		var err error
		if stored, err = scimsetup.StoredConfiguration(req.Context(), req.GatewayClient, authProvider); err != nil {
			return adapter.Parameters{}, err
		}
	}
	return adapter.EffectiveParameters(authProvider.Spec.AuthProviderManifest, adapter.ProviderState{
		AuthProviderName:      authProvider.Name,
		Configured:            configured,
		ConnectionAdapterType: adapter.ConnectionAdapterType(conn),
		Stored:                stored,
	}), nil
}

// servedAuthProviderParameters returns the configuration parameters to serve for an auth provider. Administrators and
// auditors get its effective parameters, which the configuration form shows. Everyone else gets the manifest's,
// because the effective ones reveal the provider's SCIM setup, and auth providers are readable anonymously so the
// sign-in page can render.
func servedAuthProviderParameters(req api.Context, authProvider v1.AuthProvider, configured bool, conn *gatewaytypes.SCIMConnection) (adapter.Parameters, error) {
	if !seesSCIM(req) {
		return adapter.Parameters{
			Required: authProvider.Spec.RequiredConfigurationParameters,
			Optional: authProvider.Spec.OptionalConfigurationParameters,
		}, nil
	}
	return effectiveAuthProviderParameters(req, authProvider, configured, conn)
}

// seesSCIM reports whether the requester may learn how an auth provider is set up for SCIM.
func seesSCIM(req api.Context) bool {
	return req.UserIsAdmin() || req.UserIsAuditor()
}

// setSCIMStatus reports how the provider supports SCIM and the state of its SCIM connection. Auth providers are
// readable anonymously so the sign-in page can render, so only administrators and auditors learn either.
func setSCIMStatus(req api.Context, authProvider v1.AuthProvider, conn *gatewaytypes.SCIMConnection, status *types.AuthProviderStatus) {
	if !seesSCIM(req) {
		return
	}
	if conn != nil {
		status.SCIMState = string(conn.State)
		if expiresAt := conn.TokenExpiresAt(); expiresAt != nil {
			status.SCIMTokenExpiresAt = types.NewTime(*expiresAt)
		}
	}

	a, ok := adapter.ForAuthProvider(authProvider.Name)
	if !ok || !adapter.SupportsSCIM(authProvider.Name, authProvider.Spec.AuthProviderManifest) {
		return
	}
	directory := a.DirectoryParameters()
	status.SCIM = &types.AuthProviderSCIM{
		IssuerParameter:     a.IssuerConfigurationParameter(),
		DirectoryParameters: make([]string, 0, len(directory)),
	}
	for _, d := range directory {
		status.SCIM.DirectoryParameters = append(status.SCIM.DirectoryParameters, d.Name)
	}
	if conn != nil {
		status.SCIM.ConnectionIssuer = conn.Issuer
	}
}

func setRequiresActivation(req api.Context, authProvider v1.AuthProvider, status *types.AuthProviderStatus) error {
	if authProvider.Name != system.LocalAuthProvider || !status.Configured {
		return nil
	}
	requiresActivation, err := req.GatewayClient.LocalAuthRequiresActivation(req.Context())
	status.RequiresActivation = requiresActivation
	return err
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

	var (
		configuredProvider string
		conns              []gatewaytypes.SCIMConnection
	)
	if seesSCIM(req) {
		var err error
		if configuredProvider, err = ap.dispatcher.GetConfiguredAuthProvider(req.Context()); err != nil {
			return fmt.Errorf("failed to get configured auth provider: %w", err)
		}
		if conns, err = req.GatewayClient.SCIMConnections(req.Context()); err != nil {
			return err
		}
	}

	resp := make([]types.AuthProvider, 0, len(authProviders.Items))
	for _, a := range authProviders.Items {
		var conn *gatewaytypes.SCIMConnection
		if i := slices.IndexFunc(conns, func(c gatewaytypes.SCIMConnection) bool {
			return c.AuthProviderNamespace == a.Namespace && c.AuthProviderName == a.Name
		}); i >= 0 {
			conn = &conns[i]
		}

		authProviderStatus, err := providers.AuthProviderStatus(req.Context(), a, nil, conn, ap.license)
		if err != nil {
			return err
		}
		setSCIMStatus(req, a, conn, authProviderStatus)
		authProviderStatus.Staged = staged != "" && a.Name == staged
		if authProviderStatus.Staged {
			authProviderStatus.VerifiedEmail = verifiedEmail
		}
		if err := setRequiresActivation(req, a, authProviderStatus); err != nil {
			return err
		}

		params, err := servedAuthProviderParameters(req, a, a.Name == configuredProvider, conn)
		if err != nil {
			return err
		}

		resp = append(resp, ap.convertAuthProvider(a, *authProviderStatus, params))
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

	if err := ap.validateAuthProviderConfiguration(req, authProvider, configuredProvider == authProvider.Name, envVars); err != nil {
		return err
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

// validateAuthProviderConfiguration checks a submitted configuration against the auth provider's SCIM rules, which
// the provider configuration change applies again, and against its effective parameters. configured is set for the
// configured auth provider. It first drops the parameters the provider no longer uses, so that a provider whose
// directory is managed through SCIM is configured again without them.
//
// For a provider that the configuration would set up for SCIM, it also refuses residual group data here, so that the
// administrator sees what remains. The provider configuration change checks it again under its serialization.
func (ap *AuthProviderHandler) validateAuthProviderConfiguration(req api.Context, authProvider v1.AuthProvider, configured bool, envVars map[string]string) error {
	params, setup, err := scimsetup.CheckConfiguration(req.Context(), req.GatewayClient, authProvider, func() (bool, error) {
		return configured, nil
	}, func() (map[string]string, error) {
		return scimsetup.StoredConfiguration(req.Context(), req.GatewayClient, authProvider)
	}, envVars)
	if refused, ok := errors.AsType[*scimsetup.ConfigurationError](err); ok {
		return types.NewErrBadRequest("%s", refused.Message)
	} else if err != nil {
		return err
	}

	missingEntitlements, err := ap.license.MissingEntitlements(req.Context(), authProvider.Spec.RequiredEntitlements)
	if err != nil {
		return err
	}
	if len(missingEntitlements) > 0 {
		return types.NewErrHTTP(http.StatusPaymentRequired,
			fmt.Sprintf("missing required license entitlements: %v", missingEntitlements))
	}

	var missing []string
	for _, param := range params.Required {
		if _, ok := envVars[param.Name]; !ok {
			missing = append(missing, param.Name)
		}
	}
	if len(missing) > 0 {
		return types.NewErrBadRequest("missing required configuration parameters: %s", strings.Join(missing, ", "))
	}

	if setup == scimsetup.SetupSCIMFirst {
		residual, err := scimsetup.ResidualGroupData(req.Context(), req.Storage, req.GatewayClient, authProvider)
		if err != nil {
			return err
		}
		if len(residual.Groups) > 0 || residual.MembershipCount > 0 {
			return types.NewErrHTTP(http.StatusConflict, (&scimsetup.ResidualGroupDataError{
				AuthProviderName:        authProvider.Name,
				AuthProviderDisplayName: cmp.Or(authProvider.Spec.Name, authProvider.Name),
				Data:                    *residual,
			}).Error())
		}
	}
	return nil
}

// GET /api/auth-providers/{id}/residual-group-data
// Reports what remains of an auth provider's groups from an earlier configuration. It blocks configuring the provider
// without directory credentials until the provider's auth provider cleanup, which deconfiguring runs, removes it.
func (ap *AuthProviderHandler) ResidualGroupData(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	residual, err := scimsetup.ResidualGroupData(req.Context(), req.Storage, req.GatewayClient, authProvider)
	if ineligible, ok := errors.AsType[*scimsetup.IneligibleError](err); ok {
		return types.NewErrBadRequest("%s", ineligible.Error())
	} else if err != nil {
		return err
	}
	return req.Write(residual)
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

// POST /api/auth-providers/{id}/deconfigure
// Deconfigures an auth provider. It is refused for the provider serving logins, which a switch replaces instead, and
// for a staged provider, whose staging is discarded instead.
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
	// Deconfiguring would leave the staging in place without what it set up, such as its SCIM connection.
	staged, err := ap.dispatcher.GetStagedAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get staged auth provider: %w", err)
	}
	if staged == authProvider.Name {
		return types.NewErrBadRequest("%q is staged as a replacement. Discard the staged switch instead", authProvider.Name)
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

	// The active provider is never a replacement. The controller refuses it too, but the settings are validated
	// first as a replacement's, which would report the active provider's own groups as left from an earlier
	// configuration.
	configuredProvider, err := ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configuredProvider == authProvider.Name {
		return types.NewErrBadRequest("%q is already the active authentication provider", authProvider.Name)
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

	envVars[CookieSecretEnvVar], err = generateCookieSecret()
	if err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := ap.validateAuthProviderConfiguration(req, authProvider, false, envVars); err != nil {
		return err
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

	auth.ClearAuthProviderVerifyCookie(req.ResponseWriter)
	return nil
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
		CompletionRedirectURL: "/identity-access?view=auth-providers",
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

	auth.ClearAuthProviderVerifyCookie(req.ResponseWriter)
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

// convertAuthProvider returns the API representation of an auth provider, which lists its effective configuration
// parameters, so that the configuration form needs no SCIM logic.
func (ap *AuthProviderHandler) convertAuthProvider(authProvider v1.AuthProvider, authProviderStatus types.AuthProviderStatus, params adapter.Parameters) types.AuthProvider {
	manifest := *authProvider.Spec.AuthProviderManifest.DeepCopy()
	manifest.RequiredConfigurationParameters = params.Required
	manifest.OptionalConfigurationParameters = params.Optional

	return types.AuthProvider{
		Metadata:             MetadataFrom(&authProvider),
		AuthProviderManifest: manifest,
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
