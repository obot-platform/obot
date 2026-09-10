package types

type ModelProxySettings struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

type ModelProxySettingsUpdate struct {
	Enabled *bool `json:"enabled"`
}

type ModelProxyTokenUsage struct {
	Used int64 `json:"used"`
	Max  int64 `json:"max"`
}

type ModelProxyUsage struct {
	Input   ModelProxyTokenUsage `json:"input"`
	Output  ModelProxyTokenUsage `json:"output"`
	ResetAt Time                 `json:"resetAt"`
}
