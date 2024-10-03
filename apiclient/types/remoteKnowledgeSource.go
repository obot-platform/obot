package types

var (
	RemoteKnowledgeSourceTypeOneDrive RemoteKnowledgeSourceType = "onedrive"
	RemoteKnowledgeSourceTypeNotion   RemoteKnowledgeSourceType = "notion"
	RemoteKnowledgeSourceTypeWebsite  RemoteKnowledgeSourceType = "website"
)

type RemoteKnowledgeSourceType string

type RemoteKnowledgeSource struct {
	Metadata
	RemoteKnowledgeSourceManifest `json:",inline"`
	AgentID                       string                     `json:"agentID,omitempty"`
	WorkflowID                    string                     `json:"workflowID,omitempty"`
	ThreadID                      string                     `json:"threadID,omitempty"`
	RunID                         string                     `json:"runID,omitempty"`
	State                         RemoteKnowledgeSourceState `json:"state,omitempty"`
	Status                        string                     `json:"status,omitempty"`
	Error                         string                     `json:"error,omitempty"`
}

type RemoteKnowledgeSourceManifest struct {
	SyncSchedule               string `json:"syncSchedule,omitempty"`
	RemoteKnowledgeSourceInput `json:",inline"`
}

type RemoteKnowledgeSourceList List[RemoteKnowledgeSource]

type RemoteKnowledgeSourceInput struct {
	SourceType            RemoteKnowledgeSourceType `json:"sourceType,omitempty"`
	Exclude               []string                  `json:"exclude,omitempty"`
	OneDriveConfig        *OneDriveConfig           `json:"onedriveConfig,omitempty"`
	NotionConfig          *NotionConfig             `json:"notionConfig,omitempty"`
	WebsiteCrawlingConfig *WebsiteCrawlingConfig    `json:"websiteCrawlingConfig,omitempty"`
}

type OneDriveConfig struct {
	SharedLinks []string `json:"sharedLinks,omitempty"`
}

type NotionConfig struct {
	Pages []string `json:"pages,omitempty"`
}

type WebsiteCrawlingConfig struct {
	URLs []string `json:"urls,omitempty"`
}

type RemoteKnowledgeSourceState struct {
	OneDriveState        *OneDriveLinksConnectorState   `json:"onedriveState,omitempty"`
	NotionState          *NotionConnectorState          `json:"notionState,omitempty"`
	WebsiteCrawlingState *WebsiteCrawlingConnectorState `json:"websiteCrawlingState,omitempty"`
}

type OneDriveLinksConnectorState struct {
	Folders FolderSet `json:"folders,omitempty"`
}

type NotionConnectorState struct {
	Pages map[string]NotionPage `json:"pages,omitempty"`
}

type NotionPage struct {
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

type WebsiteCrawlingConnectorState struct {
	ScrapeJobIds map[string]string `json:"scrapeJobIds"`
	Folders      FolderSet         `json:"folders"`
}
