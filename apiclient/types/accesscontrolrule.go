package types

import (
	"fmt"
	"strconv"
)

const (
	SubjectTypeGroup     SubjectType = "group"
	SubjectTypeObotGroup SubjectType = "obotGroup"
	SubjectTypeUser      SubjectType = "user"
	// SubjectTypeSelector with the ID "*" names every user (subjects:*), in every policy system. It never names an
	// agent.
	SubjectTypeSelector SubjectType = "selector"
	// SubjectTypeAgent names one agent identity by its bare decimal ID (not its "agent:<id>" storage encoding). It is
	// the only subject that names an agent: there is no wildcard for agents, so granting or restricting an agent always
	// names it. Only the policy systems whose PolicySystem.allowsAgents is true accept it.
	SubjectTypeAgent SubjectType = "agent"

	// SubjectWildcardID is the ID of the selector subject that names every user.
	SubjectWildcardID = "*"

	// The policy systems whose manifests hold subjects, for Subject.ValidateFor.
	PolicySystemVMCPProfile   PolicySystem = "vmcpProfile"
	PolicySystemModelAccess   PolicySystem = "modelAccess"
	PolicySystemSkillAccess   PolicySystem = "skillAccess"
	PolicySystemAccessControl PolicySystem = "accessControl"
	// PolicySystemAgentProfile holds the subjects of an agent's profiles: the users an agent may act
	// for. An agent never acts for another agent, so it refuses agent subjects.
	PolicySystemAgentProfile PolicySystem = "agentProfile"

	ResourceTypeMCPServerCatalogEntry ResourceType = "mcpServerCatalogEntry"
	ResourceTypeMCPServer             ResourceType = "mcpServer"
	ResourceTypeMcpCatalog            ResourceType = "mcpCatalog"
	ResourceTypeSelector              ResourceType = "selector"
)

type AccessControlRule struct {
	Metadata                  `json:",inline"`
	MCPCatalogID              string `json:"mcpCatalogID"`
	PowerUserWorkspaceID      string `json:"powerUserWorkspaceID,omitempty"`
	PowerUserID               string `json:"powerUserID,omitempty"`
	Generated                 bool   `json:"generated,omitempty"`
	AccessControlRuleManifest `json:",inline"`
}

type AccessControlRuleManifest struct {
	DisplayName string     `json:"displayName,omitempty"`
	Subjects    []Subject  `json:"subjects,omitempty"`
	Resources   []Resource `json:"resources,omitempty"`
}

type Subject struct {
	Type SubjectType `json:"type"`
	ID   string      `json:"id"`
}

type SubjectType string

// PolicySystem is a kind of policy whose manifest holds subjects. Each has its own matcher, and they differ in which
// identities they can match.
type PolicySystem string

// allowsAgents reports whether the policy system's subjects can match agents: vMCP profiles, model access policies
// and skill access rules can, and access control rules and agent profiles never do. Only these systems accept agent
// subjects. In access control rules an agent subject would match nobody, so ValidateFor refuses it rather than store it
// with a meaning it does not have.
func (p PolicySystem) allowsAgents() bool {
	switch p {
	case PolicySystemVMCPProfile, PolicySystemModelAccess, PolicySystemSkillAccess:
		return true
	}
	return false
}

type Resource struct {
	Type ResourceType `json:"type"`
	ID   string       `json:"id"`
}

type ResourceType string

type AccessControlRuleList List[AccessControlRule]

func (a AccessControlRuleManifest) Validate() error {
	for _, resource := range a.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("invalid resource: %v", err)
		}
	}
	for _, subject := range a.Subjects {
		if err := subject.ValidateFor(PolicySystemAccessControl); err != nil {
			return fmt.Errorf("invalid subject: %v", err)
		}
	}
	return nil
}

// Validate checks the subject on its own, whatever policy system holds it. It accepts every subject type, agents
// included, but an agent only by its ID; ValidateFor also checks that the policy system accepts it.
func (s Subject) Validate() error {
	switch s.Type {
	case SubjectTypeUser, SubjectTypeGroup, SubjectTypeObotGroup:
		if s.ID == "" {
			return fmt.Errorf("subject ID is required")
		}
		return nil
	case SubjectTypeSelector:
		if s.ID != SubjectWildcardID {
			return fmt.Errorf("selector subject ID must be '*'")
		}
		return nil
	case SubjectTypeAgent:
		if s.ID == SubjectWildcardID {
			return fmt.Errorf("agents must be named by ID: there is no subject for every agent, and the selector '*' names every user but no agent")
		}
		// Agents are matched by their bare decimal ID, compared as a string, so only the canonical form of one can
		// ever match: "007" or "agent:7" would silently match nobody.
		id, err := strconv.ParseUint(s.ID, 10, 0)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != s.ID {
			return fmt.Errorf("agent subject ID must be the decimal ID of an agent, such as \"7\" rather than \"agent:7\": %q", s.ID)
		}
		return nil
	}
	return fmt.Errorf("invalid subject type: %s", s.Type)
}

// ValidateFor checks the subject as Validate does, and that the policy system accepts it. Only the systems that can
// match agents (PolicySystem.allowsAgents) accept agent subjects.
func (s Subject) ValidateFor(system PolicySystem) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Type == SubjectTypeAgent && !system.allowsAgents() {
		return fmt.Errorf("agent subjects are not supported in %s", system.displayName())
	}
	return nil
}

// displayName names a policy system that refuses agent subjects, for ValidateFor's error.
func (p PolicySystem) displayName() string {
	switch p {
	case PolicySystemAccessControl:
		return "access control rules"
	case PolicySystemAgentProfile:
		return "agent profiles"
	}
	return string(p)
}

// validatePolicySubjects checks the subjects of a policy in system whose wildcard must be its only subject, such as a
// model access policy or a skill access rule: there is at least one, each is valid there, none is repeated, and a
// wildcard is alone apart from agents. The wildcard is every user and no agent, so a policy for every user that also
// grants an agent must name the agent next to it; any other subject next to it is already included.
func validatePolicySubjects(system PolicySystem, subjects []Subject) error {
	if len(subjects) == 0 {
		return fmt.Errorf("at least one subject is required")
	}

	wildcards := 0
	for _, subject := range subjects {
		if subject.ID == SubjectWildcardID {
			wildcards++
		}
	}

	seen := make(map[Subject]struct{}, len(subjects))
	for _, subject := range subjects {
		if err := subject.ValidateFor(system); err != nil {
			return fmt.Errorf("invalid subject: %w", err)
		}

		if wildcards > 0 && len(subjects) > 1 && (subject.ID == SubjectWildcardID && wildcards > 1 ||
			subject.ID != SubjectWildcardID && subject.Type != SubjectTypeAgent) {
			return fmt.Errorf("wildcard subject (*) must be the only subject, apart from agents")
		}

		if _, ok := seen[subject]; ok {
			return fmt.Errorf("duplicate subject: %s/%s", subject.Type, subject.ID)
		}
		seen[subject] = struct{}{}
	}
	return nil
}

func (r Resource) Validate() error {
	switch r.Type {
	case ResourceTypeMCPServerCatalogEntry, ResourceTypeMCPServer, ResourceTypeMcpCatalog:
		if r.ID == "" {
			return fmt.Errorf("resource ID is required")
		}
		return nil
	case ResourceTypeSelector:
		if r.ID != "*" {
			// We will change this in the future when we support selectors.
			return fmt.Errorf("selector resource ID must be '*'")
		}
		return nil
	}
	return fmt.Errorf("invalid resource type: %s", r.Type)
}
