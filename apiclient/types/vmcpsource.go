package types

// VMCPSourceManifest is a Git/local catalog definition. Runtime snapshots and
// component IDs are resolved by sync, never accepted from the source.
type VMCPSourceManifest struct {
	DisplayName string                `json:"displayName"`
	Description string                `json:"description,omitempty"`
	Icon        string                `json:"icon,omitempty"`
	Components  []VMCPSourceComponent `json:"components"`
	// Profile tool maps use the full source-qualified component entryKey.
	Profiles []VMCPProfile `json:"profiles,omitempty"`
}

type VMCPSourceComponent struct {
	Name            string                    `json:"name"`
	EntryKey        string                    `json:"entryKey"`
	Configuration   []VMCPConfigurationPolicy `json:"configuration,omitempty"`
	ForceSingleUser bool                      `json:"forceSingleUser,omitempty"`
	AllowedTools    []string                  `json:"allowedTools,omitempty"`
	ToolPrefix      string                    `json:"toolPrefix,omitempty"`
	ToolOverrides   []ToolOverride            `json:"toolOverrides,omitempty"`
}
