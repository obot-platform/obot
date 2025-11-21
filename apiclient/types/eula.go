package types

// EulaStatus represents the user's EULA acceptance status
type EulaStatus struct {
	// Accepted indicates whether the user has accepted the EULA
	Accepted bool `json:"accepted"`
}
