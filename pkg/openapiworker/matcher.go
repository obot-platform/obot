package openapiworker

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

var errInvalidPath = errors.New("invalid raw path")

type Match struct {
	Operation    Operation
	PathBindings map[string]string
}

func MatchOperation(operations []Operation, method, rawPath string) (Match, error) {
	requestSegments, err := StrictPathSegments(rawPath)
	if err != nil {
		return Match{}, NewError(ErrorCodeInvalidRequest, err.Error())
	}
	var matches []Match
	for _, operation := range operations {
		if !strings.EqualFold(operation.Method, method) {
			continue
		}
		templateSegments, templateErr := StrictPathSegments(operation.PathTemplate)
		if templateErr != nil || len(templateSegments) != len(requestSegments) {
			continue
		}
		bindings := make(map[string]string)
		matched := true
		for index, templateSegment := range templateSegments {
			if strings.HasPrefix(templateSegment.decoded, "{") && strings.HasSuffix(templateSegment.decoded, "}") {
				name := strings.TrimSpace(templateSegment.decoded[1 : len(templateSegment.decoded)-1])
				if name == "" {
					matched = false
					break
				}
				bindings[name] = requestSegments[index].escaped
				continue
			}
			if templateSegments[index].decoded != requestSegments[index].decoded {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, Match{Operation: operation, PathBindings: bindings})
		}
	}
	if len(matches) == 0 {
		return Match{}, NewError(ErrorCodeOperationNotFound, "no operation matches the method and path")
	}
	if len(matches) > 1 {
		return Match{}, NewError(ErrorCodeAmbiguousOperation, "multiple operations match the method and path")
	}
	return matches[0], nil
}

type pathSegment struct {
	escaped string
	decoded string
}

func StrictPathSegments(rawPath string) ([]pathSegment, error) {
	if rawPath == "" || !strings.HasPrefix(rawPath, "/") || strings.HasSuffix(rawPath, "/") {
		return nil, errInvalidPath
	}
	parts := strings.Split(strings.TrimPrefix(rawPath, "/"), "/")
	segments := make([]pathSegment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, errInvalidPath
		}
		decoded, err := url.PathUnescape(part)
		if err != nil || !utf8.ValidString(decoded) || strings.ContainsAny(decoded, "\x00\r\n\t") {
			return nil, errInvalidPath
		}
		segments = append(segments, pathSegment{escaped: normalizePercentEncoding(part), decoded: decoded})
	}
	return segments, nil
}

func normalizePercentEncoding(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '%' && index+2 < len(value) && isHex(value[index+1]) && isHex(value[index+2]) {
			builder.WriteByte('%')
			builder.WriteString(strings.ToUpper(value[index+1 : index+3]))
			index += 2
			continue
		}
		builder.WriteByte(value[index])
	}
	return builder.String()
}

func isHex(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f') || (value >= 'A' && value <= 'F')
}
