package types

const (
	EnforcementDecisionAllow = "allow"
	EnforcementDecisionDeny  = "deny"
)

// EnforcementDecisionServer is the resolved target MCP server of a normalized tool call.
type EnforcementDecisionServer struct {
	URL      string                  `json:"url,omitempty"`
	Package  *AllowlistServerPackage `json:"package,omitempty"`
	Command  string                  `json:"command,omitempty"`
	Hostname string                  `json:"hostname,omitempty"`
}

// EnforcementDecisionRequest is the parameter-free normalized tool call a device
// submits to the decision endpoint. The fleet configuration is resolved from the
// authenticated device identity, never from this body.
type EnforcementDecisionRequest struct {
	Agent      string                    `json:"agent,omitempty"`
	Tool       string                    `json:"tool,omitempty"`
	Kind       string                    `json:"kind,omitempty"`
	ServerName string                    `json:"serverName,omitempty"`
	Server     EnforcementDecisionServer `json:"server,omitzero"`
}

type EnforcementDecisionResponse struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// EnforcementDecisionEvent is the public, read-side shape of a recorded decision.
// It is the decision log's own event type.
type EnforcementDecisionEvent struct {
	ID                 string                     `json:"id"`
	CreatedAt          Time                       `json:"createdAt"`
	MDMConfigurationID uint                       `json:"mdmConfigurationID"`
	DeviceID           string                     `json:"deviceID,omitempty"`
	ClientIP           string                     `json:"clientIP,omitempty"`
	Agent              string                     `json:"agent,omitempty"`
	Tool               string                     `json:"tool,omitempty"`
	Kind               string                     `json:"kind,omitempty"`
	ServerName         string                     `json:"serverName,omitempty"`
	ObotHosted         bool                       `json:"obotHosted,omitempty"`
	Decision           string                     `json:"decision"`
	Reason             string                     `json:"reason,omitempty"`
	Server             *EnforcementDecisionServer `json:"server,omitempty"`
}

type EnforcementDecisionEventList List[EnforcementDecisionEvent]

type EnforcementDecisionEventResponse struct {
	EnforcementDecisionEventList `json:",inline"`
	Total                        int64 `json:"total"`
	Limit                        int   `json:"limit"`
	Offset                       int   `json:"offset"`
}
