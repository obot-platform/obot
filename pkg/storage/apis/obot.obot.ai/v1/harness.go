package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*Harness)(nil)
	_ DeleteRefs    = (*Harness)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Harness struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HarnessSpec   `json:"spec,omitempty"`
	Status HarnessStatus `json:"status,omitempty"`
}

func (in *Harness) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *Harness) Get(field string) string {
	if field == "spec.sourceID" {
		return in.Spec.SourceID
	}
	return ""
}

func (in *Harness) FieldNames() []string {
	return []string{"spec.sourceID"}
}

// DeleteRefs makes harnesses discovered from a source disappear with it.
// Harnesses registered by hand have no SourceID, and an empty ref is skipped
// by cleanup.Cleanup, so they are unaffected. Note this cascade does not
// consult the API-level in-use check: an agent outside the source that
// references a discovered harness is left dangling.
func (in *Harness) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &AgentSource{}, Name: in.Spec.SourceID},
	}
}

func (in *Harness) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.Name"},
		{"Image", "Spec.Manifest.Image"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type HarnessSpec struct {
	Manifest types.HarnessManifest `json:"manifest,omitempty"`

	// SourceID names the AgentSource this harness was discovered from. Empty
	// for harnesses an admin registered by hand, which the sync never touches.
	SourceID string `json:"sourceID,omitempty"`
	// RelativePath is where the harness was found within the source repository.
	// Agents from the same source reference the harness by this path.
	RelativePath string `json:"relativePath,omitempty"`
	// CommitSHA is the source commit this harness was built from.
	CommitSHA string `json:"commitSHA,omitempty"`
}

// HarnessStatus is empty: a harness is configuration only.
type HarnessStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HarnessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Harness `json:"items"`
}
