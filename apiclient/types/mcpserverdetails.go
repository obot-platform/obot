package types

type MCPServerEvent struct {
	Time      Time   `json:"time"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	EventType string `json:"eventType"`
	Action    string `json:"action"`
	Count     int32  `json:"count"`
}

type MCPServerDetails struct {
	DeploymentName string           `json:"deploymentName"`
	Namespace      string           `json:"namespace"`
	LastRestart    Time             `json:"lastRestart"`
	ReadyReplicas  int32            `json:"readyReplicas"`
	Replicas       int32            `json:"replicas"`
	IsAvailable    bool             `json:"isAvailable"`
	Events         []MCPServerEvent `json:"events"`
}
