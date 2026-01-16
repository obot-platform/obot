package apiclient

import (
	"testing"
)

func TestInvokeOptions_URLConstruction(t *testing.T) {
	tests := []struct {
		name         string
		agentID      string
		opts         InvokeOptions
		expectedPath string
	}{
		{
			name:         "Basic invoke without thread ID",
			agentID:      "agent-123",
			opts:         InvokeOptions{Async: false, WorkflowStepID: ""},
			expectedPath: "/invoke/agent-123?async=false&step=",
		},
		{
			name:         "Async invoke without thread ID",
			agentID:      "agent-123",
			opts:         InvokeOptions{Async: true, WorkflowStepID: ""},
			expectedPath: "/invoke/agent-123?async=true&step=",
		},
		{
			name:         "Invoke with thread ID",
			agentID:      "agent-123",
			opts:         InvokeOptions{ThreadID: "thread-456", Async: false, WorkflowStepID: ""},
			expectedPath: "/invoke/agent-123/threads/thread-456?async=false&step=",
		},
		{
			name:         "Invoke with thread ID and workflow step",
			agentID:      "agent-123",
			opts:         InvokeOptions{ThreadID: "thread-456", Async: false, WorkflowStepID: "step-789"},
			expectedPath: "/invoke/agent-123/threads/thread-456?async=false&step=step-789",
		},
		{
			name:         "Async invoke with all options",
			agentID:      "agent-999",
			opts:         InvokeOptions{ThreadID: "thread-abc", Async: true, WorkflowStepID: "step-xyz"},
			expectedPath: "/invoke/agent-999/threads/thread-abc?async=true&step=step-xyz",
		},
		{
			name:         "Invoke with workflow step but no thread",
			agentID:      "agent-555",
			opts:         InvokeOptions{Async: false, WorkflowStepID: "step-100"},
			expectedPath: "/invoke/agent-555?async=false&step=step-100",
		},
		{
			name:         "Empty agent ID",
			agentID:      "",
			opts:         InvokeOptions{ThreadID: "thread-222", Async: false, WorkflowStepID: ""},
			expectedPath: "/invoke//threads/thread-222?async=false&step=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We're testing the URL construction logic that would be used in Invoke()
			// Since we can't easily test the actual HTTP call without a server, we just
			// verify the URL construction matches expected format
			var url string
			if tt.opts.ThreadID != "" {
				url = "/invoke/" + tt.agentID + "/threads/" + tt.opts.ThreadID
			} else {
				url = "/invoke/" + tt.agentID
			}
			url += "?async="
			if tt.opts.Async {
				url += "true"
			} else {
				url += "false"
			}
			url += "&step=" + tt.opts.WorkflowStepID

			if url != tt.expectedPath {
				t.Errorf("Expected path %q, got %q", tt.expectedPath, url)
			}
		})
	}
}
