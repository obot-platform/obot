package types

// +k8s:deepcopy-gen=false

// +k8s:openapi-gen=false
type List[T any] struct {
	Items []T `json:"items"`
}

type Metadata struct {
	ID       string            `json:"id,omitempty"`
	Created  Time              `json:"created,omitempty"`
	Deleted  *Time             `json:"deleted,omitempty"`
	Links    map[string]string `json:"links,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Type     string            `json:"type,omitempty"`
}
