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

// JSONCoerce converts JSON-compatible input into the requested type.
func JSONCoerce[T any](in any, out *T) error {
	switch s := any(out).(type) {
	case *string:
		if inStr, ok := in.(string); ok {
			*s = inStr
			return nil
		}
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		*s = string(data)
		return nil
	}

	if v, ok := in.(T); ok {
		*out = v
		return nil
	}

	var data []byte
	if inBytes, ok := in.([]byte); ok {
		data = inBytes
	} else if inStr, ok := in.(string); ok {
		data = []byte(inStr)
	} else if inStrP, ok := in.(*string); ok && inStrP != nil {
		data = []byte(*inStrP)
	} else {
		var err error
		data, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(data, out)
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
