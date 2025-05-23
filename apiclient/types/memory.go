package types

// Memory represents a single memory item
type Memory struct {
	ID        string `json:"id,omitempty"`
	Content   string `json:"content,omitempty"`
	CreatedAt *Time  `json:"createdAt,omitempty"`
}

// MemoryList represents a list of memories
type MemoryList struct {
	Metadata
	Items []Memory `json:"items"`
}
