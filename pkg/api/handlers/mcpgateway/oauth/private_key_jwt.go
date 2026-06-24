package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	clientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	maxClientJWKSetBytes         = 100 * 1024
)

var privateKeyJWTSigningAlgorithms = []string{
	"RS256", "RS384", "RS512",
	"PS256", "PS384", "PS512",
	"ES256", "ES384", "ES512",
	"EdDSA",
}

func clientIDFromClientAssertion(form url.Values) (string, error) {
	if form.Get("client_assertion_type") != clientAssertionTypeJWTBearer {
		return "", fmt.Errorf("client_assertion_type must be %s", clientAssertionTypeJWTBearer)
	}

	assertion := form.Get("client_assertion")
	if assertion == "" {
		return "", fmt.Errorf("client_assertion is required")
	}

	var claims jwt.RegisteredClaims
	if _, _, err := new(jwt.Parser).ParseUnverified(assertion, &claims); err != nil {
		return "", fmt.Errorf("client_assertion is invalid: %w", err)
	}
	if claims.Issuer == "" || claims.Subject == "" || claims.Issuer != claims.Subject {
		return "", fmt.Errorf("client_assertion issuer and subject must both match the client_id")
	}

	return claims.Subject, nil
}

func (h *handler) validatePrivateKeyJWT(ctx context.Context, form url.Values, client v1.OAuthClient) error {
	if form.Get("client_assertion_type") != clientAssertionTypeJWTBearer {
		return fmt.Errorf("client_assertion_type must be %s", clientAssertionTypeJWTBearer)
	}

	assertion := form.Get("client_assertion")
	if assertion == "" {
		return fmt.Errorf("client_assertion is required")
	}

	jwks, err := h.clientJWKSet(ctx, client)
	if err != nil {
		return err
	}

	tokenEndpoint := h.oauthConfig.TokenEndpoint
	if tokenEndpoint == "" {
		tokenEndpoint = strings.TrimRight(h.baseURL, "/") + "/oauth/token"
	}

	validMethods := h.oauthConfig.TokenEndpointAuthSigningAlgValuesSupported
	if len(validMethods) == 0 {
		validMethods = privateKeyJWTSigningAlgorithms
	}

	claims := jwt.RegisteredClaims{}
	parser := jwt.NewParser(
		jwt.WithAudience(tokenEndpoint),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(client.Name),
		jwt.WithSubject(client.Name),
		jwt.WithValidMethods(validMethods),
	)

	tkn, err := parser.ParseWithClaims(assertion, &claims, func(tkn *jwt.Token) (any, error) {
		return verificationKeysForJWT(tkn, jwks, validMethods)
	})
	if err != nil {
		return fmt.Errorf("client_assertion is invalid: %w", err)
	}
	if !tkn.Valid {
		return fmt.Errorf("client_assertion is invalid")
	}

	return nil
}

func (h *handler) clientJWKSet(ctx context.Context, client v1.OAuthClient) (jose.JSONWebKeySet, error) {
	if client.Spec.Manifest.JWKS != "" {
		return parseClientJWKSet([]byte(client.Spec.Manifest.JWKS))
	}
	if client.Spec.Manifest.JWKSURI == "" {
		return jose.JSONWebKeySet{}, fmt.Errorf("client jwks_uri or jwks is required")
	}
	if err := validateClientJWKSetURL(client.Spec.Manifest.JWKSURI); err != nil {
		return jose.JSONWebKeySet{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.Spec.Manifest.JWKSURI, nil)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.clientMetadataHTTPClient.Do(req)
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("failed to fetch client jwks_uri: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return jose.JSONWebKeySet{}, fmt.Errorf("client jwks_uri returned status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
			return jose.JSONWebKeySet{}, fmt.Errorf("client jwks_uri must return JSON")
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxClientJWKSetBytes+1))
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("failed to read client jwks_uri: %w", err)
	}
	if len(body) > maxClientJWKSetBytes {
		return jose.JSONWebKeySet{}, fmt.Errorf("client jwks_uri exceeds %d bytes", maxClientJWKSetBytes)
	}

	return parseClientJWKSet(body)
}

func parseClientJWKSet(data []byte) (jose.JSONWebKeySet, error) {
	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal(data, &jwks); err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("client jwks is invalid JSON: %w", err)
	}
	if len(jwks.Keys) == 0 {
		return jose.JSONWebKeySet{}, fmt.Errorf("client jwks must contain at least one key")
	}
	return jwks, nil
}

func validateClientJWKSetURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("client jwks_uri is invalid: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("client jwks_uri must use https")
	}
	if u.Host == "" {
		return fmt.Errorf("client jwks_uri must include a host")
	}
	if u.User != nil {
		return fmt.Errorf("client jwks_uri must not include userinfo")
	}
	if u.Fragment != "" {
		return fmt.Errorf("client jwks_uri must not include a fragment")
	}
	return nil
}

func verificationKeysForJWT(tkn *jwt.Token, jwks jose.JSONWebKeySet, validMethods []string) (any, error) {
	alg := tkn.Method.Alg()
	if alg == "" || strings.EqualFold(alg, "none") || strings.HasPrefix(alg, "HS") {
		return nil, fmt.Errorf("client_assertion signing algorithm is not allowed")
	}
	if len(validMethods) > 0 && !slices.Contains(validMethods, alg) {
		return nil, fmt.Errorf("client_assertion signing algorithm %s is not supported", alg)
	}

	var keys []jose.JSONWebKey
	if kid, _ := tkn.Header["kid"].(string); kid != "" {
		keys = jwks.Key(kid)
	} else {
		keys = jwks.Keys
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("client_assertion signing key was not found")
	}

	keySet := jwt.VerificationKeySet{}
	for _, key := range keys {
		if key.Use != "" && key.Use != "sig" {
			continue
		}
		if key.Algorithm != "" && key.Algorithm != alg {
			continue
		}
		if !key.Valid() {
			continue
		}
		switch key.Key.(type) {
		case []byte:
			continue
		default:
			keySet.Keys = append(keySet.Keys, key.Key)
		}
	}
	if len(keySet.Keys) == 0 {
		return nil, fmt.Errorf("client_assertion signing key was not found")
	}

	return keySet, nil
}
