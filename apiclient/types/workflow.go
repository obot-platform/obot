package types

import "strings"

type Workflow struct {
	Metadata
	WorkflowManifest
	AliasAssigned      bool                               `json:"aliasAssigned,omitempty"`
	AuthStatus         map[string]OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
	TextEmbeddingModel string                             `json:"textEmbeddingModel,omitempty"`
}

type WorkflowList List[Workflow]

type WorkflowManifest struct {
	AgentManifest `json:",inline"`
	Credentials   []string      `json:"credentials"`
	Env           []WorkflowEnv `json:"env"`
	Steps         []Step        `json:"steps"`
	Output        string        `json:"output"`
}

type WorkflowEnv struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
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
		preamble.WriteString(oneline(s.While.Condition))
	}
	if s.If != nil {
		preamble.WriteString(" if ")
		preamble.WriteString(oneline(s.If.Condition))
	}
	if s.Step != "" {
		preamble.WriteString(" ")
		preamble.WriteString(oneline(s.Step))
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

func oneline(s string) string {
	l := strings.Split(s, "\n")[0]
	if len(l) > 80 {
		return l[:80] + "..."
	}
	return l
}

func DeleteStep(manifest *WorkflowManifest, id string) *WorkflowManifest {
	if manifest == nil || id == "" {
		return nil
	}

	result := manifest.DeepCopy()
	lookupID, _, _ := strings.Cut(id, "{")
	result.Steps = deleteStep(manifest.Steps, lookupID)
	return result
}

func deleteStep(steps []Step, id string) []Step {
	newSteps := make([]Step, 0, len(steps))
	for _, step := range steps {
		if step.ID != id {
			if step.While != nil {
				step.While.Steps = deleteStep(step.While.Steps, id)
			}
			if step.If != nil {
				step.If.Steps = deleteStep(step.If.Steps, id)
				step.If.Else = deleteStep(step.If.Else, id)
			}
			newSteps = append(newSteps, step)
		}
	}
	return newSteps
}

func AppendStep(manifest *WorkflowManifest, parentID string, step Step) *WorkflowManifest {
	if manifest == nil {
		return nil
	}

	parentID, addToElse := strings.CutSuffix(parentID, "::else")

	result := manifest.DeepCopy()
	if parentID == "" {
		result.Steps = append(result.Steps, step)
		return result
	}

	lookupID, _, _ := strings.Cut(parentID, "{")
	result.Steps = appendStep(result.Steps, lookupID, addToElse, step)
	return result
}

func appendStep(steps []Step, id string, addToElse bool, stepToAdd Step) []Step {
	result := make([]Step, 0, len(steps))

	for _, step := range steps {
		if step.ID != id {
			if step.If != nil {
				step.If.Steps = appendStep(step.If.Steps, id, addToElse, stepToAdd)
				step.If.Else = appendStep(step.If.Else, id, addToElse, stepToAdd)
			}
			if step.While != nil {
				step.While.Steps = appendStep(step.While.Steps, id, addToElse, stepToAdd)
			}
			result = append(result, step)
			continue
		}

		if step.If != nil {
			if addToElse {
				step.If.Else = append(step.If.Else, stepToAdd)
			} else {
				step.If.Steps = append(step.If.Steps, stepToAdd)
			}
		} else if step.While != nil {
			step.While.Steps = append(step.While.Steps, stepToAdd)
		}

		result = append(result, step)
	}

	return result
}

func SetStep(manifest *WorkflowManifest, step Step) {
	id := step.ID
	if manifest == nil || id == "" {
		return
	}
	lookupID, _, _ := strings.Cut(id, "{")
	found, _ := findInSteps("", manifest.Steps, lookupID)
	if found != nil {
		*found = step
	}
	return
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
