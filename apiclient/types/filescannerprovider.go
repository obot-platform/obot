package types

type FileScannerProvider struct {
	Metadata
	FileScannerProviderManifest
	FileScannerProviderStatus
}

type FileScannerProviderManifest struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ToolReference string `json:"toolReference"`
}

type FileScannerProviderStatus struct {
	CommonProviderStatus
}

type FileScannerProviderList List[FileScannerProvider]
