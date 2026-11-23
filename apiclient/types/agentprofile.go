package types

import (
	"fmt"
	"slices"
	"strings"
)

const (
	// AgentProfileIDPrefix starts the ID that Obot assigns to every AgentProfile entry.
	AgentProfileIDPrefix = "ap1"
)

// AgentProfile is a profile of an agent: users the agent may act for. An agent acting on behalf of a user whom one of
// its profiles matches has exactly its own grants (the vMCP profiles, model access policies and skill access rules
// that name it), within the scopes of the key it acts with. A profile never narrows or widens them.
type AgentProfile struct {
	// ID is assigned by Obot, and never changes. An entry sent without one is new; an entry sent with one must already
	// be the agent's.
	ID string `json:"id,omitempty"`
	// Name labels the entry for administrators. It is required and unique within the agent.
	Name string `json:"name"`
	// Subjects are the users the entry applies to: users, groups, Obot groups, or the selector "*" (All Obot Users),
	// which must then be the only subject. An agent is never a subject.
	Subjects []Subject `json:"subjects"`
}

// Validate checks the entry on its own: its name and its subjects.
func (a AgentProfile) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if err := validatePolicySubjects(PolicySystemAgentProfile, a.Subjects); err != nil {
		return err
	}
	// Policies accept {user, "*"}, which has always been valid there and names no one. A new kind of subject list need
	// not keep it, and an administrator could mistake it for All Obot Users.
	if slices.Contains(a.Subjects, Subject{Type: SubjectTypeUser, ID: SubjectWildcardID}) {
		return fmt.Errorf("invalid subject: a user subject names one user by ID; use the selector \"*\" for All Obot Users")
	}
	return nil
}
