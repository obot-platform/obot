package validation

import "strings"

func SourceIDForURL(sourceURL string) string {
	sourceURL = strings.TrimPrefix(sourceURL, "https://")
	sourceURL = strings.TrimPrefix(sourceURL, "http://")
	sourceURL = strings.TrimRight(sourceURL, "/")
	return sourceURL
}
