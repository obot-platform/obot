// Package localauth implements a built-in username/password auth provider.
//
// Unlike the other auth providers, which are external binaries launched as daemons by the
// dispatcher, this provider runs in-process so that it can share Obot's database. It speaks the
// same HTTP protocol as the external providers (/oauth2/start, /oauth2/callback, /oauth2/sign_out,
// /obot-get-state, /obot-get-user-info), so the rest of the auth stack treats it like any other.
package localauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
)

const (
	// ProviderName is the name of the AuthProvider resource for this provider.
	ProviderName = system.LocalAuthProvider

	// EmailDomainsEnvVar is the configuration parameter that restricts which email domains
	// can be used for local users. It matches the name used by the external auth providers.
	EmailDomainsEnvVar = "OBOT_AUTH_PROVIDER_EMAIL_DOMAINS"

	// LoginPath is the UI route that renders the login form.
	LoginPath = "/login/local"

	sessionDuration = 7 * 24 * time.Hour

	// The error body the proxy translates into an invalid session, which makes the browser drop
	// its cookie and log in again. See pkg/proxy/proxy.go.
	invalidSessionBody = "record not found"
)

var log = logger.Package()

type Provider struct {
	gatewayClient *client.Client
	serverURL     string
	throttle      *throttle
	tokens        *tokenSigner
}

func New(gatewayClient *client.Client, serverURL string) (*Provider, error) {
	tokens, err := newTokenSigner()
	if err != nil {
		return nil, err
	}

	return &Provider{
		gatewayClient: gatewayClient,
		serverURL:     serverURL,
		throttle:      newThrottle(),
		tokens:        tokens,
	}, nil
}

// Start serves the provider on a random loopback port and returns its URL.
// The server is shut down when the context is cancelled.
func (p *Provider) Start(ctx context.Context) (url.URL, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to listen for local auth provider: %w", err)
	}

	server := &http.Server{
		Handler:           p.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("local auth provider server exited: %v", err)
		}
	}()

	go p.cleanupSessions(ctx)
	go p.throttle.run(ctx)

	u := url.URL{Scheme: "http", Host: listener.Addr().String()}
	log.Infof("Started local auth provider: url=%s", u.String())

	return u, nil
}

func (p *Provider) handler() http.Handler {
	mux := http.NewServeMux()
	// Health check, matching what the dispatcher expects of daemon-based providers.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /oauth2/start", p.start)
	// The login form posts back to /oauth2/start rather than /oauth2/callback: the proxy only
	// forwards a callback when it holds the short-lived cookie it sets at the start of an OAuth
	// flow, and it drops that cookie once used, which would break retries after a failed login.
	mux.HandleFunc("POST /oauth2/start", p.login)
	mux.HandleFunc("GET /oauth2/sign_out", p.signOut)
	mux.HandleFunc("POST /obot-get-state", p.getState)
	mux.HandleFunc("GET /obot-get-user-info", p.getUserInfo)
	return mux
}

func (p *Provider) cleanupSessions(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := p.gatewayClient.DeleteExpiredLocalAuthSessions(ctx); err != nil {
			log.Warnf("failed to clean up expired local auth sessions: %v", err)
		}

		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

// start redirects to the login form in the UI, preserving the post-login redirect target.
func (p *Provider) start(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, LoginPath+"?rd="+url.QueryEscape(redirectTarget(r.URL.Query().Get("rd"))), http.StatusFound)
}

func (p *Provider) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	rd := redirectTarget(r.FormValue("rd"))

	// Reject cross-origin logins. Browsers always send Origin on POST, so this blocks login CSRF
	// without requiring the login form to carry a token.
	if !sameOrigin(r) {
		http.Error(w, "cross-origin login is not allowed", http.StatusForbidden)
		return
	}

	email := client.NormalizeEmail(r.FormValue("email"))
	password := r.FormValue("password")
	if email == "" || password == "" {
		p.loginFailed(w, r, rd, "Enter your email and password.")
		return
	}

	if p.throttle.blocked(email) {
		p.loginFailed(w, r, rd, "Too many failed login attempts. Try again later.")
		return
	}

	user, err := p.gatewayClient.LocalAuthUserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Errorf("failed to look up local auth user: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Verify a dummy hash when the user doesn't exist so that a missing user and a wrong password
	// take about the same amount of time.
	passwordHash := dummyHash
	if user != nil {
		passwordHash = user.PasswordHash
	}

	if err := VerifyPassword(passwordHash, password); err != nil || user == nil {
		if err != nil && !errors.Is(err, ErrInvalidPassword) {
			log.Warnf("failed to verify password for local auth user: %v", err)
		}
		p.throttle.failed(email)
		p.loginFailed(w, r, rd, "Incorrect email or password.")
		return
	}

	allowed, err := p.emailDomainAllowed(r.Context(), email)
	if err != nil {
		log.Errorf("failed to check allowed email domains: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	} else if !allowed {
		p.loginFailed(w, r, rd, "This email domain is not allowed to sign in.")
		return
	}

	p.throttle.succeeded(email)

	token, err := generateToken()
	if err != nil {
		log.Errorf("failed to generate session token: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(sessionDuration)
	if err := p.gatewayClient.CreateLocalAuthSession(r.Context(), hash.String(token), user.ID, expiresAt); err != nil {
		log.Errorf("failed to create local auth session: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.ObotAccessTokenCookie,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   p.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, rd, http.StatusFound)
}

func (p *Provider) loginFailed(w http.ResponseWriter, r *http.Request, rd, message string) {
	http.Redirect(w, r, fmt.Sprintf("%s?rd=%s&error=%s", LoginPath, url.QueryEscape(rd), url.QueryEscape(message)), http.StatusFound)
}

func (p *Provider) signOut(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.ObotAccessTokenCookie); err == nil {
		if err := p.gatewayClient.DeleteLocalAuthSession(r.Context(), hash.String(cookie.Value)); err != nil {
			log.Warnf("failed to delete local auth session: %v", err)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.ObotAccessTokenCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   p.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, redirectTarget(r.URL.Query().Get("rd")), http.StatusFound)
}

// getState resolves the session cookie on a serialized request into the user it belongs to.
// This is what authenticates every API request made with a local auth session cookie.
func (p *Provider) getState(w http.ResponseWriter, r *http.Request) {
	var sr auth.SerializableRequest
	if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	token := cookieValue(sr.Header, auth.ObotAccessTokenCookie)
	if token == "" {
		http.Error(w, invalidSessionBody, http.StatusInternalServerError)
		return
	}

	session, user, err := p.gatewayClient.LocalAuthSession(r.Context(), hash.String(token))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, invalidSessionBody, http.StatusInternalServerError)
		return
	} else if err != nil {
		log.Errorf("failed to look up local auth session: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, auth.SerializableState{
		ExpiresOn: &session.ExpiresAt,
		// The access token is what the gateway passes back to /obot-get-user-info. It carries the
		// email rather than the session token so that user info needs no database lookup: the
		// gateway asks for it from inside an open transaction, and on SQLite — which allows a
		// single connection — a query here would deadlock against that transaction.
		AccessToken:       p.tokens.sign(user.Email),
		User:              user.Email,
		PreferredUsername: user.Email,
		Email:             user.Email,
	})
}

// getUserInfo returns the profile for the access token in the Authorization header.
// The gateway uses it to fill in the user's display name.
func (p *Provider) getUserInfo(w http.ResponseWriter, r *http.Request) {
	email, err := p.tokens.verify(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	writeJSON(w, map[string]any{
		"id":    email,
		"email": email,
		"name":  email,
	})
}

// EmailDomainAllowed reports whether the given email is allowed by the provider's configured
// email domain restriction. An unconfigured provider allows nothing.
func (p *Provider) EmailDomainAllowed(ctx context.Context, email string) (bool, error) {
	return p.emailDomainAllowed(ctx, client.NormalizeEmail(email))
}

func (p *Provider) emailDomainAllowed(ctx context.Context, email string) (bool, error) {
	cred, err := p.gatewayClient.RevealCredential(ctx, []string{ProviderName, system.GenericAuthProviderCredentialContext}, ProviderName)
	if err != nil {
		if errors.As(err, &client.CredentialNotFoundError{}) {
			return false, nil
		}
		return false, err
	}

	return emailDomainAllowed(cred.Secrets[EmailDomainsEnvVar], email), nil
}

func emailDomainAllowed(domains, email string) bool {
	_, domain, ok := strings.Cut(email, "@")
	if !ok || domain == "" {
		return false
	}

	for allowed := range strings.SplitSeq(domains, ",") {
		allowed = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(allowed, "@")))
		if allowed == "*" || allowed == domain {
			return true
		}
	}

	return false
}

func (p *Provider) secureCookies() bool {
	return strings.HasPrefix(p.serverURL, "https://")
}

// redirectTarget sanitizes a post-login redirect so that it can only point back into Obot.
func redirectTarget(rd string) string {
	if !strings.HasPrefix(rd, "/") || strings.HasPrefix(rd, "//") {
		return "/"
	}
	return rd
}

// sameOrigin reports whether the request was initiated by the page it claims to come from.
// Requests without an Origin or Referer (curl, tests) are allowed: CSRF requires a browser.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	return u.Host == r.Host
}

func cookieValue(header map[string][]string, name string) string {
	// http.Request.Cookie is the only reliable cookie parser, so build a request to use it.
	r := http.Request{Header: http.Header(header)}
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Warnf("failed to write local auth provider response: %v", err)
	}
}
