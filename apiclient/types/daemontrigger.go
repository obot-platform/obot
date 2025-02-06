package types

type DaemonTrigger struct {
	Metadata
	DaemonTriggerManifest
	// TODO(njhale): Figure out what else should be here
}

type DaemonTriggerManifest struct {
	Workflow          string            `json:"workflow"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	ProviderName      string            `json:"providerName"`
	ProviderNamespace string            `json:"providerNamespace"`
	Config            map[string]string `json:"config"`
}

type DaemonTriggerList List[DaemonTrigger]
