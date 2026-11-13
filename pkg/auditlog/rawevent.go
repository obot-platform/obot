package auditlog

import (
	"bytes"
	"encoding/json"
)

// Obot Sentry leaves the tool input and output out of a local-agent hook's raw event, because the
// same values are submitted as the request and response bodies. In their place it writes a marker
// naming the field that holds the value, for example
// "tool_response": {"obot_sentry_omitted": "details.response.body"}. A marker can also say the
// original was a JSON string holding that value as compact JSON ("encoding": "json_string").
const (
	rawOmittedKey         = "obot_sentry_omitted"
	rawOmittedEncodingKey = "encoding"
	rawOmittedJSONString  = "json_string"
	rawOmittedRequest     = "details.request.body"
	rawOmittedResponse    = "details.response.body"
)

var (
	// rawOmittedKeyBytes is the marker key as it appears in a payload. It holds nothing JSON
	// escapes, so a payload without these bytes holds no marker and needs no decoding.
	rawOmittedKeyBytes = []byte(rawOmittedKey)

	// goEscapes are the escapes encoding/json writes in strings where the agent wrote the character
	// itself: it escapes <, > and & for HTML, and U+2028 and U+2029 for JavaScript. The agents that
	// send a JSON-string output (Cursor) build it with JavaScript's JSON.stringify, which escapes none
	// of them.
	goEscapes = map[string]string{
		`\u003c`: "<",
		`\u003e`: ">",
		`\u0026`: "&",
		`\u2028`: "\u2028",
		`\u2029`: "\u2029",
	}
)

// restoreRawEvent returns the raw event with each top-level Obot Sentry marker replaced by the
// body it names, so the presented raw event is the hook payload the agent sent while storage keeps
// each value once. Only the marker values are rewritten; every other byte of the raw event is kept
// as stored. A marker whose body is empty (not recorded, or blanked because the caller may not see
// payloads) or is not valid JSON is left in place. A raw event that is not a JSON object is
// returned unchanged.
func restoreRawEvent(raw, request, response json.RawMessage) json.RawMessage {
	type replacement struct {
		start, end int
		value      []byte
	}

	if !bytes.Contains(raw, rawOmittedKeyBytes) {
		return raw
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return raw
	}

	var replacements []replacement
	for dec.More() {
		if _, err := dec.Token(); err != nil {
			return raw
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return raw
		}
		end := int(dec.InputOffset())
		start := end - len(value)

		field, jsonString, ok := parseOmittedMarker(value)
		if !ok {
			continue
		}
		body := request
		if field == rawOmittedResponse {
			body = response
		}
		restored, ok := restoredValue(body, jsonString)
		if !ok {
			continue
		}
		replacements = append(replacements, replacement{start: start, end: end, value: restored})
	}
	if len(replacements) == 0 {
		return raw
	}

	out := make([]byte, 0, len(raw))
	last := 0
	for _, r := range replacements {
		out = append(out, raw[last:r.start]...)
		out = append(out, r.value...)
		last = r.end
	}
	return append(out, raw[last:]...)
}

// parseOmittedMarker reports whether value is exactly an Obot Sentry marker: an object holding
// the marker key naming the request or response body, and at most the json_string encoding.
func parseOmittedMarker(value json.RawMessage) (field string, jsonString, ok bool) {
	if len(value) == 0 || value[0] != '{' || !bytes.Contains(value, rawOmittedKeyBytes) {
		return "", false, false
	}
	var marker map[string]json.RawMessage
	if err := json.Unmarshal(value, &marker); err != nil {
		return "", false, false
	}
	if err := json.Unmarshal(marker[rawOmittedKey], &field); err != nil {
		return "", false, false
	}
	if field != rawOmittedRequest && field != rawOmittedResponse {
		return "", false, false
	}
	switch len(marker) {
	case 1:
		return field, false, true
	case 2:
		var encoding string
		if err := json.Unmarshal(marker[rawOmittedEncodingKey], &encoding); err != nil || encoding != rawOmittedJSONString {
			return "", false, false
		}
		return field, true, true
	default:
		return "", false, false
	}
}

// restoredValue returns the JSON that stands where the marker was. For a json_string marker that
// is a JSON string holding the body as compact JSON, which is the string the agent sent: Obot
// Sentry only replaces a string whose content is already compact JSON, and it submits the body
// with encoding/json, which escapes the characters undoGoEscapes puts back.
func restoredValue(body json.RawMessage, jsonString bool) ([]byte, bool) {
	if len(bytes.TrimSpace(body)) == 0 || !json.Valid(body) {
		return nil, false
	}
	if !jsonString {
		return bytes.TrimSpace(body), true
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		return nil, false
	}
	// SetEscapeHTML(false) keeps the encoder from escaping <, > and & again in the string it
	// builds. encoding/json always writes U+2028 and U+2029 escaped, which is the same string.
	var encoded bytes.Buffer
	enc := json.NewEncoder(&encoded)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(string(undoGoEscapes(compact.Bytes()))); err != nil {
		return nil, false
	}
	return bytes.TrimSuffix(encoded.Bytes(), []byte("\n")), true
}

// undoGoEscapes returns compact JSON with each of goEscapes inside a string replaced by the
// character it stands for. Other escapes are copied as they are, so an escaped backslash followed
// by "u0026" stays literal text. The result is the same JSON value.
func undoGoEscapes(compact []byte) []byte {
	if !bytes.Contains(compact, []byte(`\u`)) {
		return compact
	}

	out := make([]byte, 0, len(compact))
	inString := false
	for i := 0; i < len(compact); i++ {
		c := compact[i]
		switch {
		case !inString:
			inString = c == '"'
		case c == '"':
			inString = false
		case c == '\\' && i+1 < len(compact):
			if i+6 <= len(compact) {
				if char, ok := goEscapes[string(compact[i:i+6])]; ok {
					out = append(out, char...)
					i += 5
					continue
				}
			}
			out = append(out, c, compact[i+1])
			i++
			continue
		}
		out = append(out, c)
	}
	return out
}
