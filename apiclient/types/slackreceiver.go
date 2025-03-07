package types

type SlackReceiver struct {
	Metadata
	SlackReceiverManifest

	ClientSecret  string `json:"clientSecret,omitempty"`
	SigningSecret string `json:"signingSecret,omitempty"`
}

// SlackReceiverManifest defines the configuration for a Slack receiver
type SlackReceiverManifest struct {
	// AppID corresponds to the App ID of the Slack app. It's important to not that
	// this value is not validated. The user can insert whatever they want here. Don't
	// trust this value. Use the value from oauth flow to validate the app.
	AppID    string `json:"appId,omitempty"`
	ClientID string `json:"clientId,omitempty"`
}

type SlackReceiverList List[SlackReceiver]
