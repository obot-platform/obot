package types

type KnowledgeFile struct {
	Metadata
	FileName                  string                    `json:"fileName"`
	AgentID                   string                    `json:"agentID,omitempty"`
	WorkflowID                string                    `json:"workflowID,omitempty"`
	ThreadID                  string                    `json:"threadID,omitempty"`
	RemoteKnowledgeSourceID   string                    `json:"remoteKnowledgeSourceID,omitempty"`
	RemoteKnowledgeSourceType RemoteKnowledgeSourceType `json:"remoteKnowledgeSourceType,omitempty"`
	IngestionStatus           IngestionStatus           `json:"ingestionStatus,omitempty"`
	FileDetails               FileDetails               `json:"fileDetails,omitempty"`
	UploadID                  string                    `json:"uploadID,omitempty"`
}

type FileDetails struct {
	FilePath  string `json:"filePath,omitempty"`
	URL       string `json:"url,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
}

type IngestionStatus struct {
	Count        int    `json:"count,omitempty"`
	Reason       string `json:"reason,omitempty"`
	AbsolutePath string `json:"absolute_path,omitempty"`
	BasePath     string `json:"basePath,omitempty"`
	Filename     string `json:"filename,omitempty"`
	VectorStore  string `json:"vectorstore,omitempty"`
	Message      string `json:"msg,omitempty"`
	Flow         string `json:"flow,omitempty"`
	RootPath     string `json:"rootPath,omitempty"`
	Filepath     string `json:"filepath,omitempty"`
	Phase        string `json:"phase,omitempty"`
	NumDocuments int    `json:"num_documents,omitempty"`
	Stage        string `json:"stage,omitempty"`
	Status       string `json:"status,omitempty"`
	Component    string `json:"component,omitempty"`
	FileType     string `json:"filetype,omitempty"`
	Error        string `json:"error,omitempty"`
}

type KnowledgeFileList List[KnowledgeFile]
