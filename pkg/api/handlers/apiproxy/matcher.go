package apiproxy

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

type Operation struct {
	OperationKey   string
	LegacyToolName string
	Method         string
	PathTemplate   string
}

type MatchResult struct {
	Operation Operation
	Bindings  map[string]string
}

func MatchOperations(operations []Operation, method, rawPath string) (MatchResult, string) {
	requestSegments, ok := parseSegments(rawPath)
	if !ok {
		return MatchResult{}, "invalid_path"
	}
	var matches []MatchResult
	for _, operation := range operations {
		if !strings.EqualFold(operation.Method, method) {
			continue
		}
		templateSegments, templateOK := parseSegments(operation.PathTemplate)
		if !templateOK || len(templateSegments) != len(requestSegments) {
			continue
		}
		bindings := make(map[string]string)
		matched := true
		for index, templateSegment := range templateSegments {
			name, isParameter := pathParameterName(templateSegment.decoded)
			if isParameter {
				if name == "" {
					matched = false
					break
				}
				bindings[name] = requestSegments[index].escaped
				continue
			}
			if templateSegment.decoded != requestSegments[index].decoded {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, MatchResult{Operation: operation, Bindings: bindings})
		}
	}
	if len(matches) == 0 {
		return MatchResult{}, "no_match"
	}
	selected := matches[0]
	ambiguous := false
	for _, candidate := range matches[1:] {
		comparison := compareSpecificity(candidate.Operation.PathTemplate, selected.Operation.PathTemplate, requestSegments)
		switch comparison {
		case 1:
			selected = candidate
			ambiguous = false
		case 0:
			ambiguous = true
		}
	}
	if ambiguous {
		return MatchResult{}, "ambiguous_operation"
	}
	return selected, ""
}

func compareSpecificity(leftTemplate, rightTemplate string, segments []pathSegment) int {
	left, leftOK := parseSegments(leftTemplate)
	right, rightOK := parseSegments(rightTemplate)
	if !leftOK || !rightOK || len(left) != len(segments) || len(right) != len(segments) {
		return -1
	}
	for index := range left {
		_, leftParameter := pathParameterName(left[index].decoded)
		_, rightParameter := pathParameterName(right[index].decoded)
		if leftParameter == rightParameter {
			continue
		}
		if rightParameter {
			return 1
		}
		return -1
	}
	return 0
}

func MethodExistsForPath(operations []Operation, method, rawPath string) bool {
	requestSegments, ok := parseSegments(rawPath)
	if !ok {
		return false
	}
	for _, operation := range operations {
		if strings.EqualFold(operation.Method, method) || !validTemplateForSegments(operation.PathTemplate, requestSegments) {
			continue
		}
		if templateMatches(operation.PathTemplate, requestSegments) {
			return true
		}
	}
	return false
}

func validTemplateForSegments(template string, requestSegments []pathSegment) bool {
	templateSegments, ok := parseSegments(template)
	return ok && len(templateSegments) == len(requestSegments)
}

func templateMatches(template string, requestSegments []pathSegment) bool {
	templateSegments, ok := parseSegments(template)
	if !ok {
		return false
	}
	for index, templateSegment := range templateSegments {
		if _, isParameter := pathParameterName(templateSegment.decoded); isParameter {
			continue
		}
		if templateSegment.decoded != requestSegments[index].decoded {
			return false
		}
	}
	return true
}

type pathSegment struct {
	escaped string
	decoded string
}

func parseSegments(rawPath string) ([]pathSegment, bool) {
	if rawPath == "" || !strings.HasPrefix(rawPath, "/") || strings.HasSuffix(rawPath, "/") ||
		strings.Contains(rawPath, "//") {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(rawPath, "/"), "/")
	segments := make([]pathSegment, 0, len(parts))
	for _, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil || !utf8.ValidString(decoded) || strings.ContainsAny(decoded, "\x00\r\n\t") ||
			decoded == "." || decoded == ".." {
			return nil, false
		}
		segments = append(segments, pathSegment{escaped: normalizeEscapes(part), decoded: decoded})
	}
	return segments, true
}

func pathParameterName(segment string) (string, bool) {
	if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
		return "", false
	}
	return strings.TrimSpace(segment[1 : len(segment)-1]), true
}

func normalizeEscapes(value string) string {
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
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}
