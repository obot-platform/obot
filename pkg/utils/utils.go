//nolint:revive
package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Digest returns a SHA-256 hash of the input object. If the object is a string or byte slice, it hashes the raw data.
// For other types, it encodes the object as JSON before hashing.
func Digest(obj any) string {
	hash := sha256.New()
	switch v := obj.(type) {
	case []byte:
		hash.Write(v)
	case string:
		hash.Write([]byte(v))
	default:
		if err := json.NewEncoder(hash).Encode(obj); err != nil {
			panic(err)
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// FirstSet returns the first non-zero value from the input slice, or the zero value if all are zero.
func FirstSet[T comparable](in ...T) T {
	var zero T
	for _, i := range in {
		if i != zero {
			return i
		}
	}
	return zero
}
