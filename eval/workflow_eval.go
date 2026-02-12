package eval

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// WorkflowEvalResult holds structured results from evaluating a content-publishing workflow response.
type WorkflowEvalResult struct {
	Pass           bool
	HasURL         bool
	HasTitle       bool
	SourcesUsed    *int   // nil if not found
	ToolCallsMade  *int   // nil if not found
	Message        string
	PublishedURL   string
	Title          string
}

// EvaluateContentPublishingResponse evaluates raw response text from nanobot
// after running the content publishing workflow prompt. It checks for the expected
// output format: published post URL, title, number of sources used, number of tool calls made.
func EvaluateContentPublishingResponse(responseText string) WorkflowEvalResult {
	text := strings.TrimSpace(responseText)
	if text == "" {
		return WorkflowEvalResult{Pass: false, Message: "empty response"}
	}

	out := WorkflowEvalResult{}

	// URL: must contain something that looks like http(s) URL
	urlRx := regexp.MustCompile(`https?://[^\s\)\]\"']+`)
	if u := urlRx.FindString(text); u != "" {
		out.HasURL = true
		out.PublishedURL = u
	}

	// Title: look for "title" or "Title:" or first meaningful line / heading
	titleRx := regexp.MustCompile(`(?i)(?:title\s*[:\-]\s*)(.+?)(?:\n|$)`)
	if m := titleRx.FindStringSubmatch(text); len(m) > 1 {
		out.HasTitle = true
		out.Title = strings.TrimSpace(m[1])
	} else {
		// Fallback: any line that looks like a title (short, no URL)
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "http") || len(line) > 120 {
				continue
			}
			if len(line) >= 5 && len(line) <= 100 {
				out.HasTitle = true
				out.Title = line
				break
			}
		}
	}

	// Number of sources used
	sourcesRx := regexp.MustCompile(`(?i)(?:sources?\s+used|number\s+of\s+sources?)\s*[:\-]?\s*(\d+)`)
	if m := sourcesRx.FindStringSubmatch(text); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		out.SourcesUsed = &n
	}
	numRx := regexp.MustCompile(`(\d+)\s*(?:sources?|sources\s+used)`)
	if out.SourcesUsed == nil {
		if m := numRx.FindStringSubmatch(text); len(m) > 1 {
			n, _ := strconv.Atoi(m[1])
			out.SourcesUsed = &n
		}
	}

	// Number of tool calls made
	toolCallsRx := regexp.MustCompile(`(?i)(?:tool\s+calls?\s+made|number\s+of\s+tool\s+calls?)\s*[:\-]?\s*(\d+)`)
	if m := toolCallsRx.FindStringSubmatch(text); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		out.ToolCallsMade = &n
	}
	toolNumRx := regexp.MustCompile(`(\d+)\s*(?:tool\s+calls?|tool\s+calls?\s+made)`)
	if out.ToolCallsMade == nil {
		if m := toolNumRx.FindStringSubmatch(text); len(m) > 1 {
			n, _ := strconv.Atoi(m[1])
			out.ToolCallsMade = &n
		}
	}

	// Pass: require at least URL and one numeric metric; title is optional but preferred
	out.Pass = out.HasURL && (out.SourcesUsed != nil || out.ToolCallsMade != nil)
	if !out.HasURL {
		out.Message = "response missing published post URL"
	} else if out.SourcesUsed == nil && out.ToolCallsMade == nil {
		out.Message = "response missing number of sources used and/or tool calls made"
	} else {
		msg := "has URL"
		if out.HasTitle {
			msg += ", title"
		}
		if out.SourcesUsed != nil {
			msg += ", sources=" + strconv.Itoa(*out.SourcesUsed)
		}
		if out.ToolCallsMade != nil {
			msg += ", tool_calls=" + strconv.Itoa(*out.ToolCallsMade)
		}
		out.Message = msg
	}
	return out
}

// ReadCapturedResponse reads the workflow response from env or file.
// It checks OBOT_EVAL_CAPTURED_RESPONSE first (raw text), then OBOT_EVAL_CAPTURED_RESPONSE_FILE (path).
// Returns the text and true if found, or "", false.
func ReadCapturedResponse() (string, bool) {
	if s := os.Getenv("OBOT_EVAL_CAPTURED_RESPONSE"); s != "" {
		return strings.TrimSpace(s), true
	}
	path := os.Getenv("OBOT_EVAL_CAPTURED_RESPONSE_FILE")
	if path == "" {
		return "", false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}
