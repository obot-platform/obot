package authz

import (
	"net/http"
	"testing"
)

func patternRequest(pattern string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Pattern = pattern
	return req
}

// An administrator manages the record of a sandbox. Reaching into a running one
// is a different act: the terminal is a live console and /agent-connect proxies
// to whatever the agent serves, so both are someone else's working session
// rather than something to administer.
func TestEntersSandbox(t *testing.T) {
	for _, tt := range []struct {
		pattern string
		want    bool
	}{
		{pattern: "GET /api/hosted-agent-instances/{hosted_agent_instance_id}/terminal", want: true},
		{pattern: "/agent-connect/{hosted_agent_instance_id}", want: true},
		{pattern: "/agent-connect/{hosted_agent_instance_id}/{rest...}", want: true},

		// Managing the record, which both roles keep.
		{pattern: "GET /api/hosted-agent-instances", want: false},
		{pattern: "GET /api/hosted-agent-instances/{hosted_agent_instance_id}", want: false},
		{pattern: "PUT /api/hosted-agent-instances/{hosted_agent_instance_id}", want: false},
		{pattern: "DELETE /api/hosted-agent-instances/{hosted_agent_instance_id}", want: false},

		// A route that merely mentions an agent is not a way into one.
		{pattern: "GET /api/hosted-agents/{hosted_agent_id}", want: false},
		{pattern: "GET /api/hosted-agent-pools/{hosted_agent_pool_id}/utilization", want: false},
	} {
		t.Run(tt.pattern, func(t *testing.T) {
			if got := entersSandbox(patternRequest(tt.pattern)); got != tt.want {
				t.Errorf("entersSandbox(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

// An auditor's business is what happened, not what the credentials are.
func TestRevealsSecrets(t *testing.T) {
	for _, tt := range []struct {
		pattern string
		want    bool
	}{
		{pattern: "POST /api/hosted-agents/{hosted_agent_id}/reveal", want: true},

		{pattern: "GET /api/hosted-agents/{hosted_agent_id}", want: false},
		{pattern: "PUT /api/hosted-agents/{hosted_agent_id}", want: false},
		{pattern: "GET /api/hosted-agents", want: false},
	} {
		t.Run(tt.pattern, func(t *testing.T) {
			if got := revealsSecrets(patternRequest(tt.pattern)); got != tt.want {
				t.Errorf("revealsSecrets(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}
