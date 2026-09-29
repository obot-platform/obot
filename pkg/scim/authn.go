package scim

import (
	"errors"
	"net/http"
	"slices"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	// connectionPrincipalPrefix begins the name of a SCIM connection's principal. Its UID is the connection ID.
	connectionPrincipalPrefix = "scim-connection:"
)

// UnavailableError reports a request to the SCIM endpoint while no SCIM connection exists. The API server answers it
// with 503.
type UnavailableError struct{}

// Authenticator authenticates requests to the SCIM endpoint with the bearer token of the connection named in the
// path. It runs ahead of every other authenticator and ends the chain on SCIM routes, so no cookie, Obot credential,
// redirect, or just-in-time user creation ever applies to them. Off SCIM routes, it declines.
type Authenticator struct {
	gateway *gclient.Client
}

func (*UnavailableError) Error() string {
	return "SCIM is not enabled"
}

// NewAuthenticator returns the SCIM authenticator.
func NewAuthenticator(gateway *gclient.Client) *Authenticator {
	return &Authenticator{
		gateway: gateway,
	}
}

// AuthenticateRequest yields the principal of the connection named in the path when the request carries its current
// or still-accepted previous token. A missing or invalid token, a connection without a token, and an unknown
// connection all yield the anonymous principal, which authorization answers with 401. Without any connection, it
// returns an *UnavailableError before it looks at the token.
func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if !IsSCIMPath(req.URL.Path) {
		return nil, false, nil
	}

	exists, err := a.gateway.HasSCIMConnections(req.Context())
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, new(UnavailableError)
	}

	connectionID, _ := splitPath(req.URL.Path)
	token, ok := bearerToken(req)
	if connectionID == "" || !ok {
		return anonymous(), true, nil
	}

	conn, err := a.gateway.AuthenticateSCIMConnection(req.Context(), connectionID, token)
	if errors.Is(err, gclient.ErrSCIMConnectionNotFound) {
		return anonymous(), true, nil
	} else if _, ok := errors.AsType[*gclient.SCIMAuthenticationError](err); ok {
		return anonymous(), true, nil
	} else if err != nil {
		return nil, false, err
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   connectionPrincipalPrefix + conn.ID,
			UID:    conn.ID,
			Groups: []string{types2.GroupSCIM},
		},
	}, true, nil
}

// IsConnectionPrincipal reports whether a principal is a SCIM connection's, whose UID is the connection ID.
func IsConnectionPrincipal(u user.Info) bool {
	return u != nil && slices.Contains(u.GetGroups(), types2.GroupSCIM)
}

func anonymous() *authenticator.Response {
	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:    "anonymous",
			Name:   "anonymous",
			Groups: []string{authz.UnauthenticatedGroup},
		},
	}
}

// WriteError writes a SCIM error response with the given status. The API server uses it for the failures it answers
// before a SCIM request reaches the handler, such as authentication, rate limiting, and authorization.
func WriteError(w http.ResponseWriter, status int, detail string) {
	writeError(w, &Error{
		Status: status,
		Detail: detail,
	})
}

// WriteUnauthorized writes the response to a SCIM request that no connection's token authenticated.
func WriteUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="Obot SCIM"`)
	WriteError(w, http.StatusUnauthorized, "a valid bearer token for this SCIM connection is required")
}

// WriteUnavailable writes the response to a SCIM request while the endpoint is unavailable. The identity provider
// records the request as a failed task, which an administrator retries once the endpoint is available again.
func WriteUnavailable(w http.ResponseWriter, detail string) {
	unavailable(w, detail)
}
