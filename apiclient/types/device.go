package types

// Device is an enrolled machine belonging to a MDM configuration. It is the API
// representation of a gateway device record (the registered public key is not
// exposed).
type Device struct {
	ID                 uint   `json:"id"`
	DeviceID           string `json:"deviceID"`
	MDMConfigurationID uint   `json:"mdmConfigurationID"`
	Hostname           string `json:"hostname,omitempty"`
	OS                 string `json:"os,omitempty"`
	OSVersion          string `json:"osVersion,omitempty"`
	EnrolledAt         Time   `json:"enrolledAt"`
	LastSeenAt         *Time  `json:"lastSeenAt,omitempty"`
}

type DeviceList List[Device]

// DeviceEnrollRequest is the body of POST /api/mdm/enroll. PublicKey is the
// device's identity key as DER (PKIX / SubjectPublicKeyInfo), carried as
// base64 in JSON; the device proves possession of it by signing the access
// JWTs it presents when submitting scans.
type DeviceEnrollRequest struct {
	DeviceID  string `json:"deviceID"`
	PublicKey []byte `json:"publicKey"`
	Hostname  string `json:"hostname,omitempty"`
	OS        string `json:"os,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
}

// DeviceEnrollResponse is returned by POST /api/mdm/enroll.
type DeviceEnrollResponse struct {
	Device Device `json:"device"`
}
