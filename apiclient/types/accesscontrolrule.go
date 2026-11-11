package types

import (
	"fmt"
	"strconv"
)

const (
	SubjectTypeGroup     SubjectType = "group"
	SubjectTypeObotGroup SubjectType = "obotGroup"
	// SubjectTypeUser names one user by ID, or every user with the ID "*" (users:*).
	SubjectTypeUser SubjectType = "user"
	// SubjectTypeSelector with the ID "*" names every user and every agent (subjects:*). In access control rules,
	// which agents never match, it names every user.
	SubjectTypeSelector SubjectType = "selector"
	// SubjectTypeAgent names one agent identity by its bare decimal ID (not its "agent:<id>" storage encoding), or
	// every agent with the ID "*" (agents:*). Only the policy systems whose PolicySystem.allowsAgents is true accept
	// it.
	SubjectTypeAgent SubjectType = "agent"

	// SubjectWildcardID is the ID of a subject that names everyone its type covers.
	SubjectWildcardID = "*"

	// The policy systems whose manifests hold subjects, for Subject.ValidateFor.
	PolicySystemVMCPProfile   PolicySystem = "vmcpProfile"
	PolicySystemModelAccess   PolicySystem = "modelAccess"
	PolicySystemSkillAccess   PolicySystem = "skillAccess"
	PolicySystemAccessControl PolicySystem = "accessControl"

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
// and skill access rules can, and access control rules never do. Only these systems accept agent subjects and the
// user wildcard. In access control rules, {user, "*"} has always matched nobody, and an agent subject would match
// nobody, so ValidateFor refuses both rather than store them with a meaning they do not have.
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
// included; ValidateFor also checks that the policy system accepts it.
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
			return nil
		}
		// Agents are matched by their bare decimal ID, compared as a string, so only the canonical form of one can
		// ever match: "007" or "agent:7" would silently match nobody.
		id, err := strconv.ParseUint(s.ID, 10, 0)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != s.ID {
			return fmt.Errorf("agent subject ID must be '*' or the decimal ID of an agent, such as \"7\" rather than \"agent:7\": %q", s.ID)
		}
		return nil
	}
	return fmt.Errorf("invalid subject type: %s", s.Type)
}

// IsWildcard reports whether the subject names everyone its type covers: everyone ({selector, "*"}), every user
// ({user, "*"}) or every agent ({agent, "*"}).
func (s Subject) IsWildcard() bool {
	switch s.Type {
	case SubjectTypeSelector, SubjectTypeUser, SubjectTypeAgent:
		return s.ID == SubjectWildcardID
	}
	return false
}

// ValidateFor checks the subject as Validate does, and that the policy system accepts it. Only the systems that can
// match agents (PolicySystem.allowsAgents) accept agent subjects and the user wildcard {user, "*"}.
func (s Subject) ValidateFor(system PolicySystem) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if system.allowsAgents() {
		return nil
	}
	switch {
	case s.Type == SubjectTypeAgent:
		return fmt.Errorf("agent subjects are not supported in %s", system.displayName())
	case s.Type == SubjectTypeUser && s.ID == SubjectWildcardID:
		return fmt.Errorf("the user wildcard (user '*') is not supported in %s; use the selector '*' for every user", system.displayName())
	}
	return nil
}

// displayName names a policy system that refuses agent subjects, for ValidateFor's error.
func (p PolicySystem) displayName() string {
	switch p {
	case PolicySystemAccessControl:
		return "access control rules"
	}
	return string(p)
}

// validateWildcardSubjects checks the wildcards of one list of subjects, such as a policy's or a profile's. The
// selector wildcard (subjects:*) already names every user and every agent, so it is not combined with the user
// (users:*) or agent (agents:*) wildcard; and those two together name everyone, which the selector wildcard says
// directly.
func validateWildcardSubjects(subjects []Subject) error {
	selector, users, agents := wildcardKinds(subjects)
	switch {
	case selector && (users || agents):
		return fmt.Errorf("the selector wildcard (*) already includes every user and agent, so it cannot be combined with the user or agent wildcard")
	case users && agents:
		return fmt.Errorf("the user and agent wildcards together include everyone: use the selector wildcard (*) instead")
	}
	return nil
}

// wildcardKinds reports which wildcards subjects hold: the selector wildcard (everyone), the user wildcard and the
// agent wildcard.
func wildcardKinds(subjects []Subject) (selector, users, agents bool) {
	for _, subject := range subjects {
		if !subject.IsWildcard() {
			continue
		}
		switch subject.Type {
		case SubjectTypeSelector:
			selector = true
		case SubjectTypeUser:
			users = true
		case SubjectTypeAgent:
			agents = true
		}
	}
	return selector, users, agents
}

// validatePolicySubjects checks the subjects of a policy in system whose wildcard must be its only subject, such as a
// model access policy or a skill access rule: there is at least one, each is valid there, a wildcard is alone, and
// none is repeated.
func validatePolicySubjects(system PolicySystem, subjects []Subject) error {
	if len(subjects) == 0 {
		return fmt.Errorf("at least one subject is required")
	}

	// The wildcard rules come first, so that combining two wildcards is explained rather than refused only because
	// a wildcard must be alone.
	if err := validateWildcardSubjects(subjects); err != nil {
		return fmt.Errorf("invalid subjects: %w", err)
	}
	seen := make(map[Subject]struct{}, len(subjects))
	for _, subject := range subjects {
		if err := subject.ValidateFor(system); err != nil {
			return fmt.Errorf("invalid subject: %w", err)
		}

		// A wildcard of any kind (*, user *, agent *) must be the only subject.
		if subject.ID == "*" && len(subjects) > 1 {
			return fmt.Errorf("wildcard subject (*) must be the only subject")
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
