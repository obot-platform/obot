package devicescan

import (
	"os"
	"path/filepath"
	"strings"
)

// readGitOrigin returns the `url` value of the `[remote "origin"]`
// section in dir/.git/config, or "" if the file is missing,
// malformed, or the section/url is absent.
//
// Worktree-style .git files (which contain `gitdir: <path>` rather
// than a directory) are not followed; v1 covers the common case of an
// in-repo .git directory.
func readGitOrigin(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}

	var inOrigin bool
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inOrigin = strings.HasPrefix(trimmed, `[remote "origin"]`)
			continue
		}
		if !inOrigin {
			continue
		}
		// Match `url = <value>` or `url=<value>`. Comment lines starting
		// with '#' or ';' inside the section are skipped because they
		// don't begin with "url".
		if !strings.HasPrefix(trimmed, "url") {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("url"):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		return strings.TrimSpace(rest[1:])
	}
	return ""
}
