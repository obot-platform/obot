package types

import (
	"encoding/json"
	"fmt"
)

type NanobotConfig struct {
	Metadata
	NanobotConfigManifest
}

type NanobotConfigManifest struct {
	Extends    NanobotStringList        `json:"extends,omitempty"`
	Env        map[string]NanobotEnvDef `json:"env,omitempty"`
	Publish    NanobotPublish           `json:"publish,omitempty"`
	Agents     map[string]NanobotAgent  `json:"agents,omitempty"`
	MCPServers map[string]NanobotServer `json:"mcpServers,omitempty"`
}

type NanobotConfigList List[NanobotConfig]

// NanobotStringList represents a list of strings with special JSON unmarshaling
type NanobotStringList []string

func (s *NanobotStringList) UnmarshalJSON(data []byte) error {
	// Handle string input
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = []string{str}
		return nil
	}

	// Handle array input
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into NanobotStringList", string(data))
}

type NanobotEnvDef struct {
	Default        string            `json:"default,omitempty"`
	Description    string            `json:"description,omitempty"`
	Options        NanobotStringList `json:"options,omitempty"`
	Optional       bool              `json:"optional,omitempty"`
	Sensitive      *bool             `json:"sensitive,omitempty"`
	UseBearerToken bool              `json:"useBearerToken,omitempty"`
}

func (e *NanobotEnvDef) UnmarshalJSON(data []byte) error {
	// Handle string input
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		e.Default = str
		return nil
	}

	// Handle object input
	type Alias NanobotEnvDef
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	return json.Unmarshal(data, aux)
}

type NanobotDynamicInstructions struct {
	Instructions string            `json:"-"`
	MCPServer    string            `json:"mcpServer"`
	Prompt       string            `json:"prompt"`
	Args         map[string]string `json:"args"`
}

func (a NanobotDynamicInstructions) IsPrompt() bool {
	return a.Prompt != ""
}

func (a NanobotDynamicInstructions) IsSet() bool {
	return a.Instructions != "" || a.Prompt != ""
}

func (a NanobotDynamicInstructions) MarshalJSON() ([]byte, error) {
	if a.Instructions != "" {
		return json.Marshal(a.Instructions)
	}

	type Alias NanobotDynamicInstructions
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&a),
	})
}

func (a *NanobotDynamicInstructions) UnmarshalJSON(data []byte) error {
	// Handle string input
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		a.Instructions = str
		return nil
	}

	// Handle object input
	type Alias NanobotDynamicInstructions
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	return json.Unmarshal(data, aux)
}

type NanobotPublish struct {
	Name              string                     `json:"name,omitempty"`
	Introduction      NanobotDynamicInstructions `json:"introduction,omitempty"`
	Version           string                     `json:"version,omitempty"`
	Instructions      string                     `json:"instructions,omitempty"`
	Tools             NanobotStringList          `json:"tools,omitzero"`
	Prompts           NanobotStringList          `json:"prompts,omitzero"`
	Resources         NanobotStringList          `json:"resources,omitzero"`
	ResourceTemplates NanobotStringList          `json:"resourceTemplates,omitzero"`
	MCPServers        NanobotStringList          `json:"mcpServers,omitzero"`
	Entrypoint        NanobotStringList          `json:"entrypoint,omitempty"`
}

type NanobotField struct {
	Description string                  `json:"description,omitempty"`
	Fields      map[string]NanobotField `json:"fields,omitempty"`
	Required    *bool                   `json:"required,omitempty"`
}

func (f NanobotField) MarshalJSON() ([]byte, error) {
	type Alias NanobotField
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&f),
	})
}

func (f *NanobotField) UnmarshalJSON(data []byte) error {
	type Alias NanobotField
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(f),
	}
	return json.Unmarshal(data, aux)
}

type NanobotOutputSchema struct {
	Name        string                  `json:"name,omitempty"`
	Description string                  `json:"description,omitempty"`
	Schema      json.RawMessage         `json:"schema,omitzero"`
	Strict      bool                    `json:"strict,omitempty"`
	Fields      map[string]NanobotField `json:"fields,omitempty"`
}

func (o NanobotOutputSchema) ToSchema() json.RawMessage {
	if len(o.Schema) > 0 {
		return o.Schema
	}
	return json.RawMessage(`{}`)
}

type NanobotAgentReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type NanobotAgentCall struct {
	Name              string               `json:"name,omitempty"`
	Output            *NanobotOutputSchema `json:"output,omitempty"`
	Chat              *bool                `json:"chat,omitempty"`
	ToolChoice        string               `json:"toolChoice,omitempty"`
	Temperature       *json.Number         `json:"temperature,omitempty"`
	TopP              *json.Number         `json:"topP,omitempty"`
	NewThread         *bool                `json:"newThread,omitempty"`
	InputAsToolResult *bool                `json:"inputAsToolResult,omitempty"`
}

func (a NanobotAgentCall) MarshalJSON() ([]byte, error) {
	if a.Name != "" {
		return json.Marshal(a.Name)
	}

	type Alias NanobotAgentCall
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&a),
	})
}

func (a NanobotAgentCall) Merge(other NanobotAgentCall) (result NanobotAgentCall) {
	result = a
	if other.Name != "" {
		result.Name = other.Name
	}
	if other.Output != nil {
		result.Output = other.Output
	}
	if other.Chat != nil {
		result.Chat = other.Chat
	}
	if other.ToolChoice != "" {
		result.ToolChoice = other.ToolChoice
	}
	if other.Temperature != nil {
		result.Temperature = other.Temperature
	}
	if other.TopP != nil {
		result.TopP = other.TopP
	}
	if other.NewThread != nil {
		result.NewThread = other.NewThread
	}
	if other.InputAsToolResult != nil {
		result.InputAsToolResult = other.InputAsToolResult
	}
	return
}

func (a *NanobotAgentCall) UnmarshalJSON(data []byte) error {
	// Handle string input
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		a.Name = str
		return nil
	}

	// Handle object input
	type Alias NanobotAgentCall
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	return json.Unmarshal(data, aux)
}

type NanobotInputSchema struct {
	Name        string                  `json:"name,omitempty"`
	Description string                  `json:"description,omitempty"`
	Schema      json.RawMessage         `json:"schema,omitzero"`
	Fields      map[string]NanobotField `json:"fields,omitempty"`
}

func (i NanobotInputSchema) ToSchema() json.RawMessage {
	if len(i.Schema) > 0 {
		return i.Schema
	}
	return json.RawMessage(`{}`)
}

type NanobotAgent struct {
	Name            string                     `json:"name,omitempty"`
	ShortName       string                     `json:"shortName,omitempty"`
	Description     string                     `json:"description,omitempty"`
	Icon            string                     `json:"icon,omitempty"`
	IconDark        string                     `json:"iconDark,omitempty"`
	StarterMessages NanobotStringList          `json:"starterMessages,omitempty"`
	Instructions    NanobotDynamicInstructions `json:"instructions,omitempty"`
	Model           string                     `json:"model,omitempty"`
	Before          NanobotStringList          `json:"before,omitempty"`
	After           NanobotStringList          `json:"after,omitempty"`
	MCPServers      NanobotStringList          `json:"mcpServers,omitempty"`
	Tools           NanobotStringList          `json:"tools,omitempty"`
	Agents          NanobotStringList          `json:"agents,omitempty"`
	Flows           NanobotStringList          `json:"flows,omitempty"`
	Prompts         NanobotStringList          `json:"prompts,omitzero"`
	Reasoning       *NanobotAgentReasoning     `json:"reasoning,omitempty"`
	ThreadName      string                     `json:"threadName,omitempty"`
	Chat            *bool                      `json:"chat,omitempty"`
	// ToolExtensions  map[string]map[string]any  `json:"toolExtensions,omitempty"`
	ToolChoice  string               `json:"toolChoice,omitempty"`
	Temperature *json.Number         `json:"temperature,omitempty"`
	TopP        *json.Number         `json:"topP,omitempty"`
	Output      *NanobotOutputSchema `json:"output,omitempty"`
	Truncation  string               `json:"truncation,omitempty"`
	MaxTokens   int                  `json:"maxTokens,omitempty"`
	MimeTypes   []string             `json:"mimeTypes,omitempty"`

	Aliases      []string `json:"aliases,omitempty"`
	Cost         float64  `json:"cost,omitempty"`
	Speed        float64  `json:"speed,omitempty"`
	Intelligence float64  `json:"intelligence,omitempty"`
}

type NanobotPrompt struct {
	Description string                  `json:"description,omitempty"`
	Input       map[string]NanobotField `json:"input,omitempty"`
	Template    string                  `json:"template,omitempty"`
}

type NanobotServerSource struct {
	Repo      string `json:"repo,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Commit    string `json:"commit,omitempty"`
	Branch    string `json:"branch,omitempty"`
	SubPath   string `json:"subPath,omitempty"`
	Reference string `json:"reference,omitempty"`
}

func (s *NanobotServerSource) UnmarshalJSON(data []byte) error {
	// Handle string input
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Repo = str
		return nil
	}

	// Handle object input
	type Alias NanobotServerSource
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	return json.Unmarshal(data, aux)
}

type NanobotServer struct {
	Name        string `json:"name,omitempty"`
	ShortName   string `json:"shortName,omitempty"`
	Description string `json:"description,omitempty"`

	Image        string              `json:"image,omitempty"`
	Dockerfile   string              `json:"dockerfile,omitempty"`
	Source       NanobotServerSource `json:"source,omitempty"`
	Sandboxed    bool                `json:"sandboxed,omitempty"`
	Env          map[string]string   `json:"env,omitempty"`
	Command      string              `json:"command,omitempty"`
	Args         []string            `json:"args,omitempty"`
	BaseURL      string              `json:"url,omitempty"`
	Ports        []string            `json:"ports,omitempty"`
	ReversePorts []int               `json:"reversePorts"`
	Cwd          string              `json:"cwd,omitempty"`
	Workdir      string              `json:"workdir,omitempty"`
	Headers      map[string]string   `json:"headers,omitempty"`
}
