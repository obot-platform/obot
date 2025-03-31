package types

type Task struct {
	Metadata
	TaskManifest
	ProjectID string `json:"projectID,omitempty"`
	Alias     string `json:"alias,omitempty"`
	Managed   bool   `json:"managed"`
}

type TaskList List[Task]

type TaskManifest struct {
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	Steps          []TaskStep          `json:"steps"`
	Schedule       *Schedule           `json:"schedule"`
	Webhook        *TaskWebhook        `json:"webhook"`
	Email          *TaskEmail          `json:"email"`
	OnDemand       *TaskOnDemand       `json:"onDemand"`
	OnSlackMessage *TaskOnSlackMessage `json:"onSlackMessage"`
}

type TaskOnSlackMessage struct {
}

type TaskOnDemand struct {
	Params map[string]string `json:"params,omitempty"`
}

type TaskWebhook struct {
}

type TaskEmail struct {
}

type Schedule struct {
	// Valid values are: "hourly", "daily", "weekly", "monthly"
	Interval string `json:"interval"`
	Hour     int    `json:"hour"`
	Minute   int    `json:"minute"`
	Day      int    `json:"day"`
	Weekday  int    `json:"weekday"`
	TimeZone string `json:"timezone"`
}

type TaskStep struct {
	ID   string `json:"id,omitempty"`
	Step string `json:"step,omitempty"`
}

type TaskRun struct {
	Metadata
	TaskID    string       `json:"taskID,omitempty"`
	ThreadID  string       `json:"threadID,omitempty"`
	Input     string       `json:"input,omitempty"`
	Output    string       `json:"output,omitempty"`
	Task      TaskManifest `json:"task,omitempty"`
	StartTime *Time        `json:"startTime,omitempty"`
	EndTime   *Time        `json:"endTime,omitempty"`
	Error     string       `json:"error,omitempty"`
}

type TaskRunList List[TaskRun]
