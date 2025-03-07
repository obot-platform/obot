package types

type Task struct {
	Metadata
	TaskManifest
	ThreadID      string `json:"threadID,omitempty"`
	Alias         string `json:"alias,omitempty"`
	ProjectScoped bool   `json:"projectScoped,omitempty"`
}

type TaskList List[Task]

type TaskManifest struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Steps       []TaskStep    `json:"steps"`
	Schedule    *Schedule     `json:"schedule"`
	Webhook     *TaskWebhook  `json:"webhook"`
	Email       *TaskEmail    `json:"email"`
	OnDemand    *TaskOnDemand `json:"onDemand"`
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
}

type TaskStep struct {
	ID   string `json:"id,omitempty"`
	Step string `json:"step,omitempty"`
}

type TaskRun struct {
	Metadata
	TaskID    string       `json:"taskID,omitempty"`
	Input     string       `json:"input,omitempty"`
	Task      TaskManifest `json:"task,omitempty"`
	StartTime *Time        `json:"startTime,omitempty"`
	EndTime   *Time        `json:"endTime,omitempty"`
	Error     string       `json:"error,omitempty"`
}

type TaskRunList List[TaskRun]
