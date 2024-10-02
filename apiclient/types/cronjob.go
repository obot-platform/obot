package types

type CronJob struct {
	Metadata
	CronJobManifest
	LastRunStartedAt           *Time `json:"lastRunStartedAt,omitempty"`
	LastSuccessfulRunCompleted *Time `json:"lastSuccessfulRunCompleted,omitempty"`
	NextRunAt                  *Time `json:"nextRunAt,omitempty"`
}

type CronJobManifest struct {
	Description  string `json:"description,omitempty"`
	Schedule     string `json:"schedule,omitempty"`
	WorkflowName string `json:"workflowName,omitempty"`
	Input        string `json:"input,omitempty"`
}

type CronJobList List[CronJob]
