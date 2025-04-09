package types

import "strings"

type Workflow struct {
	Metadata
	WorkflowManifest
	ThreadID string `json:"threadID,omitempty"`
}

type WorkflowList List[Workflow]

type WorkflowManifest struct {
	Alias            string                `json:"alias"`
	Steps            []Step                `json:"steps"`
	Params           map[string]string     `json:"params,omitempty"`
	Output           string                `json:"output"`
	Name             string                `json:"name,omitempty"`
	Description      string                `json:"description,omitempty"`
	OnSlackMessage   *TaskOnSlackMessage   `json:"onSlackMessage,omitempty"`
	OnDiscordMessage *TaskOnDiscordMessage `json:"onDiscordMessage,omitempty"`
}

type EnvVar struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Existing    bool   `json:"existing"`
}

type Step struct {
	ID   string   `json:"id,omitempty"`
	Step string   `json:"step,omitempty"`
	Loop []string `json:"loop,omitempty"`
}

func (s Step) Display() string {
	preamble := strings.Builder{}
	preamble.WriteString("> Step(")
	preamble.WriteString(s.ID)
	preamble.WriteString("): ")
	if s.Step != "" {
		preamble.WriteString(" ")
		preamble.WriteString(oneLine(s.Step))
	}
	return preamble.String()
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
	}
	return nil, ""
}
