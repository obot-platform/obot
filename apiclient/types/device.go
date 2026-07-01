package types

// Device is an enrolled machine belonging to a device deployment. It is the API
// representation of a gateway device record (the registered public key is not
// exposed).
type Device struct {
	ID                 uint   `json:"id"`
	DeviceID           string `json:"deviceID"`
	DeviceDeploymentID uint   `json:"deviceDeploymentID"`
	Hostname           string `json:"hostname,omitempty"`
	OS                 string `json:"os,omitempty"`
	OSVersion          string `json:"osVersion,omitempty"`
	EnrolledAt         Time   `json:"enrolledAt"`
	LastSeenAt         *Time  `json:"lastSeenAt,omitempty"`
}

type DeviceList List[Device]

// DeviceEnrollRequest is the body of POST /api/devices/enroll. PublicKey is the
// device's identity key as base64-encoded DER (PKIX / SubjectPublicKeyInfo);
// the device proves possession of it by signing the access JWTs it presents
// when submitting scans.
type DeviceEnrollRequest struct {
	DeviceID  string `json:"deviceID"`
	PublicKey string `json:"publicKey"`
	Hostname  string `json:"hostname,omitempty"`
	OS        string `json:"os,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
}

// DeviceEnrollResponse is returned by POST /api/devices/enroll.
type DeviceEnrollResponse struct {
	Device Device `json:"device"`
}
