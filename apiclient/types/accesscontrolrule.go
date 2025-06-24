package types

type AccessControlRule struct {
	Metadata                  `json:",inline"`
	AccessControlRuleManifest `json:",inline"`
}

type AccessControlRuleManifest struct {
	DisplayName              string   `json:"displayName,omitempty"`
	UserIDs                  []string `json:"userIDs,omitempty"`
	MCPServerCatalogEntryIDs []string `json:"mcpServerCatalogEntryIDs,omitempty"`
	MCPServerIDs             []string `json:"mcpServerIDs,omitempty"`
}

type AccessControlRuleList List[AccessControlRule]
