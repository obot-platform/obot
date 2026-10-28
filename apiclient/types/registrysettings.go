package types

// RegistrySettings controls anonymous discovery; authenticated discovery remains available.
type RegistrySettings struct {
	AnonymousAccessEnabled bool `json:"anonymousAccessEnabled"`
}
