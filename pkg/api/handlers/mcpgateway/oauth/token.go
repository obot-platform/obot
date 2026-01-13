package oauth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/crypto/bcrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const (
	tokenExpiration         = 10 * time.Minute
	tokenTypeJWT            = "urn:ietf:params:oauth:token-type:jwt"
	tokenTypeAccessToken    = "urn:ietf:params:oauth:token-type:access_token"
	tokenTypeAPIKey         = "urn:obot:token-type:api-key"
	ErrUnsupportedGrantType = ErrorCode("unsupported_grant_type")
)

// TokenExchangeResponse represents an RFC 8693 token exchange response
type TokenExchangeResponse struct {
	AccessToken     string `json:"access_token"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
}

func (h *handler) token(req api.Context) error {
	if err := req.ParseForm(); err != nil {
		return types.NewErrBadRequest("failed to parse request body: %v", err)
	}

	var clientSecret string
	clientID := req.FormValue("client_id")
	if clientID == "" {
		creds := strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Basic ")
		if creds == "" {
			return types.NewErrHTTP(http.StatusUnauthorized, "Invalid client credentials")
		}

		c, err := base64.StdEncoding.DecodeString(creds)
		if err != nil {
			return types.NewErrHTTP(http.StatusUnauthorized, "Invalid client credentials")
		}

		idx := bytes.LastIndex(c, []byte{':'})
		if idx == -1 {
			return types.NewErrHTTP(http.StatusUnauthorized, "Invalid client credentials")
		}

		clientID, clientSecret = string(c[:idx]), string(c[idx+1:])
		if clientID == "" {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "client_id is required",
			})
		}

		clientID, err = url.QueryUnescape(clientID)
		if err != nil {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "client_id is invalid",
			})
		}
	} else {
		clientSecret = req.FormValue("client_secret")
	}

	clientNamespace, clientName, ok := strings.Cut(clientID, ":")
	if !ok {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "client_id is invalid",
		})
	}

	var client v1.OAuthClient
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: clientNamespace, Name: clientName}, &client); err != nil {
		return err
	}

	switch client.Spec.Manifest.TokenEndpointAuthMethod {
	case "client_secret_basic", "client_secret_post":
		if bcrypt.CompareHashAndPassword(client.Spec.ClientSecretHash, []byte(clientSecret)) != nil {
			return types.NewErrHTTP(http.StatusUnauthorized, "Invalid client credentials")
		}
	}

	grantType := req.FormValue("grant_type")
	if !slices.Contains(h.oauthConfig.GrantTypesSupported, grantType) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: fmt.Sprintf("grant_type must be one of %s, not %s", strings.Join(h.oauthConfig.GrantTypesSupported, ", "), grantType),
		})
	}

	if len(client.Spec.Manifest.GrantTypes) > 0 && !slices.Contains(client.Spec.Manifest.GrantTypes, grantType) || len(client.Spec.Manifest.GrantTypes) == 0 && grantType != "authorization_code" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "client is not allowed to use authorization_code grant type",
		})
	}

	switch grantType {
	case "authorization_code":
		return h.doAuthorizationCode(req, client, req.FormValue("code"), req.FormValue("code_verifier"))
	case "refresh_token":
		return h.doRefreshToken(req, client, req.FormValue("refresh_token"))
	case "urn:ietf:params:oauth:grant-type:token-exchange":
		return h.doTokenExchange(req, client, req.FormValue("resource"), req.FormValue("subject_token"), req.FormValue("subject_token_type"), req.FormValue("requested_token_type"))
	default:
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: fmt.Sprintf("grant_type must be one of %s, not %s", strings.Join(h.oauthConfig.GrantTypesSupported, ", "), grantType),
		})
	}
}

func (h *handler) doAuthorizationCode(req api.Context, oauthClient v1.OAuthClient, code, codeVerifier string) error {
	if code == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "code is required",
		})
	}

	var oauthAuthRequestList v1.OAuthAuthRequestList
	if err := req.Storage.List(req.Context(), &oauthAuthRequestList, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.hashedAuthCode": fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
		})),
	}); err != nil {
		return err
	}
	if len(oauthAuthRequestList.Items) != 1 {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "code is invalid",
		})
	}

	oauthAuthRequest := oauthAuthRequestList.Items[0]

	// Authorization codes are one-time use
	if err := req.Storage.Delete(req.Context(), &oauthAuthRequest); err != nil {
		// Don't return an error if we can't delete the auth request
		log.Warnf("failed to delete auth request: %v", err)
	}

	if oauthAuthRequest.Spec.CodeChallenge != "" {
		switch oauthAuthRequest.Spec.CodeChallengeMethod {
		case "S256":
			hashedCodeVerifier := sha256.Sum256([]byte(codeVerifier))
			if oauthAuthRequest.Spec.CodeChallenge != base64.RawURLEncoding.EncodeToString(hashedCodeVerifier[:]) {
				return types.NewErrBadRequest("%v", Error{
					Code:        ErrInvalidRequest,
					Description: "code_verifier is invalid",
				})
			}
		case "plain":
			if oauthAuthRequest.Spec.CodeChallenge != codeVerifier {
				return types.NewErrBadRequest("%v", Error{
					Code:        ErrInvalidRequest,
					Description: "code_verifier is invalid",
				})
			}
		default:
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "code_challenge_method must be S256 or plain. ",
			})
		}
	}

	userID := fmt.Sprintf("%d", oauthAuthRequest.Spec.UserID)
	user, err := req.GatewayClient.UserByID(req.Context(), userID)
	if err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "invalid user",
		})
	}

	now := time.Now()
	tknCtx := persistent.TokenContext{
		Audience:              oauthAuthRequest.Spec.Resource,
		OAuthScope:            oauthAuthRequest.Spec.Scope,
		IssuedAt:              now,
		ExpiresAt:             now.Add(tokenExpiration),
		UserID:                userID,
		UserName:              user.Username,
		UserEmail:             user.Email,
		Picture:               user.IconURL,
		UserGroups:            user.Role.Groups(),
		AuthProviderName:      oauthAuthRequest.Spec.AuthProviderName,
		AuthProviderNamespace: oauthAuthRequest.Spec.AuthProviderNamespace,
		AuthProviderUserID:    oauthAuthRequest.Spec.AuthProviderUserID,
		MCPID:                 oauthAuthRequest.Spec.MCPID,
	}
	tkn, err := h.tokenService.NewToken(req.Context(), tknCtx)
	if err != nil {
		return fmt.Errorf("failed to create auth token: %w", err)
	}

	refreshToken := strings.ToLower(rand.Text() + rand.Text())

	oauthToken := v1.OAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oauthClient.Namespace,
			Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
		},
		Spec: v1.OAuthTokenSpec{
			ClientID:              oauthClient.Name,
			Resource:              oauthAuthRequest.Spec.Resource,
			UserID:                oauthAuthRequest.Spec.UserID,
			Scope:                 oauthAuthRequest.Spec.Scope,
			AuthProviderNamespace: oauthAuthRequest.Spec.AuthProviderNamespace,
			AuthProviderName:      oauthAuthRequest.Spec.AuthProviderName,
			AuthProviderUserID:    oauthAuthRequest.Spec.AuthProviderUserID,
			MCPID:                 oauthAuthRequest.Spec.MCPID,
		},
	}

	if err = req.Create(&oauthToken); err != nil {
		return fmt.Errorf("failed to create oauth token: %w", err)
	}

	return req.Write(types.OAuthToken{
		AccessToken:  tkn,
		TokenType:    "bearer",
		ExpiresIn:    int(time.Until(tknCtx.ExpiresAt).Milliseconds() / 1000),
		RefreshToken: refreshToken,
	})
}

func (h *handler) doRefreshToken(req api.Context, oauthClient v1.OAuthClient, refreshToken string) error {
	if refreshToken == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "refresh_token is required",
		})
	}

	var oauthToken v1.OAuthToken
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: oauthClient.Namespace, Name: fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))}, &oauthToken); err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "refresh_token is invalid",
		})
	}

	if err := req.Delete(&oauthToken); err != nil {
		return fmt.Errorf("failed to refresh oauth token: %w", err)
	}

	userID := fmt.Sprintf("%d", oauthToken.Spec.UserID)
	user, err := req.GatewayClient.UserByID(req.Context(), userID)
	if err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "invalid user",
		})
	}

	// If this is an MCP server instance and the resource isn't the MCP server, then update it to the MCP server.
	if system.IsMCPServerInstanceID(oauthToken.Spec.MCPID) && strings.HasSuffix(oauthToken.Spec.Resource, "/"+oauthToken.Spec.MCPID) {
		// If this is an MCP server instance ID, we need to get the MCP server ID
		var mcpServerInstance v1.MCPServerInstance
		if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: oauthClient.Namespace, Name: oauthToken.Spec.MCPID}, &mcpServerInstance); err != nil {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "invalid MCP server",
			})
		}

		oauthToken.Spec.Resource = fmt.Sprintf("%s/mcp-connect/%s", h.baseURL, mcpServerInstance.Spec.MCPServerName)
	}

	now := time.Now()
	tknCtx := persistent.TokenContext{
		Scope:                 oauthToken.Spec.Scope,
		Audience:              oauthToken.Spec.Resource,
		IssuedAt:              now,
		ExpiresAt:             now.Add(tokenExpiration),
		UserID:                userID,
		UserName:              user.Username,
		UserEmail:             user.Email,
		Picture:               user.IconURL,
		UserGroups:            user.Role.Groups(),
		AuthProviderName:      oauthToken.Spec.AuthProviderName,
		AuthProviderNamespace: oauthToken.Spec.AuthProviderNamespace,
		AuthProviderUserID:    oauthToken.Spec.AuthProviderUserID,
		MCPID:                 oauthToken.Spec.MCPID,
	}
	tkn, err := h.tokenService.NewToken(req.Context(), tknCtx)
	if err != nil {
		return fmt.Errorf("failed to create auth token: %w", err)
	}

	refreshToken = strings.ToLower(rand.Text() + rand.Text())

	oauthToken = v1.OAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oauthClient.Namespace,
			Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
		},
		Spec: v1.OAuthTokenSpec{
			Resource:              oauthToken.Spec.Resource,
			ClientID:              oauthClient.Name,
			UserID:                oauthToken.Spec.UserID,
			AuthProviderNamespace: oauthToken.Spec.AuthProviderNamespace,
			AuthProviderName:      oauthToken.Spec.AuthProviderName,
			AuthProviderUserID:    oauthToken.Spec.AuthProviderUserID,
			MCPID:                 oauthToken.Spec.MCPID,
		},
	}

	if err = req.Create(&oauthToken); err != nil {
		return fmt.Errorf("failed to create new oauth token: %w", err)
	}

	return req.Write(types.OAuthToken{
		AccessToken:  tkn,
		TokenType:    "bearer",
		ExpiresIn:    int(time.Until(tknCtx.ExpiresAt).Milliseconds() / 1000),
		RefreshToken: refreshToken,
	})
}

func (h *handler) doTokenExchange(req api.Context, oauthClient v1.OAuthClient, resource, subjectToken, subjectTokenType, requestedTokenType string) error {
	if subjectToken == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "subject_token is required",
		})
	}

	if subjectTokenType != tokenTypeJWT && subjectTokenType != tokenTypeAPIKey {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "subject_token_type must be urn:ietf:params:oauth:token-type:jwt or urn:obot:token-type:api-key",
		})
	}

	// Validate optional requested_token_type parameter
	if requestedTokenType != "" && requestedTokenType != tokenTypeAccessToken {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "requested_token_type must be urn:ietf:params:oauth:token-type:access_token",
		})
	}

	if resource == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "resource is required",
		})
	}

	var (
		mcpID           string
		userID          string
		tokenCtx        *persistent.TokenContext
		apiKeyExpiresAt *time.Time
	)

	if subjectTokenType == tokenTypeAPIKey {
		// Validate the API key
		apiKey, err := req.GatewayClient.ValidateAPIKey(req.Context(), subjectToken)
		if err != nil {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "invalid API key",
			})
		}

		// Get the MCP ID from the OAuth client
		mcpID = oauthClient.Spec.MCPServerName
		if mcpID == "" {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "OAuth client not associated with an MCP server",
			})
		}

		// Check if API key has access to this MCP server
		if err := validateAPIKeyAccess(req, apiKey, mcpID); err != nil {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrAccessDenied,
				Description: err.Error(),
			})
		}

		userID = fmt.Sprintf("%d", apiKey.UserID)
		apiKeyExpiresAt = apiKey.ExpiresAt
	} else {
		// Parse the subject token JWT (existing logic)
		var err error
		tokenCtx, err = h.tokenService.DecodeToken(req.Context(), subjectToken)
		if err != nil {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "invalid subject_token",
			})
		}

		mcpID = tokenCtx.MCPID
		if mcpID == "" {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "subject_token missing mcp_id claim",
			})
		}

		userID = tokenCtx.UserID
		if userID == "" {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "subject_token missing sub claim",
			})
		}
	}

	// Ephemeral OAuth clients don't have an MCP server in the database. They are for generating tool previews.
	if !oauthClient.Spec.Ephemeral && system.IsMCPServerID(mcpID) {
		var mcpServer v1.MCPServer
		if err := req.Get(&mcpServer, mcpID); err != nil {
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "failed to retrieve MCP server " + mcpID,
			})
		}

		if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
			_, componentMCPID, ok := strings.Cut(resource, "/mcp-connect/")
			audienceID := componentMCPID

			var (
				token     string
				expiresAt time.Time
			)

			if ok {
				if system.IsMCPServerInstanceID(componentMCPID) {
					// Ensure this MCP server instance belongs to this composite MCP server.
					var component v1.MCPServerInstance
					if err := req.Get(&component, componentMCPID); err != nil || component.Spec.CompositeName != mcpServer.Name {
						return types.NewErrBadRequest("%v", Error{
							Code:        ErrInvalidRequest,
							Description: "failed to retrieve composite MCP server " + componentMCPID,
						})
					}

					audienceID = component.Spec.MCPServerName
				} else {
					// Ensure this MCP server belongs to this composite MCP server.
					var component v1.MCPServer
					if err := req.Get(&component, componentMCPID); err != nil || component.Spec.CompositeName != mcpServer.Name {
						return types.NewErrBadRequest("%v", Error{
							Code:        ErrInvalidRequest,
							Description: "failed to retrieve composite MCP server " + componentMCPID,
						})
					}
				}

				if subjectTokenType == tokenTypeAPIKey {
					// Pass the API key through to component servers for their own token exchange
					token = subjectToken
					expiresAt = time.Now().Add(tokenExpiration)
					if apiKeyExpiresAt != nil {
						expiresAt = *apiKeyExpiresAt
					}
				} else {
					// For JWTs, update the existing token context and create a new token
					tokenCtx.MCPID = componentMCPID
					tokenCtx.Audience = fmt.Sprintf("%s/mcp-connect/%s", h.baseURL, audienceID)
					expiresAt = tokenCtx.ExpiresAt

					var err error
					token, err = h.tokenService.NewToken(req.Context(), *tokenCtx)
					if err != nil {
						log.Errorf("failed to create token for component MCP server %s: %v", componentMCPID, err)
						return types.NewErrBadRequest("%v", Error{
							Code:        ErrServerError,
							Description: "failed to create token",
						})
					}
				}
			} else {
				// No component MCP ID in resource, return the original token
				token = subjectToken
				if tokenCtx != nil {
					expiresAt = tokenCtx.ExpiresAt
				} else if apiKeyExpiresAt != nil {
					expiresAt = *apiKeyExpiresAt
				} else {
					expiresAt = time.Now().Add(tokenExpiration)
				}
			}

			// For composite MCP servers, return the token.
			// This ensures it gets passed to the component MCP servers so they can do token exchange.
			return req.Write(TokenExchangeResponse{
				AccessToken:     token,
				IssuedTokenType: tokenTypeAccessToken,
				TokenType:       "Bearer",
				ExpiresIn:       max(int(time.Until(expiresAt).Seconds()), 0),
			})
		}
	} else if system.IsMCPServerInstanceID(mcpID) {
		return types.NewErrNotFound("no token exchange for %s", resource)
	}

	// Get the token store for this user and MCP
	store := h.tokenStore.ForUserAndMCP(userID, mcpID)

	// Retrieve the OAuth configuration and token
	config, token, err := store.GetTokenConfig(req.Context(), resource)
	if err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "failed to retrieve token configuration",
		})
	}

	if config == nil || token == nil {
		return types.NewErrNotFound("no token to exchange for %s", resource)
	}

	// Refresh the token if needed
	tok, err := config.TokenSource(req.Context(), token).Token()
	if err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "failed to refresh token",
		})
	}

	// Store the refreshed token if it changed
	if tok.AccessToken != token.AccessToken || tok.RefreshToken != token.RefreshToken || tok.Expiry.Unix() != token.Expiry.Unix() {
		if err = store.SetTokenConfig(req.Context(), resource, config, tok); err != nil {
			return fmt.Errorf("failed to store token: %w", err)
		}
	}

	// Return RFC 8693 compliant response
	return req.Write(TokenExchangeResponse{
		AccessToken:     tok.AccessToken,
		IssuedTokenType: tokenTypeAccessToken,
		TokenType:       "Bearer",
		ExpiresIn:       max(int(time.Until(tok.Expiry).Seconds()), 0),
	})
}

// validateAPIKeyAccess checks if the API key has access to the specified MCP server.
// For component servers (servers that belong to a composite), it instead checks whether
// the corresponding composite server is in the allowed list.
func validateAPIKeyAccess(ctx api.Context, apiKey *gwtypes.APIKey, mcpID string) error {
	if slices.Contains(apiKey.MCPServerIDs, "*") || slices.Contains(apiKey.MCPServerIDs, mcpID) {
		return nil
	}

	// Check if this is a component server - if so, check the composite server ID
	var mcpServer v1.MCPServer
	if err := ctx.Get(&mcpServer, mcpID); err == nil && mcpServer.Spec.CompositeName != "" {
		// This is a component server, check if the composite server is allowed
		if slices.Contains(apiKey.MCPServerIDs, mcpServer.Spec.CompositeName) {
			return nil
		}
	}

	return fmt.Errorf("API key does not have access to MCP server %s", mcpID)
}
