package principal

import (
	"testing"

	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestProfileGrantRoundTrip(t *testing.T) {
	owner := ProfileGrant("vmcp1abc", "vp1def")
	if owner != "vmcp-profile:vmcp1abc/vp1def" {
		t.Fatalf("ProfileGrant() = %q", owner)
	}
	vmcpID, profileID, ok := ParseProfileGrant(owner)
	if !ok || vmcpID != "vmcp1abc" || profileID != "vp1def" {
		t.Fatalf("ParseProfileGrant(%q) = %q, %q, %v", owner, vmcpID, profileID, ok)
	}
	if !IsProfileGrant(owner) {
		t.Fatalf("IsProfileGrant(%q) = false", owner)
	}

	for _, malformed := range []string{
		"",
		"7",
		"agent:7",
		"vmcp-profile:",
		"vmcp-profile:vmcp1abc",
		"vmcp-profile:/vp1def",
		"vmcp-profile:vmcp1abc/",
		"vmcp-profile:vmcp1abc/vp1def/extra",
	} {
		if _, _, ok := ParseProfileGrant(malformed); ok {
			t.Errorf("ParseProfileGrant(%q) succeeded", malformed)
		}
	}
}

// TestProfileGrantIsNeverAPrincipal checks that the owner of a profile's grants is never read as a user or an agent,
// however a caller carrying it presents itself: it cannot be parsed, a caller with it is no principal, and it never
// claims to be an agent, which would confine it as one rather than refuse it.
func TestProfileGrantIsNeverAPrincipal(t *testing.T) {
	owner := ProfileGrant("vmcp1abc", "vp1def")
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
	if !IsProfileGrant("vmcp-profile:anything") {
		t.Fatal("IsProfileGrant() of a malformed profile grant owner = false")
	}
}
