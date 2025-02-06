package types

type Task struct {
	Metadata
	TaskManifest
	Alias string `json:"alias,omitempty"`
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
	Timezone string `json:"timezone"`
}

type TaskStep struct {
	ID   string  `json:"id,omitempty"`
	If   *TaskIf `json:"if,omitempty"`
	Step string  `json:"step,omitempty"`
}

type TaskIf struct {
	Condition string     `json:"condition,omitempty"`
	Steps     []TaskStep `json:"steps,omitempty"`
	Else      []TaskStep `json:"else,omitempty"`
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
