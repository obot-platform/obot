package types

import (
	"encoding/json"
)

var (
	KnowledgeSourceTypeOneDrive KnowledgeSourceType = "onedrive"
	KnowledgeSourceTypeNotion   KnowledgeSourceType = "notion"
	KnowledgeSourceTypeWebsite  KnowledgeSourceType = "website"
)

type KnowledgeSourceState string

func (k KnowledgeSourceState) IsTerminal() bool {
	return k == KnowledgeSourceStateSynced || k == KnowledgeSourceStateError
}

const (
	KnowledgeSourceStatePending KnowledgeSourceState = "pending"
	KnowledgeSourceStateSyncing KnowledgeSourceState = "syncing"
	KnowledgeSourceStateSynced  KnowledgeSourceState = "synced"
	KnowledgeSourceStateError   KnowledgeSourceState = "error"
)

type KnowledgeSourceType string

type KnowledgeSource struct {
	Metadata
	KnowledgeSourceManifest `json:",inline"`
	AgentID                 string               `json:"agentID,omitempty"`
	State                   KnowledgeSourceState `json:"state,omitempty"`
	SyncDetails             json.RawMessage      `json:"syncDetails,omitempty"`
	Status                  string               `json:"status,omitempty"`
	Error                   string               `json:"error,omitempty"`
	LastSyncStartTime       *Time                `json:"lastSyncStartTime,omitempty"`
	LastSyncEndTime         *Time                `json:"lastSyncEndTime,omitempty"`
	LastRunID               string               `json:"lastRunID,omitempty"`
}

type KnowledgeSourceManifest struct {
	SyncSchedule          string   `json:"syncSchedule,omitempty"`
	AutoApprove           *bool    `json:"autoApprove,omitempty"`
	FilePathPrefixInclude []string `json:"filePathPrefixInclude,omitempty"`
	FilePathPrefixExclude []string `json:"filePathPrefixExclude,omitempty"`
	KnowledgeSourceInput  `json:",inline"`
}

type KnowledgeSourceList List[KnowledgeSource]

type KnowledgeSourceInput struct {
	OneDriveConfig        *OneDriveConfig        `json:"onedriveConfig,omitempty"`
	NotionConfig          *NotionConfig          `json:"notionConfig,omitempty"`
	WebsiteCrawlingConfig *WebsiteCrawlingConfig `json:"websiteCrawlingConfig,omitempty"`
}

func (k *KnowledgeSourceInput) Validate() error {
	var setCount int
	if k.OneDriveConfig != nil {
		setCount++
	}
	if k.NotionConfig != nil {
		setCount++
	}
	if k.WebsiteCrawlingConfig != nil {
		setCount++
	}
	if setCount == 0 {
		return NewErrBadRequest("knowledge source input must have one of the following set: onedriveConfig, notionConfig, websiteCrawlingConfig")
	}
	if setCount > 1 {
		return NewErrBadRequest("knowledge source input can only have one of the following set: onedriveConfig, notionConfig, websiteCrawlingConfig")
	}
	return nil
}

func (k *KnowledgeSourceInput) GetCredential() string {
	if k.OneDriveConfig != nil {
		return "onedrive"
	}
	if k.NotionConfig != nil {
		return "notion"
	}
	return ""
}

func (k *KnowledgeSourceInput) GetType() KnowledgeSourceType {
	if k.OneDriveConfig != nil {
		return KnowledgeSourceTypeOneDrive
	}
	if k.NotionConfig != nil {
		return KnowledgeSourceTypeNotion
	}
	if k.WebsiteCrawlingConfig != nil {
		return KnowledgeSourceTypeWebsite
	}
	return ""
}

type OneDriveConfig struct {
	SharedLinks []string `json:"sharedLinks,omitempty"`
}

type NotionConfig struct{}

type WebsiteCrawlingConfig struct {
	URLs []string `json:"urls,omitempty"`
}
