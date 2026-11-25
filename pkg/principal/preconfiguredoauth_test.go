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

// TestOwnersAreNeverPrincipals checks that the owners that stand for something other than a caller, the owner of a
// vMCP's preconfigured OAuth grants and the owner of a user's setup through an agent, are never read as a user or an
// agent, however a caller carrying one presents itself: neither can be parsed, a caller with one is no principal and
// owns nothing, and the admission check refuses a caller with either.
func TestOwnersAreNeverPrincipals(t *testing.T) {
	for _, owner := range []string{
		PreconfiguredOAuthOwner("vmcp1abc"),
		DelegatedOwner{AgentID: 5, UserID: 7}.String(),
	} {
		if p, err := Parse(owner); err == nil {
			t.Fatalf("Parse(%q) = %#v, want an error", owner, p)
		}
		if id, ok := ParseAgent(owner); ok {
			t.Fatalf("ParseAgent(%q) = %q, want none", owner, id)
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
			if got, err := CallerOwner(caller); err == nil {
				t.Errorf("CallerOwner() of a caller with UID %q marked %q = %q", owner, kind, got)
			}
		}
		if !IsPreconfiguredOAuthOwner(owner) && !IsDelegatedOwner(owner) {
			t.Errorf("the admission check would not refuse a caller with UID %q", owner)
		}
	}
	// A caller's UID with the prefix is refused by the admission check whatever follows it.
	if !IsPreconfiguredOAuthOwner("vmcp-oauth:anything/else") {
		t.Fatal("IsPreconfiguredOAuthOwner() of a malformed preconfigured OAuth owner = false")
	}
}
