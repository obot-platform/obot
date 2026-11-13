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
	if len(value) == 0 || value[0] != '{' {
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
// Sentry only replaces a string whose content is already compact JSON.
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
	var encoded bytes.Buffer
	enc := json.NewEncoder(&encoded)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(compact.String()); err != nil {
		return nil, false
	}
	return bytes.TrimSuffix(encoded.Bytes(), []byte("\n")), true
}
