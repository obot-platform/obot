package executionbudget

import (
	"io"
	"net/http"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
)

type AuditCapture struct {
	Headers           map[string]string `json:"headers"`
	Body              []byte            `json:"body,omitempty"`
	OriginalBodyBytes int64             `json:"originalBodyBytes"`
	Truncated         bool              `json:"truncated"`
}

type AuditLimits struct {
	HeaderLimit int64
	BodyLimit   int64
}

func CaptureBoundedAudit(header http.Header, body io.Reader, sensitiveNames []string, limit int64) (AuditCapture, error) {
	return CaptureBoundedAuditWithLimits(header, body, sensitiveNames, AuditLimits{
		HeaderLimit: DefaultHeaderLimit,
		BodyLimit:   limit,
	})
}

func CaptureBoundedAuditWithLimits(header http.Header, body io.Reader, sensitiveNames []string, limits AuditLimits) (AuditCapture, error) {
	sensitive := make(map[string]struct{}, len(sensitiveNames))
	for _, name := range sensitiveNames {
		sensitive[textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))] = struct{}{}
		sensitive[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	capture := AuditCapture{Headers: make(map[string]string, len(header))}
	names := make([]string, 0, len(header))
	for name := range header {
		names = append(names, name)
	}
	sort.Strings(names)
	known := map[string]struct{}{
		"Accept": {}, "Accept-Encoding": {}, "Content-Length": {}, "Content-Type": {},
		"Date": {}, "Host": {}, "User-Agent": {},
	}
	for _, name := range names {
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if _, blocked := sensitive[canonical]; blocked {
			capture.Headers[name] = "[REDACTED]"
			continue
		}
		if _, visible := known[canonical]; !visible {
			capture.Headers[name] = strconv.Itoa(len(header.Get(name))) + " bytes"
			continue
		}
		capture.Headers[name] = header.Get(name)
	}

	var serializedHeaders int64
	for name, value := range capture.Headers {
		serializedHeaders += int64(len(name)) + int64(len(value)) + 2
	}
	if serializedHeaders > limits.HeaderLimit {
		return AuditCapture{}, ErrLimitExceeded
	}

	if limits.BodyLimit == 0 {
		return capture, nil
	}
	reader := LimitReader(body, limits.BodyLimit+1)
	data, err := io.ReadAll(reader)
	if err != nil && len(data) <= int(limits.BodyLimit) {
		return capture, err
	}
	capture.OriginalBodyBytes = reader.BytesRead()
	if capture.OriginalBodyBytes > limits.BodyLimit {
		capture.Body = data[:limits.BodyLimit]
		capture.Truncated = true
	} else {
		capture.Body = data
	}
	return capture, nil
}

func (capture AuditCapture) ContainsSecret(secret string) bool {
	if secret == "" {
		return false
	}
	return strings.Contains(strings.Join(headerValues(capture.Headers), "\n"), secret) ||
		strings.Contains(string(capture.Body), secret)
}

func headerValues(headers map[string]string) []string {
	values := make([]string, 0, len(headers))
	for key, value := range headers {
		values = append(values, key, value)
	}
	return values
}
