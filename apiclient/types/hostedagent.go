package types

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

type HostedAgent struct {
	Metadata            `json:",inline"`
	HostedAgentManifest `json:",inline"`
}

type HostedAgentManifest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	IconDark    string `json:"iconDark,omitempty"`

	// HarnessID names the Harness this agent runs on. The harness supplies the
	// docker image; the agent supplies configuration.
	HarnessID string `json:"harnessID,omitempty"`
	// GitRepo is an optional git repository made available to the agent.
	GitRepo string `json:"gitRepo,omitempty"`

	// ModelProviders, Models, MCPServers, and Skills are the IDs of configured
	// services made available to the agent at runtime. MCPServers holds MCP
	// gateway IDs, the same handles used by /mcp-connect/{mcp_id}, so an entry
	// may name a catalog entry or a server. Models may also hold obot://<alias>
	// references to default model aliases.
	ModelProviders []string `json:"modelProviders,omitempty"`
	Models         []string `json:"models,omitempty"`
	MCPServers     []string `json:"mcpServers,omitempty"`
	Skills         []string `json:"skills,omitempty"`

	Env []HostedAgentEnv `json:"env,omitempty"`

	// Questions are asked of the user when they create an instance.
	Questions []HostedAgentQuestion `json:"questions,omitempty"`

	// AllowUser* let a user attach their own resources to an instance, on top of
	// the ones configured above.
	AllowUserMCPServers bool `json:"allowUserMCPServers,omitempty"`
	AllowUserSkills     bool `json:"allowUserSkills,omitempty"`
	AllowUserModels     bool `json:"allowUserModels,omitempty"`
	// AllowUserGitRepo lets a user set their own git repository on an instance,
	// overriding the agent's GitRepo if one is configured.
	AllowUserGitRepo bool `json:"allowUserGitRepo,omitempty"`

	// MaxInstancesPerUser caps instances per user. Zero means unlimited.
	MaxInstancesPerUser int `json:"maxInstancesPerUser,omitempty"`
}

// HostedAgentQuestion defines a single value collected from the user when they
// create an instance. It mirrors the shape of MCPEnv/MCPHeader so that the
// definition of a field and its rendering stay consistent across the product,
// with a Type added so answers can be validated.
type HostedAgentQuestion struct {
	Key         string                  `json:"key"`
	Name        string                  `json:"name,omitempty"`
	Description string                  `json:"description,omitempty"`
	Type        HostedAgentQuestionType `json:"type,omitempty"`
	Required    bool                    `json:"required,omitempty"`
	Sensitive   bool                    `json:"sensitive,omitempty"`
	Default     string                  `json:"default,omitempty"`
	// Options enumerates the allowed answers. Required for select, ignored otherwise.
	Options []string `json:"options,omitempty"`
}

type HostedAgentQuestionType string

const (
	HostedAgentQuestionTypeString   HostedAgentQuestionType = "string"
	HostedAgentQuestionTypeNumber   HostedAgentQuestionType = "number"
	HostedAgentQuestionTypeBoolean  HostedAgentQuestionType = "boolean"
	HostedAgentQuestionTypeSelect   HostedAgentQuestionType = "select"
	HostedAgentQuestionTypeSchedule HostedAgentQuestionType = "schedule"
)

// HostedAgentEnv describes an environment variable. Values for entries marked
// Sensitive are never stored on the resource; they live in the credential store
// and Value is blank both in the spec and on the wire.
type HostedAgentEnv struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// HostedAgentState describes the orchestration state of an instance. Agents
// themselves are templates and carry no state; only instances run.
type HostedAgentState string

const (
	HostedAgentStatePending HostedAgentState = "pending"
	HostedAgentStateReady   HostedAgentState = "ready"
	HostedAgentStateError   HostedAgentState = "error"
)

func (m HostedAgentManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.HarnessID == "" {
		return fmt.Errorf("harnessID is required")
	}
	if m.GitRepo != "" {
		if err := ValidateGitRepoURL(m.GitRepo); err != nil {
			return err
		}
	}
	if m.MaxInstancesPerUser < 0 {
		return fmt.Errorf("maxInstancesPerUser must be greater than or equal to 0")
	}

	keys := make(map[string]struct{}, len(m.Env))
	for _, env := range m.Env {
		if env.Key == "" {
			return fmt.Errorf("env key is required")
		}
		if _, ok := keys[env.Key]; ok {
			return fmt.Errorf("duplicate env key: %s", env.Key)
		}
		keys[env.Key] = struct{}{}
	}

	questionKeys := make(map[string]struct{}, len(m.Questions))
	for _, question := range m.Questions {
		if err := question.Validate(); err != nil {
			return fmt.Errorf("invalid question: %w", err)
		}
		if _, ok := questionKeys[question.Key]; ok {
			return fmt.Errorf("duplicate question key: %s", question.Key)
		}
		questionKeys[question.Key] = struct{}{}
	}

	return nil
}

func (q HostedAgentQuestion) Validate() error {
	if q.Key == "" {
		return fmt.Errorf("question key is required")
	}

	switch q.Type {
	case HostedAgentQuestionTypeSelect:
		if len(q.Options) == 0 {
			return fmt.Errorf("question %s: select requires at least one option", q.Key)
		}
		seen := make(map[string]struct{}, len(q.Options))
		for _, option := range q.Options {
			if option == "" {
				return fmt.Errorf("question %s: select options cannot be empty", q.Key)
			}
			if _, ok := seen[option]; ok {
				return fmt.Errorf("question %s: duplicate option %s", q.Key, option)
			}
			seen[option] = struct{}{}
		}
	case HostedAgentQuestionTypeString, HostedAgentQuestionTypeNumber,
		HostedAgentQuestionTypeBoolean, HostedAgentQuestionTypeSchedule, "":
		if len(q.Options) > 0 {
			return fmt.Errorf("question %s: options are only valid for select", q.Key)
		}
	default:
		return fmt.Errorf("question %s: invalid type %s", q.Key, q.Type)
	}

	if q.Default != "" {
		if err := q.ValidateAnswer(q.Default); err != nil {
			return fmt.Errorf("question %s: invalid default: %w", q.Key, err)
		}
	}

	return nil
}

// ValidateAnswer checks a single answer against the question's type. An empty
// answer is treated as unanswered and is the caller's business, not this
// function's, so that required-ness is enforced in one place.
//
// Schedule answers are only checked for shape here. This package is dependency
// free by design, so authoritative cron parsing lives in the server, which
// calls ValidateScheduleAnswers alongside this.
func (q HostedAgentQuestion) ValidateAnswer(answer string) error {
	if answer == "" {
		return nil
	}

	switch q.Type {
	case HostedAgentQuestionTypeNumber:
		if _, err := strconv.ParseFloat(answer, 64); err != nil {
			return fmt.Errorf("must be a number")
		}
	case HostedAgentQuestionTypeBoolean:
		if _, err := strconv.ParseBool(answer); err != nil {
			return fmt.Errorf("must be true or false")
		}
	case HostedAgentQuestionTypeSelect:
		if !slices.Contains(q.Options, answer) {
			return fmt.Errorf("must be one of: %s", strings.Join(q.Options, ", "))
		}
	case HostedAgentQuestionTypeSchedule:
		if fields := len(strings.Fields(answer)); fields < 5 || fields > 6 {
			return fmt.Errorf("must be a valid cron expression")
		}
	}

	return nil
}

// ScheduleAnswers returns the answers belonging to schedule questions, so the
// server can run them through a real cron parser.
func (m HostedAgentManifest) ScheduleAnswers(answers map[string]string) map[string]string {
	var result map[string]string
	for _, question := range m.Questions {
		if question.Type != HostedAgentQuestionTypeSchedule {
			continue
		}
		if answer := answers[question.Key]; answer != "" {
			if result == nil {
				result = map[string]string{}
			}
			result[question.Key] = answer
		}
	}
	return result
}

// ValidateAnswers checks a full set of answers against the agent's questions,
// rejecting missing required answers and answers to questions that don't exist.
func (m HostedAgentManifest) ValidateAnswers(answers map[string]string) error {
	known := make(map[string]struct{}, len(m.Questions))

	for _, question := range m.Questions {
		known[question.Key] = struct{}{}

		answer, ok := answers[question.Key]
		if !ok || answer == "" {
			if question.Required && question.Default == "" {
				return fmt.Errorf("answer for %s is required", question.Key)
			}
			continue
		}

		if err := question.ValidateAnswer(answer); err != nil {
			return fmt.Errorf("answer for %s: %w", question.Key, err)
		}
	}

	for key := range answers {
		if _, ok := known[key]; !ok {
			return fmt.Errorf("unknown answer: %s", key)
		}
	}

	return nil
}

// ApplyAnswerDefaults fills in defaults for questions the user left blank.
func (m HostedAgentManifest) ApplyAnswerDefaults(answers map[string]string) map[string]string {
	if len(m.Questions) == 0 {
		return answers
	}

	result := make(map[string]string, len(answers)+len(m.Questions))
	maps.Copy(result, answers)

	for _, question := range m.Questions {
		if result[question.Key] == "" && question.Default != "" {
			result[question.Key] = question.Default
		}
	}

	return result
}

type HostedAgentList List[HostedAgent]
