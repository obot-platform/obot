package principal

import (
	"testing"

	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestPreconfiguredOAuthOwnerRoundTrip(t *testing.T) {
	owner := PreconfiguredOAuthOwner("vmcp1abc")
	if owner != "vmcp-oauth:vmcp1abc" {
		t.Fatalf("PreconfiguredOAuthOwner() = %q", owner)
	}
	vmcpID, ok := ParsePreconfiguredOAuthOwner(owner)
	if !ok || vmcpID != "vmcp1abc" {
		t.Fatalf("ParsePreconfiguredOAuthOwner(%q) = %q, %v", owner, vmcpID, ok)
	}
	if !IsPreconfiguredOAuthOwner(owner) {
		t.Fatalf("IsPreconfiguredOAuthOwner(%q) = false", owner)
	}

	for _, malformed := range []string{
		"",
		"7",
		"agent:7",
		"vmcp-oauth:",
		"vmcp-oauth:vmcp1abc/component",
		"vmcp-profile:vmcp1abc/vp1def",
	} {
		if _, ok := ParsePreconfiguredOAuthOwner(malformed); ok {
			t.Errorf("ParsePreconfiguredOAuthOwner(%q) succeeded", malformed)
		}
	}
}

// TestPreconfiguredOAuthOwnerIsNeverAPrincipal checks that the owner of a vMCP's preconfigured OAuth grants is never
// read as a user or an agent, however a caller carrying it presents itself: it cannot be parsed, a caller with it is no
// principal, and it never claims to be an agent, which would confine it as one rather than refuse it.
func TestPreconfiguredOAuthOwnerIsNeverAPrincipal(t *testing.T) {
	owner := PreconfiguredOAuthOwner("vmcp1abc")
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
	if !IsPreconfiguredOAuthOwner("vmcp-oauth:anything/else") {
		t.Fatal("IsPreconfiguredOAuthOwner() of a malformed preconfigured OAuth owner = false")
	}
}
