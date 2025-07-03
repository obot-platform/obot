package mcpcatalog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

var githubToken = os.Getenv("GITHUB_AUTH_TOKEN")

func isGitHubURL(catalogURL string) bool {
	u, err := url.Parse(catalogURL)
	return err == nil && u.Host == "github.com"
}

func readGitHubCatalog(catalogURL string) ([]types.MCPServerCatalogEntryManifest, error) {
	// Make sure we don't use plain HTTP
	if strings.HasPrefix(catalogURL, "http://") {
		return nil, fmt.Errorf("only HTTPS is supported for GitHub catalogs")
	}

	// Normalize the URL to ensure HTTPS
	if !strings.HasPrefix(catalogURL, "https://") {
		catalogURL = "https://" + catalogURL
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

	var (
		rawBaseURL            = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", org, repo, branch)
		catalogPatterns       = []string{"*.json"} // Default to all JSON files
		usingObotCatalogsFile bool
	)

	// First try to get .obotcatalogs file
	req, err := http.NewRequest(http.MethodGet, rawBaseURL+"/.obotcatalogs", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		usingObotCatalogsFile = true
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var patterns []string
		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				patterns = append(patterns, line)
			}
		}
		if scanner.Err() != nil && scanner.Err() != io.EOF {
			log.Warnf("Failed to read .obotcatalogs file: %v", scanner.Err())
		} else if len(patterns) > 0 {
			catalogPatterns = patterns
		}
	}

	// Get repository file listing using GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", org, repo, branch)
	req, err = http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list repository contents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list repository contents: %s %s", resp.Status, string(body))
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

	var entries []types.MCPServerCatalogEntryManifest
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
		req, err := http.NewRequest(http.MethodGet, rawBaseURL+"/"+item.Path, nil)
		if err != nil {
			log.Warnf("Failed to get contents of %s: %v", item.Path, err)
			continue
		}
		if githubToken != "" {
			req.Header.Set("Authorization", "Bearer "+githubToken)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Warnf("Failed to get contents of %s: %v", item.Path, err)
			continue
		}
		content, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Warnf("Failed to read contents of %s: %v", item.Path, err)
			continue
		} else if resp.StatusCode != http.StatusOK {
			log.Warnf("Failed to get contents of %s: (status: %s) (response body: %s)", item.Path, resp.Status, string(content))
			continue
		}

		// Try to unmarshal as array first
		var fileEntries []types.MCPServerCatalogEntryManifest
		if err := json.Unmarshal(content, &fileEntries); err != nil {
			// If that fails, try single object
			var entry types.MCPServerCatalogEntryManifest
			if err := json.Unmarshal(content, &entry); err != nil {
				if usingObotCatalogsFile {
					log.Warnf("Failed to parse %s as catalog entry: %v", item.Path, err)
				} else {
					log.Debugf("Failed to parse %s as catalog entry: %v", item.Path, err)
				}
				continue
			}
			fileEntries = []types.MCPServerCatalogEntryManifest{entry}
		}

		entries = append(entries, fileEntries...)
	}

	return entries, nil
}
