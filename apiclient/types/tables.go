package types

type Table struct {
	Name string `json:"name"`
}

type TableList List[Table]

// +k8s:deepcopy-gen=false

// +k8s:openapi-gen=false
type TableRow struct {
	Columns []string       `json:"columns,omitempty"`
	Values  map[string]any `json:"values"`
}

// +k8s:deepcopy-gen=false

// +k8s:openapi-gen=false
type TableRowList List[TableRow]
