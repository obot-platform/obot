package toolreference

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

func isGitHubURL(catalogURL string) bool {
	u, err := url.Parse(catalogURL)
	return err == nil && u.Host == "github.com"
}

func readGitHubCatalog(catalogURL string) ([]catalogEntryInfo, error) {
	// Normalize the URL to ensure HTTPS
	if !strings.HasPrefix(catalogURL, "http://") && !strings.HasPrefix(catalogURL, "https://") {
		catalogURL = "https://" + catalogURL
	}

	// Validate protocol is HTTPS
	if strings.HasPrefix(catalogURL, "http://") {
		return nil, fmt.Errorf("only HTTPS is supported for GitHub catalogs")
	}

	// Parse URL to ensure it's valid
	u, err := url.Parse(catalogURL)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub URL: %w", err)
	}

	// Should not be possible, but check anyway.
	if u.Host != "github.com" {
		return nil, fmt.Errorf("not a GitHub URL: %s", catalogURL)
	}

	// Convert github.com URL to raw.githubusercontent.com
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URL format, expected github.com/org/repo")
	}
	org, repo := parts[0], parts[1]
	branch := "main"
	if len(parts) > 2 {
		branch = parts[2]
	}

	// First try to get .obotcatalogs file
	rawBaseURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", org, repo, branch)
	catalogPatterns := []string{"*.json"} // Default to all JSON files

	var usingObotCatalogsFile bool
	resp, err := http.Get(rawBaseURL + "/.obotcatalogs")
	if err == nil && resp.StatusCode == http.StatusOK {
		usingObotCatalogsFile = true
		defer resp.Body.Close()
		content, err := io.ReadAll(resp.Body)
		if err == nil {
			// Split content by newlines and filter empty lines
			var patterns []string
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					patterns = append(patterns, line)
				}
			}
			if len(patterns) > 0 {
				catalogPatterns = patterns
			}
		}
	}

	// Get repository file listing using GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", org, repo, branch)
	resp, err = http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to list repository contents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repository contents: %s", resp.Status)
	}

	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("failed to decode repository listing: %w", err)
	}

	var entries []catalogEntryInfo
	for _, item := range tree.Tree {
		if item.Type != "blob" {
			continue
		}

		// Check if file matches any pattern
		var matches bool
		for _, pattern := range catalogPatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(item.Path)); matched {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}

		// Get file contents
		resp, err := http.Get(rawBaseURL + "/" + item.Path)
		if err != nil {
			log.Warnf("Failed to get contents of %s: %v", item.Path, err)
			continue
		}

		content, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Warnf("Failed to read contents of %s: %v", item.Path, err)
			continue
		}

		// Try to unmarshal as array first
		var fileEntries []catalogEntryInfo
		if err := json.Unmarshal(content, &fileEntries); err != nil {
			// If that fails, try single object
			var entry catalogEntryInfo
			if err := json.Unmarshal(content, &entry); err != nil {
				if usingObotCatalogsFile {
					log.Warnf("Failed to parse %s as catalog entry: %v", item.Path, err)
				} else {
					log.Debugf("Failed to parse %s as catalog entry: %v", item.Path, err)
				}
				continue
			}
			fileEntries = []catalogEntryInfo{entry}
		}

		entries = append(entries, fileEntries...)
	}

	return entries, nil
}
