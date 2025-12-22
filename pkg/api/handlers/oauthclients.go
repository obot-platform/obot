package handlers

import (
	"crypto/rand"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/crypto/bcrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type OAuthClientsHandler struct {
	oauthServerConfig OAuthAuthorizationServerConfig
	serverURL         string
}

func NewOAuthClientsHandler(oauthServerConfig OAuthAuthorizationServerConfig, serverURL string) *OAuthClientsHandler {
	return &OAuthClientsHandler{
		oauthServerConfig: oauthServerConfig,
		serverURL:         serverURL,
	}
}

// List handles the GET /api/oauth-clients endpoint.
func (h *OAuthClientsHandler) List(req api.Context) error {
	var selector map[string]string
	if req.URL.Query().Get("all") != "true" {
		selector = map[string]string{"spec.static": "true"}
	}
	var oauthClients v1.OAuthClientList
	if err := req.List(&oauthClients, kclient.MatchingFieldsSelector{Selector: fields.SelectorFromSet(selector)}); err != nil {
		return err
	}

	clients := make([]types.OAuthClient, 0, len(oauthClients.Items))
	for _, client := range oauthClients.Items {
		clients = append(clients, ConvertClient(client, h.serverURL, ""))
	}

	return req.Write(types.OAuthClientList{Items: clients})
}

// Get handles the GET /api/oauth-clients/{client_id} endpoint.
func (h *OAuthClientsHandler) Get(req api.Context) error {
	namespace, name, ok := strings.Cut(req.PathValue("client_id"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client ID: %s", req.PathValue("client_id"))
	}

	var oauthClient v1.OAuthClient
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: namespace, Name: name}, &oauthClient); err != nil {
		return err
	}

	return req.Write(ConvertClient(oauthClient, h.serverURL, ""))
}

// Create handles the POST /api/oauth-clients endpoint.
func (h *OAuthClientsHandler) Create(req api.Context) error {
	var input types.OAuthClientManifest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to read request body: %v", err)
	}

	client := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.OAuthClientPrefix + strings.ToLower(rand.Text()),
			Namespace: req.Namespace(),
		},
		Spec: v1.OAuthClientSpec{
			Manifest: input,
			Static:   true,
		},
	}

	err := ValidateClientConfig(&client, h.oauthServerConfig)
	if err != nil {
		return types.NewErrBadRequest("%v", err)
	}

	clientSecret := rand.Text() + rand.Text()
	client.Spec.ClientSecretHash, err = bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to generate client secret hash: %v", err)
	}

	if err := req.Create(&client); err != nil {
		return err
	}

	return req.Write(ConvertClient(client, h.serverURL, clientSecret))
}

// Update handles the PUT /api/oauth-clients/{client_id} endpoint.
func (h *OAuthClientsHandler) Update(req api.Context) error {
	namespace, name, ok := strings.Cut(req.PathValue("client_id"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client ID: %s", req.PathValue("client_id"))
	}

	var input types.OAuthClientManifest
	if err := req.Read(&input); err != nil {
		return err
	}

	var client v1.OAuthClient
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: namespace, Name: name}, &client); err != nil {
		return err
	}

	client.Spec.Manifest = input

	if err := ValidateClientConfig(&client, h.oauthServerConfig); err != nil {
		return err
	}

	if err := req.Update(&client); err != nil {
		return err
	}

	return req.Write(ConvertClient(client, h.serverURL, ""))
}

// Delete handles the DELETE /api/oauth-clients/{client_id} endpoint.
func (h *OAuthClientsHandler) Delete(req api.Context) error {
	namespace, name, ok := strings.Cut(req.PathValue("client_id"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client ID: %s", req.PathValue("client_id"))
	}
	return req.Delete(&v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	})
}

// RollClientSecret handles the POST /api/oauth-clients/{client_id}/roll-secret endpoint.
func (h *OAuthClientsHandler) RollClientSecret(req api.Context) error {
	namespace, name, ok := strings.Cut(req.PathValue("client_id"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client name: %s", req.PathValue("client_id"))
	}

	var client v1.OAuthClient
	err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: namespace, Name: name}, &client)
	if err != nil {
		return err
	}

	clientSecret := rand.Text() + rand.Text()
	client.Spec.ClientSecretHash, err = bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to generate client secret hash: %v", err)
	}

	if err := req.Update(&client); err != nil {
		return err
	}

	return req.Write(ConvertClient(client, h.serverURL, clientSecret))
}

func ValidateClientConfig(oauthClient *v1.OAuthClient, oauthConfig OAuthAuthorizationServerConfig) error {
	//nolint: staticcheck
	if oauthClient.Spec.Manifest.RedirectURI != "" {
		oauthClient.Spec.Manifest.RedirectURIs = append(oauthClient.Spec.Manifest.RedirectURIs, oauthClient.Spec.Manifest.RedirectURI)
	}
	if len(oauthClient.Spec.Manifest.RedirectURIs) == 0 {
		return fmt.Errorf("redirect_uris is required")
	}
	if oauthClient.Spec.Manifest.TokenEndpointAuthMethod != "" && !slices.Contains(oauthConfig.TokenEndpointAuthMethodsSupported, oauthClient.Spec.Manifest.TokenEndpointAuthMethod) {
		return fmt.Errorf("token_endpoint_auth_method must be %s, not %s", strings.Join(oauthConfig.TokenEndpointAuthMethodsSupported, ", "), oauthClient.Spec.Manifest.TokenEndpointAuthMethod)
	}
	for scope := range strings.SplitSeq(oauthClient.Spec.Manifest.Scope, " ") {
		if scope != "" && !slices.Contains(oauthConfig.ScopesSupported, scope) {
			return fmt.Errorf("scope %s is not supported", scope)
		}
	}

	return nil
}

func ConvertClient(oauthClient v1.OAuthClient, baseURL, clientSecret string) types.OAuthClient {
	client := ConvertDynamicClient(oauthClient, baseURL, clientSecret, "")
	client.AuthorizeURL = fmt.Sprintf("%s/oauth/authorize", baseURL)
	client.TokenURL = fmt.Sprintf("%s/oauth/token", baseURL)
	return client
}

func ConvertDynamicClient(oauthClient v1.OAuthClient, baseURL, clientSecret, registrationToken string) types.OAuthClient {
	clientID := fmt.Sprintf("%s:%s", oauthClient.Namespace, oauthClient.Name)
	var registrationURI string
	if registrationToken != "" {
		registrationURI = fmt.Sprintf("%s/oauth/register/%s", baseURL, clientID)
	}
	return types.OAuthClient{
		Metadata:                   MetadataFrom(&oauthClient),
		OAuthClientManifest:        oauthClient.Spec.Manifest,
		RegistrationAccessToken:    registrationToken,
		RegistrationClientURI:      registrationURI,
		RegistrationTokenIssuedAt:  max(oauthClient.Spec.RegistrationTokenIssuedAt.Unix(), 0),
		RegistrationTokenExpiresAt: max(oauthClient.Spec.RegistrationTokenExpiresAt.Unix(), 0),
		ClientID:                   clientID,
		ClientSecret:               clientSecret,
		ClientSecretIssuedAt:       max(oauthClient.Spec.ClientSecretIssuedAt.Unix(), 0),
		ClientSecretExpiresAt:      max(oauthClient.Spec.ClientSecretExpiresAt.Unix(), 0),
		Static:                     oauthClient.Spec.Static,
	}
}

// OAuthAuthorizationServerConfig represents the response from /.well-known/oauth-authorization-server
// as defined in RFC 8414 (OAuth 2.0 Authorization Server Metadata)
type OAuthAuthorizationServerConfig struct {
	// Issuer is the authorization server's issuer identifier, which is a URL that uses the "https" scheme
	// and has no query or fragment components. REQUIRED.
	Issuer string `json:"issuer"`
	// AuthorizationEndpoint is the URL of the authorization server's authorization endpoint.
	// REQUIRED unless no grant types are supported that use the authorization endpoint.
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	// TokenEndpoint is the URL of the authorization server's token endpoint.
	// REQUIRED unless only the implicit grant type is supported.
	TokenEndpoint string `json:"token_endpoint"`
	// JWKSURI is the URL of the authorization server's JWK Set document.
	// OPTIONAL.
	JWKSURI string `json:"jwks_uri,omitempty"`
	// RegistrationEndpoint is the URL of the authorization server's OAuth 2.0 Dynamic Client Registration endpoint.
	// OPTIONAL.
	RegistrationEndpoint string `json:"registration_endpoint,omitempty"`
	// ScopesSupported is a JSON array containing a list of the OAuth 2.0 scope values that this authorization server supports.
	// RECOMMENDED.
	ScopesSupported []string `json:"scopes_supported,omitempty"`
	// ResponseTypesSupported is a JSON array containing a list of the OAuth 2.0 response_type values that this authorization server supports.
	// REQUIRED.
	ResponseTypesSupported []string `json:"response_types_supported"`
	// ResponseModesSupported is a JSON array containing a list of the OAuth 2.0 response_mode values that this authorization server supports.
	// OPTIONAL. Default is ["query", "fragment"].
	ResponseModesSupported []string `json:"response_modes_supported,omitempty"`
	// GrantTypesSupported is a JSON array containing a list of the OAuth 2.0 grant type values that this authorization server supports.
	// OPTIONAL. Default is ["authorization_code", "implicit"].
	GrantTypesSupported []string `json:"grant_types_supported,omitempty"`
	// TokenEndpointAuthMethodsSupported is a JSON array containing a list of client authentication methods supported by this token endpoint.
	// OPTIONAL. Default is "client_secret_basic".
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	// TokenEndpointAuthSigningAlgValuesSupported is a JSON array containing a list of the JWS signing algorithms supported by the token endpoint.
	// OPTIONAL. Required if "private_key_jwt" or "client_secret_jwt" authentication methods are specified.
	TokenEndpointAuthSigningAlgValuesSupported []string `json:"token_endpoint_auth_signing_alg_values_supported,omitempty"`
	// ServiceDocumentation is the URL of a page containing human-readable information that developers might want or need to know.
	// OPTIONAL.
	ServiceDocumentation string `json:"service_documentation,omitempty"`
	// UILocalesSupported is a JSON array of language tag values from BCP 47 for the user interface.
	// OPTIONAL.
	UILocalesSupported []string `json:"ui_locales_supported,omitempty"`
	// OPPolicyURI is the URL that the authorization server provides to read about the authorization server's requirements.
	// OPTIONAL.
	OPPolicyURI string `json:"op_policy_uri,omitempty"`
	// OPTosURI is the URL that the authorization server provides to read about the authorization server's terms of service.
	// OPTIONAL.
	OPTosURI string `json:"op_tos_uri,omitempty"`
	// RevocationEndpoint is the URL of the authorization server's OAuth 2.0 revocation endpoint.
	// OPTIONAL.
	RevocationEndpoint string `json:"revocation_endpoint,omitempty"`
	// RevocationEndpointAuthMethodsSupported is a JSON array containing a list of client authentication methods supported by this revocation endpoint.
	// OPTIONAL. Default is "client_secret_basic".
	RevocationEndpointAuthMethodsSupported []string `json:"revocation_endpoint_auth_methods_supported,omitempty"`
	// IntrospectionEndpoint is the URL of the authorization server's OAuth 2.0 introspection endpoint.
	// OPTIONAL.
	IntrospectionEndpoint string `json:"introspection_endpoint,omitempty"`
	// IntrospectionEndpointAuthMethodsSupported is a JSON array containing a list of client authentication methods supported by this introspection endpoint.
	// OPTIONAL.
	IntrospectionEndpointAuthMethodsSupported []string `json:"introspection_endpoint_auth_methods_supported,omitempty"`
	// IntrospectionEndpointAuthSigningAlgValuesSupported is a JSON array containing a list of the JWS signing algorithms supported by the introspection endpoint.
	// OPTIONAL. Required if "private_key_jwt" or "client_secret_jwt" authentication methods are specified.
	IntrospectionEndpointAuthSigningAlgValuesSupported []string `json:"introspection_endpoint_auth_signing_alg_values_supported,omitempty"`
	// CodeChallengeMethodsSupported is a JSON array containing a list of PKCE code challenge methods supported by this authorization server.
	// OPTIONAL. If omitted, the authorization server does not support PKCE.
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported,omitempty"`

	// Additional fields can be added here
	UserInfoEndpoint string `json:"userinfo_endpoint,omitempty"`
}
