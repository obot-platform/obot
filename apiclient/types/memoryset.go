package types

type MemorySet struct {
	Metadata
	MemorySetManifest
}

// SlackReceiverManifest defines the configuration for a Slack receiver
type MemorySetManifest struct {
	Memories []Memory `json:"memories,omitempty"`
}

type Memory struct {
	ID        string `json:"id,omitempty"`
	Content   string `json:"content,omitempty"`
	CreatedAt Time   `json:"createdAt,omitempty"`
}

type MemorySetList List[MemorySet]
