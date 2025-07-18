package mcpcatalog

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/obot-platform/obot/apiclient/types"
	"sigs.k8s.io/yaml"
)

var githubToken = os.Getenv("GITHUB_AUTH_TOKEN")

func isGitHubURL(catalogURL string) bool {
	u, err := url.Parse(catalogURL)
	return err == nil && u.Host == "github.com"
}

// validateBranchName validates that the branch name doesn't contain suspicious characters
func validateBranchName(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Check for path traversal attempts and other suspicious characters
	if strings.Contains(branch, "..") || strings.Contains(branch, "/") ||
		strings.Contains(branch, "\\") || strings.Contains(branch, ":") ||
		strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name: %s", branch)
	}

	return nil
}

// isPathSafe checks if a file path is safe to read (not a symlink and within bounds)
func isPathSafe(path, baseDir string) error {
	// Check if it's a symbolic link
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Skip symbolic links to prevent path traversal
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic links are not allowed for security reasons")
	}

	// Resolve the absolute path and ensure it's within the base directory
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute base directory: %w", err)
	}

	// Ensure the file is within the base directory
	if !strings.HasPrefix(absPath, absBaseDir+string(filepath.Separator)) {
		return fmt.Errorf("file path is outside the allowed directory")
	}

	return nil
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

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URL format, expected github.com/org/repo")
	}
	org, repo := parts[0], parts[1]
	branch := "main"
	if len(parts) > 2 {
		branch = parts[2]
		// Validate branch name for security
		if err := validateBranchName(branch); err != nil {
			return nil, fmt.Errorf("invalid branch name: %w", err)
		}
	}

	// Create temporary directory for cloning
	tempDir, err := os.MkdirTemp("", "catalog-clone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the repository
	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", org, repo)

	// Set up clone options
	cloneOptions := &git.CloneOptions{
		URL:           cloneURL,
		Depth:         1,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
	}

	// Set up git credentials if token is available
	if githubToken != "" {
		cloneOptions.Auth = &http.BasicAuth{
			Username: "obot", // Use a dummy username
			Password: githubToken,
		}
	}

	// Use go-git to clone the repository
	_, err = git.PlainClone(tempDir, false, cloneOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	var (
		catalogPatterns       = []string{"*.json", "*.yaml", "*.yml"} // Default to all JSON and YAML files
		usingObotCatalogsFile bool
	)

	// First try to get .obotcatalogs file
	obotCatalogsPath := filepath.Join(tempDir, ".obotcatalogs")
	if content, err := os.ReadFile(obotCatalogsPath); err == nil {
		usingObotCatalogsFile = true
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
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

	// Walk through the cloned repository to find matching files
	var (
		entries   []types.MCPServerCatalogEntryManifest
		fileCount int
	)
	const maxFiles = 1000 // Limit the number of files processed to prevent resource exhaustion

	err = filepath.WalkDir(tempDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from repository root
		relPath, err := filepath.Rel(tempDir, path)
		if err != nil {
			return err
		}

		// Skip the .git directory specifically
		if d.IsDir() && (relPath == ".git" || strings.HasPrefix(relPath, ".git/")) {
			return filepath.SkipDir
		}

		// Skip directories (but continue walking into them)
		if d.IsDir() {
			return nil
		}

		// Check file count limit
		fileCount++
		if fileCount > maxFiles {
			return fmt.Errorf("too many files to process (limit: %d)", maxFiles)
		}

		// Check if file matches any pattern
		var matches bool
		for _, pattern := range catalogPatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
				matches = true
				break
			}
		}
		if !matches {
			return nil
		}

		// Security check: ensure the file is safe to read
		if err := isPathSafe(path, tempDir); err != nil {
			log.Warnf("Skipping unsafe file %s: %v", relPath, err)
			return nil
		}

		// Read file contents
		content, err := os.ReadFile(path)
		if err != nil {
			log.Warnf("Failed to read contents of %s: %v", relPath, err)
			return nil
		}

		// Try to unmarshal as array first
		var fileEntries []types.MCPServerCatalogEntryManifest
		if err := yaml.Unmarshal(content, &fileEntries); err != nil {
			// If that fails, try single object with YAML
			var entry types.MCPServerCatalogEntryManifest
			if err := yaml.Unmarshal(content, &entry); err != nil {
				if usingObotCatalogsFile {
					log.Warnf("Failed to parse %s as catalog entry: %v", relPath, err)
				} else {
					log.Debugf("Failed to parse %s as catalog entry: %v", relPath, err)
				}
				return nil
			}
			fileEntries = []types.MCPServerCatalogEntryManifest{entry}
		}

		entries = append(entries, fileEntries...)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk repository files: %w", err)
	}

	return entries, nil
}
