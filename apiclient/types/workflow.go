package types

import "strings"

type Workflow struct {
	Metadata
	WorkflowManifest
	ThreadID      string                             `json:"threadID,omitempty"`
	AliasAssigned *bool                              `json:"aliasAssigned,omitempty"`
	AuthStatus    map[string]OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
	// ToolInfo provides information about the tools for this workflow, like which credentials they use and whether that
	// credential has been created. This is a pointer so that we can distinguish between an empty map (no tool information)
	// and nil (tool information not processed yet).
	ToolInfo           *map[string]ToolInfo `json:"toolInfo,omitempty"`
	TextEmbeddingModel string               `json:"textEmbeddingModel,omitempty"`
}

type WorkflowList List[Workflow]

type WorkflowManifest struct {
	AgentManifest `json:",inline"`
	Steps         []Step `json:"steps"`
	Output        string `json:"output"`
}

type EnvVar struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Existing    bool   `json:"existing"`
}

type Step struct {
	ID          string    `json:"id,omitempty"`
	Name        string    `json:"name,omitempty"`
	Description string    `json:"description,omitempty"`
	If          *If       `json:"if,omitempty"`
	While       *While    `json:"while,omitempty"`
	Template    *Template `json:"template,omitempty"`
	Tools       []string  `json:"tools,omitempty"`
	Agents      []string  `json:"agents,omitempty"`
	Workflows   []string  `json:"workflows,omitempty"`

	Step        string   `json:"step,omitempty"`
	Cache       *bool    `json:"cache,omitempty"`
	Temperature *float32 `json:"temperature,omitempty"`
}

func (s *Step) SetCondition(condition string) {
	s.Step = ""
	s.Template = nil
	if s.While != nil {
		s.If = nil
		s.While.Condition = condition
	}
	if s.If != nil {
		s.While = nil
		s.If.Condition = condition
	}
}

func (s *Step) SetArgs(args map[string]string) {
	if s.Template != nil {
		s.Template.Args = args
	}
	s.If = nil
	s.While = nil
	s.Step = ""
}

func (s *Step) SetPrompt(prompt string) {
	s.Step = prompt
	s.Template = nil
	s.While = nil
	s.If = nil
}

type Template struct {
	Name string            `json:"name,omitempty"`
	Args map[string]string `json:"args,omitempty"`
}

func (s Step) Display() string {
	preamble := strings.Builder{}
	preamble.WriteString("> Step(")
	preamble.WriteString(s.ID)
	preamble.WriteString("): ")
	if s.Name != "" {
		preamble.WriteString(s.Name)
	}
	if s.While != nil {
		preamble.WriteString(" while ")
		preamble.WriteString(oneLine(s.While.Condition))
	}
	if s.If != nil {
		preamble.WriteString(" if ")
		preamble.WriteString(oneLine(s.If.Condition))
	}
	if s.Step != "" {
		preamble.WriteString(" ")
		preamble.WriteString(oneLine(s.Step))
	}
	return preamble.String()
}

type SubFlow struct {
	Input    string `json:"input,omitempty"`
	Workflow string `json:"workflow,omitempty"`
}

type If struct {
	Condition string `json:"condition,omitempty"`
	Steps     []Step `json:"steps,omitempty"`
	Else      []Step `json:"else,omitempty"`
}

type While struct {
	Condition string `json:"condition,omitempty"`
	MaxLoops  int    `json:"maxLoops,omitempty"`
	Steps     []Step `json:"steps,omitempty"`
}

func oneLine(s string) string {
	l := strings.Split(s, "\n")[0]
	if len(l) > 80 {
		return l[:80] + "..."
	}
	return l
}

func FindStep(manifest *WorkflowManifest, id string) (_ *Step, parentID string) {
	if manifest == nil || id == "" {
		return nil, ""
	}
	lookupID, _, _ := strings.Cut(id, "{")
	found, parentID := findInSteps("", manifest.Steps, lookupID)
	if found != nil && found.ID != id {
		found = found.DeepCopy()
		found.ID = id
	}
	return found, parentID
}

func findInSteps(parentID string, steps []Step, id string) (*Step, string) {
	for i, step := range steps {
		if step.ID == id {
			return &steps[i], parentID
		}
		if step.While != nil {
			if found, parentID := findInSteps(step.ID, step.While.Steps, id); found != nil {
				return found, parentID
			}
		}
		if step.If != nil {
			if found, parentID := findInSteps(step.ID, step.If.Steps, id); found != nil {
				return found, parentID
			}
			if found, parentID := findInSteps(step.ID, step.If.Else, id); found != nil {
				return found, parentID
			}
		}
	}
	return nil, ""
}
