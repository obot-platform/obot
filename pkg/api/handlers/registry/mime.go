package registry

import (
	"net/url"
	"path"
	"strings"
)

// guessMimeType uses the URL path only; registry discovery never fetches icons.
func guessMimeType(iconURL string) string {
	u, err := url.Parse(iconURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}
