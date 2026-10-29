package server

import (
	"slices"
	"testing"

	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/provenance"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestProvenanceSubject(t *testing.T) {
	tests := []struct {
		name            string
		user            kuser.Info
		userAgent       string
		clientSessionID string
		wantSubject     provenance.Subject
		wantKey         provenance.SessionKey
	}{
		{
			name: "claude code with an api key",
			user: &kuser.DefaultInfo{
				UID: "u1",
				Extra: map[string][]string{
					"email":                   {"craig@example.com"},
					"auth_provider_groups":    {"eng", "sec"},
					principal.APIKeyIDExtra:   {"42"},
					principal.APIKeyNameExtra: {"laptop"},
				},
			},
			userAgent:       "claude-cli/2.0.0 (external, cli)",
			clientSessionID: "cc-session-1",
			wantSubject: provenance.Subject{
				UserID: "u1",
				Email:  "craig@example.com",
				Groups: []string{"eng", "sec"},
				Actor:  provenance.Actor{Agent: "claude_code", AgentVersion: "2.0.0", APIKeyID: "42", ClientSessionID: "cc-session-1"},
			},
			wantKey: provenance.SessionKey{ClientSessionID: "cc-session-1", Agent: "claude_code", UserID: "u1", APIKeyID: "42"},
		},
		{
			name:      "unknown client without a session",
			user:      &kuser.DefaultInfo{UID: "u2"},
			userAgent: "python-requests/2.31",
			wantSubject: provenance.Subject{
				UserID: "u2",
				Actor:  provenance.Actor{Agent: "python-requests", AgentVersion: "2.31"},
			},
			wantKey: provenance.SessionKey{Agent: "python-requests", UserID: "u2"},
		},
		{
			name: "no user agent",
			user: &kuser.DefaultInfo{UID: "u3"},
			wantSubject: provenance.Subject{
				UserID: "u3",
				Actor:  provenance.Actor{Agent: "client"},
			},
			wantKey: provenance.SessionKey{Agent: "client", UserID: "u3"},
		},
		{
			name:        "nil user",
			userAgent:   "codex_cli_rs/0.42.0",
			wantSubject: provenance.Subject{Actor: provenance.Actor{Agent: "codex", AgentVersion: "0.42.0"}},
			wantKey:     provenance.SessionKey{Agent: "codex"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, key := provenanceSubject(tt.user, tt.userAgent, tt.clientSessionID)
			if subject.UserID != tt.wantSubject.UserID || subject.Email != tt.wantSubject.Email || !slices.Equal(subject.Groups, tt.wantSubject.Groups) {
				t.Errorf("subject = %+v, want %+v", subject, tt.wantSubject)
			}
			if !actorEqual(subject.Actor, tt.wantSubject.Actor) {
				t.Errorf("actor = %+v, want %+v", subject.Actor, tt.wantSubject.Actor)
			}
			if key != tt.wantKey {
				t.Errorf("key = %+v, want %+v", key, tt.wantKey)
			}
		})
	}
}

func actorEqual(a, b provenance.Actor) bool {
	return a.Agent == b.Agent && a.AgentVersion == b.AgentVersion && a.DeviceID == b.DeviceID && a.EnvironmentID == b.EnvironmentID &&
		a.APIKeyID == b.APIKeyID && a.ClientSessionID == b.ClientSessionID && slices.Equal(a.Delegation, b.Delegation)
}
