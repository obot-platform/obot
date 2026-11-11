package v1

import (
	"encoding/json"
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ DeleteRefs    = (*MCPServerCatalogEntry)(nil)
	_ fields.Fields = (*MCPServerCatalogEntry)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   MCPServerCatalogEntrySpec   `json:"spec"`
	Status MCPServerCatalogEntryStatus `json:"status"`
}

type MCPServerCatalogEntrySpec struct {
	// Deprecated: preserve the pre-v2 manifest until the composite migration has completed.
	// It is deliberately not part of the public catalog schema.
	LegacyCompositeManifest json.RawMessage                     `json:"-"`
	Manifest                types.MCPServerCatalogEntryManifest `json:"manifest"`
	UnsupportedTools        []string                            `json:"unsupportedTools,omitempty"`
	MCPCatalogName          string                              `json:"mcpCatalogName,omitempty"`
	Editable                bool                                `json:"editable,omitempty"`
	Detached                bool                                `json:"detached"`
	SourceURL               string                              `json:"sourceURL,omitempty"`
	// PowerUserWorkspaceID contains the name of the PowerUserWorkspace that owns this catalog entry, if there is one.
	PowerUserWorkspaceID string `json:"powerUserWorkspaceID,omitempty"`
}

type MCPServerCatalogEntryStatus struct {
	// UserCount contains the current number of users with an MCP server created from this catalog entry.
	// For multi-user entries, this is the sum of MCPServerInstanceUserCount across each MCPServer created from this entry (not de-duplicated across servers).
	UserCount int `json:"userCount,omitempty"`
	// LastUpdated is the timestamp when this catalog entry was last updated.
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
	// ToolPreviewsLastGenerated is the timestamp when the tool previews were last generated for this catalog entry.
	ToolPreviewsLastGenerated *metav1.Time `json:"toolPreviewsLastGenerated,omitempty"`
	// ManifestHash is a SHA256 hash of the catalog entry configuration used to detect changes.
	ManifestHash string `json:"manifestHash,omitempty"`
	// NeedsUpdate indicates whether this composite catalog entry's component snapshots have drifted from their sources.
	NeedsUpdate bool `json:"needsUpdate,omitempty"`
	// OAuthCredentialConfigured indicates whether OAuth credentials have been configured for this remote catalog entry.
	// Only relevant when Runtime is "remote" and RemoteConfig.StaticOAuthRequired is true.
	OAuthCredentialConfigured bool `json:"oauthCredentialConfigured,omitempty"`
	// StaticConfigurationRevision is the static configuration revision whose superseded revisions
	// have been deleted.
	StaticConfigurationRevision string `json:"staticConfigurationRevision,omitempty"`
	// OpenAPISourceDrift is the read-only result of the latest automatic probe
	// that re-imported this OpenAPI entry's Source and compared it with the
	// stored snapshot. Detection never changes the snapshot; adopting reported
	// changes always requires the explicit save-and-republish flow.
	OpenAPISourceDrift *OpenAPISourceDriftStatus `json:"openapiSourceDrift,omitempty"`
}

// OpenAPISourceDriftStatus reports upstream schema drift for OpenAPI entries.
// It is informational only; nothing is auto-applied.
type OpenAPISourceDriftStatus struct {
	// Detected is true when the probed source compiles to a different
	// operation set or operation signatures than the stored snapshot.
	Detected bool `json:"detected,omitempty"`
	// LastProbedAt is when the probe last completed, successfully or not.
	LastProbedAt metav1.Time `json:"lastProbedAt,omitempty"`
	// NextProbeAt is when the next periodic probe is due.
	NextProbeAt metav1.Time `json:"nextProbeAt,omitempty"`
	// SnapshotDigest is the SHA-256 of the stored normalized snapshot bytes.
	SnapshotDigest string `json:"snapshotDigest,omitempty"`
	// SourceDigest is the SHA-256 of the freshly fetched normalized schema.
	SourceDigest string `json:"sourceDigest,omitempty"`
	// AddedOperations lists operation keys ("METHOD /path") present in the
	// source but not the snapshot. Bounded by the schema size limit.
	AddedOperations []string `json:"addedOperations,omitempty"`
	// RemovedOperations lists operation keys present in the snapshot but not
	// the source, i.e. upstream interfaces that no longer exist.
	RemovedOperations []string `json:"removedOperations,omitempty"`
	// ChangedOperations lists operation keys whose parameter, body, schema, or
	// metadata signature differs between snapshot and source.
	ChangedOperations []string `json:"changedOperations,omitempty"`
	// ProbeError is the bounded error message when the last probe failed
	// (fetch failure, parse failure, or compile failure). Empty on success.
	ProbeError string `json:"probeError,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MCPServerCatalogEntry `json:"items"`
}

func (s *MCPServerCatalogEntrySpec) UnmarshalJSON(data []byte) error {
	type spec MCPServerCatalogEntrySpec
	if err := json.Unmarshal(data, (*spec)(s)); err != nil {
		return err
	}
	s.LegacyCompositeManifest = nil
	if s.Manifest.Runtime == types.RuntimeComposite {
		var raw struct {
			Manifest json.RawMessage `json:"manifest"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		s.LegacyCompositeManifest = raw.Manifest
	}
	return nil
}

func (s MCPServerCatalogEntrySpec) MarshalJSON() ([]byte, error) {
	type spec MCPServerCatalogEntrySpec
	if len(s.LegacyCompositeManifest) == 0 {
		return json.Marshal(spec(s))
	}
	return json.Marshal(struct {
		spec
		Manifest json.RawMessage `json:"manifest"`
	}{spec: spec(s), Manifest: s.LegacyCompositeManifest})
}

func (in *MCPServerCatalogEntry) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"MCP Catalog", "Spec.MCPCatalogName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *MCPServerCatalogEntry) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPServerCatalogEntry) Get(field string) string {
	switch field {
	case "spec.mcpCatalogName":
		return in.Spec.MCPCatalogName
	case "spec.powerUserWorkspaceID":
		return in.Spec.PowerUserWorkspaceID
	case "spec.manifest.runtime":
		return string(in.Spec.Manifest.Runtime)
	case "spec.manifest.remoteConfig.tunnelName":
		if in.Spec.Manifest.RemoteConfig != nil {
			return in.Spec.Manifest.RemoteConfig.TunnelName
		}
		return ""
	}
	return ""
}

func (in *MCPServerCatalogEntry) FieldNames() []string {
	return []string{
		"spec.mcpCatalogName",
		"spec.powerUserWorkspaceID",
		"spec.manifest.runtime",
		"spec.manifest.remoteConfig.tunnelName",
	}
}

func (in *MCPServerCatalogEntry) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &MCPCatalog{}, Name: in.Spec.MCPCatalogName},
		{ObjType: &PowerUserWorkspace{}, Name: in.Spec.PowerUserWorkspaceID},
	}
}

// IsGitManaged mirrors the existing GitOps heuristic: sourced, non-editable,
// non-detached catalog entries are treated as git-managed.
func (in *MCPServerCatalogEntry) IsGitManaged() bool {
	return !in.Spec.Detached && !in.Spec.Editable && in.Spec.SourceURL != ""
}
