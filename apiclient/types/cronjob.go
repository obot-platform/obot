package types

type CronJob struct {
	Metadata
	CronJobManifest
	LastRunStartedAt           *Time `json:"lastRunStartedAt,omitempty"`
	LastSuccessfulRunCompleted *Time `json:"lastSuccessfulRunCompleted,omitempty"`
	NextRunAt                  *Time `json:"nextRunAt,omitempty"`
}

type CronJobManifest struct {
	Description string `json:"description,omitempty"`
	Schedule    string `json:"schedule,omitempty"`
	WorkflowID  string `json:"workflowID,omitempty"`
	Input       string `json:"input,omitempty"`
}

type CronJobList List[CronJob]
