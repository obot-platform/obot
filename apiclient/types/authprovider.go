package types

type AuthProvider struct {
	Metadata
	AuthProviderManifest
	AuthProviderStatus
}

type AuthProviderManifest struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ToolReference string `json:"toolReference"`
}

type AuthProviderStatus struct {
	CommonProviderStatus
}

type AuthProviderList List[AuthProvider]
