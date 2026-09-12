package client

import (
	"encoding/json"
	"unicode/utf8"
)

func limitAuditBody(body json.RawMessage, limit *int) json.RawMessage {
	if limit == nil || len(body) == 0 {
		return body
	}

	if *limit <= 0 {
		return nil
	}

	if len(body) <= *limit {
		return body
	}

	end := *limit
	for end > 0 && !utf8.RuneStart(body[end]) {
		end--
	}

	// Marshaling these primitive fields cannot fail. The preview is text, since
	// a prefix of a JSON document is generally not itself valid JSON.
	preview, _ := json.Marshal(struct {
		Truncated     bool   `json:"_obotAuditTruncated"`
		OriginalBytes int    `json:"originalBytes"`
		Preview       string `json:"preview"`
	}{
		Truncated:     true,
		OriginalBytes: len(body),
		Preview:       string(body[:end]),
	})

	return preview
}
