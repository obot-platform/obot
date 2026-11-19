package principal

import (
	"testing"

	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestAgentOAuthGrantOwnerRoundTrip(t *testing.T) {
	owner := AgentOAuthGrantOwner("vmcp1abc")
	if owner != "vmcp-agent-oauth:vmcp1abc" {
		t.Fatalf("AgentOAuthGrantOwner() = %q", owner)
	}
	vmcpID, ok := ParseAgentOAuthGrantOwner(owner)
	if !ok || vmcpID != "vmcp1abc" {
		t.Fatalf("ParseAgentOAuthGrantOwner(%q) = %q, %v", owner, vmcpID, ok)
	}
	if !IsAgentOAuthGrantOwner(owner) {
		t.Fatalf("IsAgentOAuthGrantOwner(%q) = false", owner)
	}

	for _, malformed := range []string{
		"",
		"7",
		"agent:7",
		"vmcp-agent-oauth:",
		"vmcp-agent-oauth:vmcp1abc/component",
		"vmcp-profile:vmcp1abc/vp1def",
	} {
		if _, ok := ParseAgentOAuthGrantOwner(malformed); ok {
			t.Errorf("ParseAgentOAuthGrantOwner(%q) succeeded", malformed)
		}
	}
}

// TestAgentOAuthGrantOwnerIsNeverAPrincipal checks that the owner of a vMCP's agent OAuth grants is never read as a user
// or an agent, however a caller carrying it presents itself: it cannot be parsed, a caller with it is no principal, and
// it never claims to be an agent, which would confine it as one rather than refuse it.
func TestAgentOAuthGrantOwnerIsNeverAPrincipal(t *testing.T) {
	owner := AgentOAuthGrantOwner("vmcp1abc")
	if p, err := Parse(owner); err == nil {
		t.Fatalf("Parse(%q) = %#v, want an error", owner, p)
	}
	for _, kind := range []Kind{KindUser, KindAgent, ""} {
		extra := map[string][]string{}
		if kind != "" {
			RecordKind(extra, kind)
		}
		caller := &kuser.DefaultInfo{UID: owner, Name: owner, Extra: extra}
		if p, ok := FromUserInfo(caller); ok {
			t.Errorf("FromUserInfo() of a caller with UID %q marked %q = %#v", owner, kind, p)
		}
	}
	// A caller's UID with the prefix is refused by the admission check whatever follows it.
	if !IsAgentOAuthGrantOwner("vmcp-agent-oauth:anything/else") {
		t.Fatal("IsAgentOAuthGrantOwner() of a malformed agent OAuth grant owner = false")
	}
}
