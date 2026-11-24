package llmengine

import (
	"bytes"

	"github.com/maximhq/bifrost/core/schemas"
)

// IsSecretReference reports whether the engine would read a JSON value from the host's
// environment or a vault, rather than take it as given, wherever it accepts a secret. It
// decodes the value with the engine's own decoder, so it can't disagree with the engine on
// spelling, such as key names in another case.
func IsSecretReference(raw []byte) bool {
	var v schemas.SecretVar
	if err := v.UnmarshalJSON(raw); err != nil {
		return false
	}
	return v.IsFromSecret()
}

// SecretValue returns the value the engine reads from a JSON value it accepts as a secret,
// such as a region, which may be a string or an object holding one.
func SecretValue(raw []byte) string {
	var v schemas.SecretVar
	if err := v.UnmarshalJSON(raw); err != nil {
		return ""
	}
	return v.GetValue()
}

// HasSecretValue reports whether a JSON value the engine accepts as an optional secret, such as
// a CA certificate, holds a value. JSON null leaves the setting unset.
func HasSecretValue(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && SecretValue(trimmed) != ""
}

// IsMaskedSecret reports whether a JSON value is a secret as the engine lists it, redacted,
// which the engine takes to mean "keep the stored value" when it is sent back in an update.
func IsMaskedSecret(raw []byte) bool {
	var v schemas.SecretVar
	if err := v.UnmarshalJSON(raw); err != nil {
		return false
	}
	return v.IsMaskedPlaceholder()
}
