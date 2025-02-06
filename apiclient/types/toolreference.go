package types

type ToolReferenceType string

const (
	ToolReferenceTypeTool                    ToolReferenceType = "tool"
	ToolReferenceTypeStepTemplate            ToolReferenceType = "stepTemplate"
	ToolReferenceTypeKnowledgeDataSource     ToolReferenceType = "knowledgeDataSource"
	ToolReferenceTypeKnowledgeDocumentLoader ToolReferenceType = "knowledgeDocumentLoader"
	ToolReferenceTypeSystem                  ToolReferenceType = "system"
	ToolReferenceTypeModelProvider           ToolReferenceType = "modelProvider"
	ToolReferenceTypeAuthProvider            ToolReferenceType = "authProvider"
	ToolReferenceTypeDaemonTriggerProvider   ToolReferenceType = "daemonTriggerProvider"
)

type ToolReferenceManifest struct {
	Name      string            `json:"name"`
	ToolType  ToolReferenceType `json:"toolType"`
	Reference string            `json:"reference,omitempty"`
	Active    bool              `json:"active,omitempty"`
}

type ToolReference struct {
	Metadata
	ToolReferenceManifest
	Resolved    bool              `json:"resolved,omitempty"`
	Error       string            `json:"error,omitempty"`
	Builtin     bool              `json:"builtin,omitempty"`
	Description string            `json:"description,omitempty"`
	Credentials []string          `json:"credentials,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
}

type ToolReferenceList List[ToolReference]
