package validation

import "strings"

func SourceIDForURL(sourceURL string) string {
	sourceURL = strings.TrimPrefix(sourceURL, "https://")
	sourceURL = strings.TrimPrefix(sourceURL, "http://")
	if len(sourceURL) > 1 {
		sourceURL = strings.TrimRight(sourceURL, "/")
	}
	return sourceURL
}
