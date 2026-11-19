package principal

import (
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestParseAndString(t *testing.T) {
	for _, tt := range []struct {
		name    string
		encoded string
		want    Principal
		// canonical is the encoding String returns, when it differs from encoded.
		canonical string
	}{
		{
			name:    "user",
			encoded: "5",
			want:    User(5),
		},
		{
			name:    "agent",
			encoded: "agent:12",
			want:    Agent(12),
		},
		{
			name:      "user with leading zeros",
			encoded:   "007",
			want:      User(7),
			canonical: "7",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.encoded)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tt.encoded, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.encoded, got, tt.want)
			}
			wantString := tt.encoded
			if tt.canonical != "" {
				wantString = tt.canonical
			}
			if s := got.String(); s != wantString {
				t.Fatalf("String() = %q, want %q", s, wantString)
			}
			if again, err := Parse(got.String()); err != nil || again != got {
				t.Fatalf("Parse(String()) = %#v, %v; want %#v", again, err, got)
			}
		})
	}
}

func TestParseRejectsInvalidEncodings(t *testing.T) {
	for _, encoded := range []string{
		"",
		"agent:",
		"agent:abc",
		"agent:-1",
		"user:5",
		"5x",
		"-5",
		"+5",
		" 5",
		"anonymous",
		"Agent:5",
	} {
		if got, err := Parse(encoded); err == nil {
			t.Errorf("Parse(%q) = %#v, want error", encoded, got)
		}
	}
}

func TestPrincipalIDs(t *testing.T) {
	for _, tt := range []struct {
		name        string
		principal   Principal
		wantUserID  uint
		wantAgentID uint
	}{
		{
			name:       "user",
			principal:  User(5),
			wantUserID: 5,
		},
		{
			name:        "agent",
			principal:   Agent(5),
			wantAgentID: 5,
		},
		{
			name: "zero principal",
		},
		{
			name: "user with a malformed ID",
			principal: Principal{
				Kind: KindUser,
				ID:   "abc",
			},
		},
		{
			name: "agent with a malformed ID",
			principal: Principal{
				Kind: KindAgent,
				ID:   "abc",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if id, ok := tt.principal.UserID(); id != tt.wantUserID || ok != (tt.wantUserID != 0) {
				t.Fatalf("UserID() = %d, %v; want %d", id, ok, tt.wantUserID)
			}
			if id, ok := tt.principal.AgentID(); id != tt.wantAgentID || ok != (tt.wantAgentID != 0) {
				t.Fatalf("AgentID() = %d, %v; want %d", id, ok, tt.wantAgentID)
			}
		})
	}
}

func TestFromUserInfo(t *testing.T) {
	marked := func(uid string, kind Kind, groups ...string) kuser.Info {
		extra := map[string][]string{}
		RecordKind(extra, kind)
		return &kuser.DefaultInfo{
			UID:    uid,
			Groups: groups,
			Extra:  extra,
		}
	}

	for _, tt := range []struct {
		name   string
		user   kuser.Info
		want   Principal
		wantOK bool
	}{
		{
			name:   "user marked as a user",
			user:   marked("7", KindUser, types.GroupAuthenticated),
			want:   User(7),
			wantOK: true,
		},
		{
			name:   "agent marked as an agent",
			user:   marked("agent:9", KindAgent),
			want:   Agent(9),
			wantOK: true,
		},
		{
			name:   "user API key with device-scans",
			user:   marked("42", KindUser, types.GroupDeviceScans, types.GroupAuthenticated),
			want:   User(42),
			wantOK: true,
		},
		{
			name: "decimal UID without a marker",
			user: &kuser.DefaultInfo{UID: "7", Groups: []string{types.GroupAuthenticated}},
		},
		{
			name: "agent UID without a marker",
			user: &kuser.DefaultInfo{UID: "agent:9"},
		},
		{
			name: "decimal UID marked as an agent",
			user: marked("7", KindAgent),
		},
		{
			name: "agent UID marked as a user",
			user: marked("agent:9", KindUser),
		},
		{
			name: "malformed agent UID marked as an agent",
			user: marked("agent:nine", KindAgent),
		},
		{
			name: "malformed UID marked as a user",
			user: marked("anonymous", KindUser),
		},
		{
			name: "unknown marker",
			user: marked("7", Kind("robot")),
		},
		{
			name: "empty marker",
			user: marked("7", Kind("")),
		},
		{
			name: "more than one marker",
			user: &kuser.DefaultInfo{
				UID:   "7",
				Extra: map[string][]string{KindExtra: {string(KindUser), string(KindUser)}},
			},
		},
		{
			name: "nil",
		},
		{
			name: "anonymous",
			user: &kuser.DefaultInfo{UID: "anonymous", Name: "anonymous", Groups: []string{"unauthenticated"}},
		},
		{
			name: "metrics",
			user: &kuser.DefaultInfo{UID: "metrics", Name: "metrics", Groups: []string{"metrics"}},
		},
		{
			name: "tunnel peer with a decimal peer ID",
			user: &kuser.DefaultInfo{UID: "42", Groups: []string{types.GroupTunnelPeer}},
		},
		{
			name: "SCIM connection",
			user: &kuser.DefaultInfo{UID: "4b0e4d1c-46f5-4a39-9ad6-3c1f1e6b8f80", Groups: []string{types.GroupSCIM}},
		},
		{
			name: "device",
			user: &kuser.DefaultInfo{
				UID:    "device:abc",
				Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
				Extra:  map[string][]string{"device_id": {"abc"}},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FromUserInfo(tt.user)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("FromUserInfo() = %#v, %v; want %#v, %v", got, ok, tt.want, tt.wantOK)
			}
			// IsAgent holds only for a well-formed agent; ClaimsAgent is the lenient check for malformed ones.
			if got, want := IsAgent(tt.user), tt.want.Kind == KindAgent; got != want {
				t.Fatalf("IsAgent() = %v, want %v", got, want)
			}
		})
	}
}

func TestRecordKindReplacesSuppliedValue(t *testing.T) {
	extra := map[string][]string{KindExtra: {string(KindAgent), string(KindAgent)}}
	RecordKind(extra, KindUser)
	if got := extra[KindExtra]; len(got) != 1 || got[0] != string(KindUser) {
		t.Fatalf("%s = %q, want [%q]", KindExtra, got, KindUser)
	}
}

func TestClaimsAgent(t *testing.T) {
	withKinds := func(uid string, kinds ...Kind) kuser.Info {
		extra := map[string][]string{}
		for _, kind := range kinds {
			extra[KindExtra] = append(extra[KindExtra], string(kind))
		}
		return &kuser.DefaultInfo{
			UID:   uid,
			Extra: extra,
		}
	}

	for _, tt := range []struct {
		name string
		user kuser.Info
		want bool
	}{
		{
			name: "agent",
			user: withKinds("agent:7", KindAgent),
			want: true,
		},
		{
			name: "user",
			user: withKinds("7", KindUser),
		},
		{
			name: "user with no recorded kind",
			user: withKinds("7"),
		},
		{
			name: "system principal",
			user: withKinds("system"),
		},
		{
			name: "agent UID recorded as a user",
			user: withKinds("agent:7", KindUser),
			want: true,
		},
		{
			name: "agent UID with no recorded kind",
			user: withKinds("agent:7"),
			want: true,
		},
		{
			name: "malformed agent UID",
			user: withKinds("agent:seven"),
			want: true,
		},
		{
			name: "user UID recorded as an agent",
			user: withKinds("7", KindAgent),
			want: true,
		},
		{
			name: "kinds recorded more than once",
			user: withKinds("7", KindUser, KindAgent),
			want: true,
		},
		{
			name: "nil",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClaimsAgent(tt.user); got != tt.want {
				t.Fatalf("ClaimsAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAgent(t *testing.T) {
	for _, tt := range []struct {
		owner      string
		wantID     string
		wantOK     bool
		wantClaims bool
	}{
		{
			owner:      "agent:7",
			wantID:     "7",
			wantOK:     true,
			wantClaims: true,
		},
		{
			owner:      "agent:007",
			wantID:     "7",
			wantOK:     true,
			wantClaims: true,
		},
		{
			owner: "7",
		},
		{
			owner:      "agent:seven",
			wantClaims: true,
		},
		{
			owner:      "agent:",
			wantClaims: true,
		},
		{
			owner: "user:7",
		},
		{
			owner: "",
		},
	} {
		t.Run(tt.owner, func(t *testing.T) {
			id, ok := ParseAgent(tt.owner)
			if id != tt.wantID || ok != tt.wantOK {
				t.Fatalf("ParseAgent(%q) = %q, %v; want %q, %v", tt.owner, id, ok, tt.wantID, tt.wantOK)
			}
			if got := ClaimsAgentOwner(tt.owner); got != tt.wantClaims {
				t.Fatalf("ClaimsAgentOwner(%q) = %v, want %v", tt.owner, got, tt.wantClaims)
			}
		})
	}
}

func TestCallerOwner(t *testing.T) {
	marked := func(uid string, kind Kind) kuser.Info {
		extra := map[string][]string{}
		RecordKind(extra, kind)
		return &kuser.DefaultInfo{
			UID:   uid,
			Extra: extra,
		}
	}

	for _, tt := range []struct {
		name    string
		caller  kuser.Info
		want    string
		wantErr bool
	}{
		{
			name:   "user",
			caller: marked("7", KindUser),
			want:   "7",
		},
		{
			// An agent's owner never collides with the user's with the same numeric ID.
			name:   "agent",
			caller: marked("agent:7", KindAgent),
			want:   "agent:7",
		},
		{
			name:   "caller without a recorded kind",
			caller: &kuser.DefaultInfo{UID: "system"},
			want:   "system",
		},
		{
			name:    "malformed agent",
			caller:  marked("7", KindAgent),
			wantErr: true,
		},
		{
			name:    "agent UID without a recorded kind",
			caller:  &kuser.DefaultInfo{UID: "agent:7"},
			wantErr: true,
		},
		{
			name:    "agent OAuth grant owner",
			caller:  marked(AgentOAuthGrantOwner("vmcp1"), KindUser),
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CallerOwner(tt.caller)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CallerOwner() error = %v, want error %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("CallerOwner() = %q, want %q", got, tt.want)
			}
			if !tt.wantErr {
				if _, isAgent := ParseAgent(got); isAgent != IsAgent(tt.caller) {
					t.Fatalf("IsAgent() disagrees with the owner %q", got)
				}
			}
		})
	}
}
