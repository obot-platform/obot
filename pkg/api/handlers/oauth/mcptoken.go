package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (h *handler) mcpToken(req api.Context) error {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    h.oauthConfig.Issuer,
		Subject:   strings.ToLower(rand.Text()),
		Audience:  jwt.ClaimStrings{"obot"},
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		// TODO: What's the best way to handle refreshing tokens?
		// ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "obot-key"
	token.Header["jku"] = fmt.Sprintf("%s/.well-known/jwks.json", h.baseURL)

	tkn, err := token.SignedString(h.key)
	if err != nil {
		return fmt.Errorf("failed to sign mcp token: %w", err)
	}

	return req.Write(tkn)
}

func (h *handler) mcpConfig(req api.Context) error {
	token := strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer ")
	identity := req.Request.Header.Get("X-Obot-Identity")
	if identity == "" {
		return types.NewErrHTTP(http.StatusUnauthorized, "missing identity header")
	}

	identityToken, err := jwt.Parse(identity, func(*jwt.Token) (interface{}, error) {
		return h.key.Public(), nil
	})
	if err != nil || !identityToken.Valid {
		return types.NewErrHTTP(http.StatusUnauthorized, fmt.Sprintf("failed to parse identity token: %v", err))
	}

	var pointerToken v1.MCPPointerToken
	if err = req.Get(&pointerToken, fmt.Sprintf("%x", sha256.Sum256([]byte(token)))); err != nil {
		return types.NewErrHTTP(http.StatusUnauthorized, fmt.Sprintf("failed to get mcp pointer token: %v", err))
	}

	identitySubject, err := identityToken.Claims.GetSubject()
	if err != nil {
		return types.NewErrHTTP(http.StatusUnauthorized, fmt.Sprintf("failed to get subject from identity token: %v", err))
	}

	if identitySubject != pointerToken.Spec.Resource {
		return types.NewErrHTTP(http.StatusUnauthorized, "identity token does not match pointer token")
	}

	var serverConfig v1.MCPServerConfig
	if err = req.Get(&serverConfig, pointerToken.Spec.Resource); err != nil {
		return fmt.Errorf("failed to get mcp server config: %w", err)
	}

	return req.Write(serverConfig.Spec)
}
