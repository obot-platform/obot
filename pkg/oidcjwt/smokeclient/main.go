package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	var (
		listen    = flag.String("listen", "127.0.0.1:18080", "address for the local OIDC issuer to listen on")
		issuerURL = flag.String("issuer-url", "", "issuer URL advertised in discovery and JWT iss; defaults from listen address")
		audience  = flag.String("audience", "obot-default", "JWT audience and OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE")
		subject   = flag.String("subject", "studio-smoke-user", "JWT subject")
		email     = flag.String("email", "studio-smoke-user@example.com", "JWT email claim")
		roles     = flag.String("roles", "admin", "comma-separated JWT roles claim")
		eligible  = flag.Bool("eligible", true, "JWT eligible claim")
		ttl       = flag.Duration("ttl", 5*time.Minute, "JWT lifetime")
		obotURL   = flag.String("obot-url", "", "optional Obot API URL to call with the minted JWT")
		hold      = flag.Bool("hold", false, "keep the issuer server running after printing/calling")
	)
	flag.Parse()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	exitOnErr("generate key", err)

	ln, err := net.Listen("tcp", *listen)
	exitOnErr("listen", err)
	defer ln.Close()

	if *issuerURL == "" {
		*issuerURL = defaultIssuerURL(ln.Addr().String())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", discoveryHandler(*issuerURL))
	mux.HandleFunc("/.well-known/jwks.json", jwksHandler(priv, "smoke-key"))

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "issuer server failed: %v\n", err)
			os.Exit(1)
		}
	}()
	defer server.Shutdown(context.Background())

	token, err := mintJWT(priv, "smoke-key", *issuerURL, *audience, *subject, *email, splitCSV(*roles), *eligible, *ttl)
	exitOnErr("mint jwt", err)

	fmt.Printf("issuer listening: %s\n\n", *issuerURL)
	fmt.Println("Configure Obot with:")
	fmt.Printf("  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER=%s\n", *issuerURL)
	fmt.Printf("  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE=%s\n", *audience)
	fmt.Println("  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME=eligible")
	fmt.Println("  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME=roles")
	fmt.Println("  OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES=admin")
	fmt.Printf("\nJWT:\n%s\n", token)

	if *obotURL != "" {
		status, body, err := callObot(*obotURL, token)
		exitOnErr("call obot", err)
		fmt.Printf("\nObot response: %s\n%s\n", status, body)
	}

	if *hold || *obotURL == "" {
		fmt.Println("\nPress Ctrl+C to stop the issuer.")
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
	}
}

func discoveryHandler(issuerURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuerURL,
			"jwks_uri":                              strings.TrimRight(issuerURL, "/") + "/.well-known/jwks.json",
			"authorization_endpoint":                strings.TrimRight(issuerURL, "/") + "/auth",
			"token_endpoint":                        strings.TrimRight(issuerURL, "/") + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	}
}

func jwksHandler(priv *rsa.PrivateKey, kid string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		pub := priv.PublicKey
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": kid,
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	}
}

func mintJWT(priv *rsa.PrivateKey, kid, iss, aud, sub, email string, roles []string, eligible bool, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":                strings.TrimRight(iss, "/"),
		"aud":                aud,
		"sub":                sub,
		"iat":                now.Unix(),
		"exp":                now.Add(ttl).Unix(),
		"email":              email,
		"eligible":           eligible,
		"roles":              roles,
		"name":               sub,
		"preferred_username": sub,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	return tok.SignedString(priv)
}

func callObot(url, token string) (string, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	return resp.Status, string(body), nil
}

func defaultIssuerURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func exitOnErr(action string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
		os.Exit(1)
	}
}
