package logutil

import "net/url"

// SanitizeURL masks credentials in a URL for safe display/logging.
// Returns the URL with password replaced by [REDACTED].
// Example: https://user:pass@gitlab.com/repo.git -> https://user:[REDACTED]@gitlab.com/repo.git
func SanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	if u.User == nil {
		return rawURL
	}

	_, hasPassword := u.User.Password()
	if !hasPassword {
		return rawURL
	}

	result := u.Scheme + "://" + u.User.Username() + ":[REDACTED]@" + u.Host + u.RequestURI()
	return result
}
