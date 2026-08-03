package authz

import (
	"net/http"
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func agentConnectRequest(path string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	return req
}

// An unauthenticated caller must reach the handler so a browser is redirected
// to sign in. The handler redirects before reading anything, so this exposes
// nothing -- not even whether the instance exists.
func TestAllowAgentConnectSignInForUnauthenticated(t *testing.T) {
	a := &Authorizer{}
	anonymous := newUser(&user.DefaultInfo{Name: "anonymous", Groups: []string{UnauthenticatedGroup}})

	if !a.allowAgentConnectSignIn(agentConnectRequest("/agent-connect/hai1abc"), anonymous) {
		t.Error("an unauthenticated caller must reach the handler to be sent to sign in")
	}
	if !a.allowAgentConnectSignIn(agentConnectRequest("/agent-connect/hai1abc/some/path"), anonymous) {
		t.Error("sub-paths must behave the same")
	}
}

// The allowance must not become a way to reach anything else.
func TestAllowAgentConnectSignInIsScopedToTheRoute(t *testing.T) {
	a := &Authorizer{}
	anonymous := newUser(&user.DefaultInfo{Name: "anonymous", Groups: []string{UnauthenticatedGroup}})

	for _, path := range []string{
		"/api/hosted-agent-instances",
		"/api/hosted-agent-instances/hai1abc",
		"/mcp-connect/ms1abc",
		"/api/llm-proxy/openai/v1/chat/completions",
		"/agent-connectX/hai1abc",
		"/",
	} {
		if a.allowAgentConnectSignIn(agentConnectRequest(path), anonymous) {
			t.Errorf("%s must not be reachable through the sign-in allowance", path)
		}
	}
}

// The branch is for unauthenticated callers only. An authenticated one must go
// through apiResources, where checkHostedAgentInstance narrows to the owner --
// otherwise any signed-in user could proxy to anyone's sandbox.
func TestAllowAgentConnectSignInRejectsAuthenticatedCallers(t *testing.T) {
	a := &Authorizer{}

	for _, u := range []user.Info{
		&user.DefaultInfo{Name: "someone@example.com", UID: "7", Groups: []string{types.GroupAuthenticated}},
		&user.DefaultInfo{Name: "admin@example.com", UID: "1", Groups: []string{types.GroupAuthenticated, "admin"}},
		&user.DefaultInfo{Name: "hosted-agent:hai1zzz", UID: "hosted-agent:hai1zzz", Groups: []string{types.GroupAuthenticated, "hosted-agent"}},
	} {
		if a.allowAgentConnectSignIn(agentConnectRequest("/agent-connect/hai1abc"), newUser(u)) {
			t.Errorf("%s must be authorized against the instance, not waved through", u.GetName())
		}
	}
}
