package mcptester

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var (
	githubOwnerPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?$`)
	githubRepoPattern  = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,100}$`)
)

// sanitizePackageReference preserves only a recognized GitHub repository's
// owner/name. Other HTTP(S) hosts retain only their origin. Repository identity
// is metadata; credentials, revisions, subpaths, queries and fragments are not.
func sanitizePackageReference(value string) (string, error) {
	if strings.ContainsFunc(value, unicode.IsSpace) {
		return "", errDescriptor
	}

	if repository, ok := strings.CutPrefix(value, "github:"); ok {
		value = "https://github.com/" + repository
	}

	for _, prefix := range []string{"git+https://", "git+http://"} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimPrefix(value, "git+")
			break
		}
	}

	if !strings.Contains(value, "://") {
		if !packagePattern.MatchString(value) {
			return "", errDescriptor
		}

		return value, nil
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", errDescriptor
	}

	// SSH is recognized only for GitHub, whose repository identity has the same
	// meaning over HTTPS. Do not invent an HTTPS origin for arbitrary SSH hosts.
	if parsed.Scheme == "git+ssh" && strings.EqualFold(parsed.Host, "github.com") {
		parsed.Scheme = "https"
	}

	origin, err := sanitizeURL(parsed.String())
	if err != nil {
		return "", err
	}

	if !strings.EqualFold(parsed.Host, "github.com") {
		return origin, nil
	}

	if parsed.Path == "" || parsed.Path == "/" {
		return "https://github.com", nil
	}

	// Validate the escaped components so encoded separators or credentials cannot
	// be decoded into a plausible repository name.
	parts := strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 2 {
		return "", errDescriptor
	}

	owner := parts[0]
	repo, _, _ := strings.Cut(parts[1], "@")
	repo = strings.TrimSuffix(repo, ".git")
	if !githubOwnerPattern.MatchString(owner) || strings.Contains(owner, "--") ||
		!githubRepoPattern.MatchString(repo) || repo == "." || repo == ".." {
		return "", errDescriptor
	}

	return "https://github.com/" + owner + "/" + repo, nil
}
