// Package eval provides an evaluation framework for the nanobot workflow feature.
// Evals are API-driven (no UI automation), realistic, and can run against a live Obot instance.
package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Result represents the outcome of a single eval case.
type Result struct {
	Name      string        `json:"name"`
	Pass      bool          `json:"pass"`
	Duration  time.Duration `json:"duration_ms"`
	Message   string        `json:"message,omitempty"`
	Trajectory []string     `json:"trajectory,omitempty"` // optional step log for debugging
}

// Case is a single evaluation scenario.
type Case struct {
	Name        string
	Description string
	Run         func(ctx *Context) Result
}

// Context provides shared state and client for eval cases.
type Context struct {
	BaseURL string
	Client  *Client
	Trajectory []string
}

// AppendStep records a step in the trajectory (for debugging and trajectory-quality evals).
func (c *Context) AppendStep(format string, args ...any) {
	c.Trajectory = append(c.Trajectory, fmt.Sprintf(format, args...))
}

// RunAll runs all given cases and returns results. Skips cases if BaseURL or auth is missing.
func RunAll(cases []Case, baseURL, authHeader string) ([]Result, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("OBOT_EVAL_BASE_URL is required")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	client, err := NewClient(baseURL, authHeader)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(cases))
	for _, c := range cases {
		ctx := &Context{BaseURL: baseURL, Client: client}
		start := time.Now()
		result := c.Run(ctx)
		result.Name = c.Name
		result.Duration = time.Since(start)
		result.Trajectory = ctx.Trajectory
		results = append(results, result)
	}
	return results, nil
}

// RunFromEnv runs all given cases using OBOT_EVAL_BASE_URL and OBOT_EVAL_AUTH_HEADER.
// If OBOT_EVAL_BASE_URL is not set, returns (nil, nil) so tests can skip.
func RunFromEnv(cases []Case) ([]Result, error) {
	baseURL := os.Getenv("OBOT_EVAL_BASE_URL")
	authHeader := os.Getenv("OBOT_EVAL_AUTH_HEADER")
	if baseURL == "" {
		return nil, nil
	}
	return RunAll(cases, baseURL, authHeader)
}

// WriteResultsJSON writes results to w as JSON.
func WriteResultsJSON(results []Result, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// PassCount returns the number of passing results.
func PassCount(results []Result) int {
	n := 0
	for _, r := range results {
		if r.Pass {
			n++
		}
	}
	return n
}
