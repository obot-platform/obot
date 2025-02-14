package types

type CronJob struct {
	Metadata
	CronJobManifest
	LastRunStartedAt           *Time  `json:"lastRunStartedAt,omitempty"`
	LastSuccessfulRunCompleted *Time  `json:"lastSuccessfulRunCompleted,omitempty"`
	NextRunAt                  *Time  `json:"nextRunAt,omitempty"`
	Timezone                   string `json:"timezone,omitempty"`
}

type CronJobManifest struct {
	Description  string    `json:"description,omitempty"`
	Schedule     string    `json:"schedule,omitempty"`
	Workflow     string    `json:"workflow,omitempty"`
	Input        string    `json:"input,omitempty"`
	TaskSchedule *Schedule `json:"taskSchedule,omitempty"`

	// Timezone is the timezone to use for the cron job. If not set, the UTC timezone is used
	Timezone string `json:"timezone,omitempty"`
}

type CronJobList List[CronJob]
