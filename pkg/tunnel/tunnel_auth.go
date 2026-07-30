package tunnel

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Authenticator authenticates the credential stored on the MCPTunnel
// that owns the supplied bearer token. It yields a non-user principal
// restricted by the API authorizer to opening a tunnel connection.
type Authenticator struct {
	tunnels kclient.Reader
}

// NewTunnelAuthenticator creates a tunnel credential authenticator.
func NewTunnelAuthenticator(tunnels kclient.Reader) *Authenticator {
	return &Authenticator{tunnels: tunnels}
}

// AuthenticateRequest implements authenticator.Request.
func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if !isTunnelConnectRequest(req) || a.tunnels == nil {
		return nil, false, nil
	}

	token, ok := tunnelBearerToken(req)
	if !ok {
		return nil, false, nil
	}
	matcher, ok := newCredentialMatcher(token)
	if !ok {
		return nil, false, nil
	}

	var tunnels v1.MCPTunnelList
	if err := a.tunnels.List(req.Context(), &tunnels,
		kclient.InNamespace(system.DefaultNamespace),
		kclient.MatchingFields{v1.MCPTunnelCredentialIDField: matcher.credentialID},
	); err != nil {
		return nil, false, fmt.Errorf("failed to list MCP tunnels: %w", err)
	}

	var matchedName string
	for _, tunnel := range tunnels.Items {
		if !matcher.Matches(tunnel.Spec.Credential) {
			continue
		}
		if matchedName != "" {
			return nil, false, errors.New("tunnel credential matches multiple MCP tunnels")
		}
		matchedName = tunnel.Name
	}
	if matchedName == "" {
		return nil, false, nil
	}
	if err := types.ValidateTunnelName(matchedName); err != nil {
		return nil, false, fmt.Errorf("matched MCP tunnel has an invalid name: %w", err)
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   matchedName,
			UID:    matchedName,
			Groups: []string{types.GroupTunnel},
		},
	}, true, nil
}

func tunnelBearerToken(req *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	return token, ok && token != ""
}

func isTunnelConnectRequest(req *http.Request) bool {
	return req.Method == http.MethodGet &&
		req.URL.Path == "/tunnel/connect"
}
