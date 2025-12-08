//nolint:revive
package hash

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

func String(obj any) string {
	switch v := obj.(type) {
	case []byte:
		return fmt.Sprintf("%x", sha256.Sum256(v))
	case string:
		return fmt.Sprintf("%x", sha256.Sum256([]byte(v)))
	default:
		data, _ := json.Marshal(obj)
		return String(data)
	}
}
