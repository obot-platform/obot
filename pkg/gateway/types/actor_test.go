package types

import (
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func callerOfKind(uid string, kind principal.Kind) kuser.Info {
	extra := map[string][]string{}
	principal.RecordKind(extra, kind)
	return &kuser.DefaultInfo{UID: uid, Extra: extra}
}

func TestActorForCaller(t *testing.T) {
	for name, test := range map[string]struct {
		caller kuser.Info
		want   Actor
	}{
		"user": {
			caller: callerOfKind("7", principal.KindUser),
			want:   Actor{PrincipalKind: "user", UserID: "7"},
		},
		"agent has no user ID": {
			caller: callerOfKind("agent:7", principal.KindAgent),
			want:   Actor{PrincipalKind: "agent", PrincipalID: "7"},
		},
		"a caller that is not a principal keeps its UID": {
			caller: &kuser.DefaultInfo{UID: "system:serviceaccount"},
			want:   Actor{UserID: "system:serviceaccount"},
		},
		"an agent UID recorded as a user is not an agent": {
			caller: callerOfKind("agent:7", principal.KindUser),
			want:   Actor{UserID: "agent:7"},
		},
		"a decimal UID recorded as an agent is not a user": {
			caller: callerOfKind("7", principal.KindAgent),
			want:   Actor{UserID: "7"},
		},
		"no caller": {
			caller: nil,
			want:   Actor{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ActorForCaller(test.caller); got != test.want {
				t.Errorf("ActorForCaller() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestActorForOwner(t *testing.T) {
	for name, test := range map[string]struct {
		owner string
		want  Actor
	}{
		"user": {
			owner: "7",
			want:  Actor{PrincipalKind: "user", UserID: "7"},
		},
		"agent": {
			owner: "agent:7",
			want:  Actor{PrincipalKind: "agent", PrincipalID: "7"},
		},
		"something that is not an owner keeps being the user ID": {
			owner: "someone@example.com",
			want:  Actor{UserID: "someone@example.com"},
		},
		"empty": {
			owner: "",
			want:  Actor{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ActorForOwner(test.owner); got != test.want {
				t.Errorf("ActorForOwner() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestActorRefAndOwner(t *testing.T) {
	for name, test := range map[string]struct {
		actor     Actor
		wantRef   apitypes.PrincipalRef
		wantOwner string
	}{
		"agent": {
			actor:     Actor{PrincipalKind: "agent", PrincipalID: "7"},
			wantRef:   apitypes.PrincipalRef{Kind: apitypes.PrincipalKindAgent, ID: "7"},
			wantOwner: "agent:7",
		},
		"a user's row, including one from before agents, reads as its user": {
			actor:     Actor{PrincipalKind: "user", UserID: "7"},
			wantRef:   apitypes.PrincipalRef{Kind: apitypes.PrincipalKindUser, ID: "7"},
			wantOwner: "7",
		},
		"a row with no principal": {
			actor:   Actor{PrincipalKind: "user"},
			wantRef: apitypes.PrincipalRef{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := test.actor.Ref(); got != test.wantRef {
				t.Errorf("Ref() = %+v, want %+v", got, test.wantRef)
			}
			if got := test.actor.Owner(); got != test.wantOwner {
				t.Errorf("Owner() = %q, want %q", got, test.wantOwner)
			}
		})
	}
}
