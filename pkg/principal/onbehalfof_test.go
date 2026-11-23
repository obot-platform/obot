package principal

import (
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// caller returns a caller with uid, the recorded kind (none when empty), and extra's other values.
func caller(uid string, kind Kind, extra map[string][]string) kuser.Info {
	if extra == nil {
		extra = map[string][]string{}
	}
	if kind != "" {
		RecordPrincipal(extra, kind, types.UserStatusActive)
	}
	return &kuser.DefaultInfo{UID: uid, Name: uid, Extra: extra}
}

func TestOnBehalfOfRoundTripsOnAnAgent(t *testing.T) {
	info := caller("agent:5", KindAgent, nil)
	RecordOnBehalfOf(info.GetExtra(), 7)

	userID, ok := OnBehalfOf(info)
	if !ok || userID != "7" {
		t.Fatalf("OnBehalfOf() = %q, %v; want 7", userID, ok)
	}
	// The caller is still the agent.
	if p, ok := FromUserInfo(info); !ok || p != Agent(5) {
		t.Fatalf("FromUserInfo() = %#v, %v; want agent 5", p, ok)
	}
	owner, ok := DelegatedOwnerOf(info)
	if !ok || owner != (DelegatedOwner{AgentID: 5, UserID: 7}) {
		t.Fatalf("DelegatedOwnerOf() = %#v, %v; want agent 5 for user 7", owner, ok)
	}
	if got, want := owner.String(), "agent:5/user:7"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if HasInvalidOnBehalfOf(info) {
		t.Fatal("HasInvalidOnBehalfOf() of an agent acting for a user = true")
	}

	// Recording again replaces the user.
	RecordOnBehalfOf(info.GetExtra(), 8)
	if userID, ok := OnBehalfOf(info); !ok || userID != "8" {
		t.Fatalf("OnBehalfOf() after recording again = %q, %v; want 8", userID, ok)
	}
}

func TestOnBehalfOfOfACallerThatActsForNoOne(t *testing.T) {
	for _, info := range []kuser.Info{
		nil,
		caller("agent:5", KindAgent, nil),
		caller("7", KindUser, nil),
		caller("anonymous", "", nil),
	} {
		if userID, ok := OnBehalfOf(info); ok {
			t.Errorf("OnBehalfOf(%v) = %q", info, userID)
		}
		if _, ok := DelegatedOwnerOf(info); ok {
			t.Errorf("DelegatedOwnerOf(%v) succeeded", info)
		}
		if HasInvalidOnBehalfOf(info) {
			t.Errorf("HasInvalidOnBehalfOf(%v) = true", info)
		}
	}
}

// TestOnBehalfOfIsRefusedOnAnythingButAnAgent checks that only a well-formed agent acts for a user: any other caller
// that carries an on-behalf-of user, or an agent with a malformed one, is no principal at all, rather than read as
// itself or as the user it names.
func TestOnBehalfOfIsRefusedOnAnythingButAnAgent(t *testing.T) {
	for _, tt := range []struct {
		name       string
		uid        string
		kind       Kind
		onBehalfOf []string
	}{
		{
			name:       "user",
			uid:        "8",
			kind:       KindUser,
			onBehalfOf: []string{"7"},
		},
		{
			name:       "user acting for itself",
			uid:        "7",
			kind:       KindUser,
			onBehalfOf: []string{"7"},
		},
		{
			name:       "caller with no kind",
			uid:        "8",
			onBehalfOf: []string{"7"},
		},
		{
			name:       "agent acting for an agent",
			uid:        "agent:5",
			kind:       KindAgent,
			onBehalfOf: []string{"agent:6"},
		},
		{
			name:       "agent acting for two users",
			uid:        "agent:5",
			kind:       KindAgent,
			onBehalfOf: []string{"7", "8"},
		},
		{
			name: "agent acting for no user ID",
			uid:  "agent:5",
			kind: KindAgent,
		},
		{
			name:       "agent acting for an empty user ID",
			uid:        "agent:5",
			kind:       KindAgent,
			onBehalfOf: []string{""},
		},
		{
			name:       "agent acting for a malformed user ID",
			uid:        "agent:5",
			kind:       KindAgent,
			onBehalfOf: []string{"7x"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := caller(tt.uid, tt.kind, nil)
			info.GetExtra()[OnBehalfOfExtra] = tt.onBehalfOf

			if p, ok := FromUserInfo(info); ok {
				t.Errorf("FromUserInfo() = %#v", p)
			}
			if userID, ok := OnBehalfOf(info); ok {
				t.Errorf("OnBehalfOf() = %q", userID)
			}
			if _, ok := DelegatedOwnerOf(info); ok {
				t.Error("DelegatedOwnerOf() succeeded")
			}
			if !HasInvalidOnBehalfOf(info) {
				t.Error("HasInvalidOnBehalfOf() = false")
			}
			if owner, err := CallerOwner(info); err == nil {
				t.Errorf("CallerOwner() = %q", owner)
			}
		})
	}
}

// TestOnBehalfOfIsStrippedFromProviderExtras checks that an on-behalf-of user that an auth provider or a credential
// supplied never survives authentication: recording the kind, which every authenticator of users and agents does,
// removes it.
func TestOnBehalfOfIsStrippedFromProviderExtras(t *testing.T) {
	for _, kind := range []Kind{KindUser, KindAgent} {
		t.Run(string(kind), func(t *testing.T) {
			supplied := map[string][]string{
				OnBehalfOfExtra: {"7"},
				DelegationExtra: {`{"userID":"7"}`},
				"email":         {"mallory@example.com"},
			}
			uid := "8"
			if kind == KindAgent {
				uid = "agent:8"
			}
			info := caller(uid, kind, supplied)

			if _, present := info.GetExtra()[OnBehalfOfExtra]; present {
				t.Fatal("the supplied on-behalf-of user survived recording the kind")
			}
			if _, present := info.GetExtra()[DelegationExtra]; present {
				t.Fatal("the supplied delegation record survived recording the kind")
			}
			if userID, ok := OnBehalfOf(info); ok {
				t.Fatalf("OnBehalfOf() = %q", userID)
			}
			if p, ok := FromUserInfo(info); !ok || p.Kind != kind {
				t.Fatalf("FromUserInfo() = %#v, %v; want the %s", p, ok, kind)
			}
		})
	}
}

func TestDelegatedOwnerRoundTrip(t *testing.T) {
	owner := DelegatedOwner{AgentID: 5, UserID: 7}
	encoded := owner.String()
	if encoded != "agent:5/user:7" {
		t.Fatalf("String() = %q", encoded)
	}
	parsed, ok := ParseDelegatedOwner(encoded)
	if !ok || parsed != owner {
		t.Fatalf("ParseDelegatedOwner(%q) = %#v, %v", encoded, parsed, ok)
	}
	if !IsDelegatedOwner(encoded) {
		t.Fatalf("IsDelegatedOwner(%q) = false", encoded)
	}

	for _, malformed := range []string{
		"",
		"7",
		"agent:5",
		"agent:5/user:",
		"agent:/user:7",
		"agent:0/user:7",
		"agent:5/user:0",
		"agent:05/user:7",
		"agent:5/user:07",
		"agent:5/user:7/",
		"agent:5/user:agent:7",
		"agent:5/agent:7",
		"user:7/agent:5",
		"vmcp-oauth:vmcp1abc",
	} {
		if got, ok := ParseDelegatedOwner(malformed); ok {
			t.Errorf("ParseDelegatedOwner(%q) = %#v", malformed, got)
		}
	}

	// A caller's UID of the owner's shape is refused whatever follows the agent prefix.
	for _, shaped := range []string{"agent:5/user:7", "agent:5/anything", "agent:/"} {
		if !IsDelegatedOwner(shaped) {
			t.Errorf("IsDelegatedOwner(%q) = false", shaped)
		}
	}
	for _, other := range []string{"", "7", "agent:5", "vmcp-oauth:vmcp1abc/component"} {
		if IsDelegatedOwner(other) {
			t.Errorf("IsDelegatedOwner(%q) = true", other)
		}
	}
}
