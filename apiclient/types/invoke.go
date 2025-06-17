package types

import "github.com/gptscript-ai/go-gptscript"

// +k8s:deepcopy-gen=false

// +k8s:openapi-gen=false
type InvokeResponse struct {
	Events   <-chan Progress
	ThreadID string
}

type PromptResponse struct {
	ID        string            `json:"id,omitempty"`
	Responses map[string]string `json:"response,omitempty"`
}

type Progress struct {
	// RunID should be populated for all progress events to associate this event with a run
	// If RunID is not populated, the event will not specify tied to any particular run
	RunID string `json:"runID,omitempty"`

	// ThreadID may be populated on any event indicating the current thread of the event stream
	ThreadID string `json:"threadID,omitempty"`

	// ParentRunID is the parent run of the run that is specified in the RunID field
	ParentRunID string `json:"parentRunID,omitempty"`

	// Time is the time the event was generated
	Time *Time `json:"time,omitempty"`

	// Content is the output data. The content for all events should be concatenated to form the entire output
	// If you wish to print event and are not concerned with tracking the internal progress when one can just
	// only the content field in a very simple loop
	Content string `json:"content"`

	// ContentID is a unique identifier for the content. This is used to track the content across multiple events.
	// This field applies to Content and ToolInput.Content fields.
	ContentID string `json:"contentID,omitempty"`

	// NOTE: Only one of the follow fields will be populated, never more than one. If none of the below fields are
	// populated, you should only care about the content field which should have some content to print. You should
	// process the below fields first before considering the content field.

	// Some input that was provided to the run
	Input string `json:"input,omitempty"`
	// InputIsStepTemplateInput indicates that the input will be passed to a step template. Later an event will be
	// sent with the step template invoke information in the StepTemplateInvoke field
	InputIsStepTemplateInput bool `json:"inputIsStepTemplateInput,omitempty"`
	// StepTemplateInvoke indicates that a step template is being invoked
	StepTemplateInvoke *StepTemplateInvoke `json:"stepTemplateInvoke,omitempty"`
	// If prompt is set, content will also be set, but you can ignore the content field and instead handle the explicit
	// information in the prompt field which will provider more information for things such as OAuth
	Prompt *Prompt `json:"prompt,omitempty"`
	// The step that is currently being executed. When this is set the following events are assumed to be part of
	// this step until the next step is set. This field is not always set, only set when the set changes
	Step *Step `json:"step,omitempty"`
	// ToolInput indicates the LLM is currently generating tool arguments which can sometime take a while
	ToolInput *ToolInput `json:"toolInput,omitempty"`
	// ToolCall indicates the LLM is currently calling a tool.
	ToolCall *ToolCall `json:"toolCall,omitempty"`
	// WaitingOnModel indicates we are waiting for the model to start responding with content
	WaitingOnModel bool `json:"waitingOnModel,omitempty"`
	// Error indicates that an error occurred
	Error string `json:"error,omitempty"`
	// The run is done, either success or failure
	RunComplete bool `json:"runComplete,omitempty"`
	// ReplayComplete indicates that all existing events have been sent and future events will be new events
	ReplayComplete bool `json:"replayComplete,omitempty"`
	// Username is the username of the user who triggered the run
	Username string `json:"username,omitempty"`
}

type StepTemplateInvoke struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Args        map[string]string `json:"args,omitempty"`
	Result      string            `json:"result,omitempty"`
}

type Prompt struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Time        *Time             `json:"time,omitempty"`
	Message     string            `json:"message,omitempty"`
	Fields      Fields            `json:"fields,omitempty"`
	Sensitive   bool              `json:"sensitive,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Field should match exactly what is in the GPTScript SDK
type Field struct {
	Name        string   `json:"name,omitempty"`
	Sensitive   *bool    `json:"sensitive,omitempty"`
	Description string   `json:"description,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type Fields []Field

func ToFields(fields gptscript.Fields) Fields {
	f := make(Fields, len(fields))
	for i, field := range fields {
		f[i] = Field{
			Name:        field.Name,
			Sensitive:   field.Sensitive,
			Description: field.Description,
			Options:     field.Options,
		}
	}
	return f
}

type ToolInput struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Input       string            `json:"input,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ToolCall struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	TaskID      string            `json:"taskID,omitempty"`
	TaskRunID   string            `json:"taskRunID,omitempty"`
	Input       string            `json:"input,omitempty"`
	Output      string            `json:"output,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
