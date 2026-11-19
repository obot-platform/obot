package auth

import (
	"context"
	"net/http"
	"time"
)

const (
	ObotAccessTokenCookie = "obot_access_token"

	// ObotBootstrapCookie carries the bootstrap token of a browser that logged in with it.
	ObotBootstrapCookie = "obot-bootstrap"

	// AuthProviderVerifyCookie carries the ID of an in-flight staged-provider verification across
	// the OAuth round trip, so only the browser that started it can log in through the staged
	// provider.
	AuthProviderVerifyCookie = "obot_auth_provider_verify"

	// AuthProviderVerifyWindow is how long a staged-provider verification stays open.
	AuthProviderVerifyWindow = 15 * time.Minute

	// setupLoginCookie marks a browser that just finished the OAuth round trip of a setup login, which only the
	// bootstrap user can start. GET /api/setup/oauth-complete requires it, so that another site cannot navigate a
	// signed-in user there to cache or promote them. It also lets that endpoint's redirect to /oauth2/sign_out
	// through, which the browser reports as cross-site because the navigation began at the identity provider.
	//
	// Its presence alone is what lets a request through, so it must not be one that another host can plant. Over
	// https it carries the __Host- prefix (see SetupLoginCookieName), which browsers only accept from Obot's own host.
	// Use the SetupLogin helpers below rather than this name, so that every place agrees on the name.
	setupLoginCookie = "obot_setup_login"

	// hostCookiePrefix makes a browser accept a cookie only if it is Secure, has Path=/ and no Domain, so that a
	// sibling subdomain, such as another tenant under the same parent domain, cannot set one for Obot's host.
	hostCookiePrefix = "__Host-"

	// SetupLoginWindow is how long the setup login cookie lasts. The cookie is only needed for the redirects that
	// immediately follow the OAuth round trip, so it is short.
	SetupLoginWindow = 5 * time.Minute
)

// SerializableRequest represents an HTTP request that can be serialized for authentication flows
type SerializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

// SerializableState represents the authentication state returned from auth providers
type SerializableState struct {
	ExpiresOn             *time.Time `json:"expiresOn"`
	AccessToken           string     `json:"accessToken"`
	PreferredUsername     string     `json:"preferredUsername"`
	User                  string     `json:"user"`
	Email                 string     `json:"email"`
	SetCookies            []string   `json:"setCookies"`
	RequirePasswordChange bool       `json:"requirePasswordChange,omitempty"`
}

// GroupInfo represents information about a user group from an authentication provider
type GroupInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	IconURL *string `json:"iconURL"`
}

type authProviderURLKey struct{}
type authProviderGroupIDPrefixKey struct{}

// SetupLoginCookieName returns the name of the cookie that marks a finished setup login. secure is whether Obot's
// server URL is https.
//
// Over https the name carries the __Host- prefix, so a same-site host cannot plant the cookie. Browsers reject a
// __Host- cookie that is not Secure, so over plain http the name has no prefix and the cookie has no such protection.
// Neither name is ever accepted in place of the other: accepting the unprefixed name over https would let a planted
// cookie through again.
func SetupLoginCookieName(secure bool) string {
	if secure {
		return hostCookiePrefix + setupLoginCookie
	}
	return setupLoginCookie
}

// SetSetupLoginCookie marks this browser as having just finished the setup login for the token request id.
func SetSetupLoginCookie(w http.ResponseWriter, id string, secure bool) {
	http.SetCookie(w, setupLoginCookieFor(id, int(SetupLoginWindow.Seconds()), secure))
}

// HasSetupLoginCookie reports whether r comes from a browser that just finished a setup login. A cookie that was
// cleared but that the browser still sends is empty, and does not count.
func HasSetupLoginCookie(r *http.Request, secure bool) bool {
	c, err := r.Cookie(SetupLoginCookieName(secure))
	return err == nil && c.Value != ""
}

// ClearSetupLoginCookie removes the setup login cookie once the setup login it marks has finished.
func ClearSetupLoginCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, setupLoginCookieFor("", -1, secure))
}

// setupLoginCookieFor builds the setup login cookie. Path=/ and no Domain are required by the __Host- prefix, and
// Path=/ also lets both /api/setup/oauth-complete and /oauth2/sign_out read it.
func setupLoginCookieFor(value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     SetupLoginCookieName(secure),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func ClearAuthProviderVerifyCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthProviderVerifyCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// ContextWithProviderURL adds the auth provider URL to the context
func ContextWithProviderURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, authProviderURLKey{}, url)
}

// ProviderURLFromContext retrieves the auth provider URL from the context
func ProviderURLFromContext(ctx context.Context) string {
	url, _ := ctx.Value(authProviderURLKey{}).(string)
	return url
}

// ContextWithProviderGroupIDPrefix adds the auth provider's declared group ID prefix to the context.
func ContextWithProviderGroupIDPrefix(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, authProviderGroupIDPrefixKey{}, prefix)
}

// ProviderGroupIDPrefixFromContext retrieves the auth provider's declared group ID prefix.
func ProviderGroupIDPrefixFromContext(ctx context.Context) string {
	prefix, _ := ctx.Value(authProviderGroupIDPrefixKey{}).(string)
	return prefix
}
