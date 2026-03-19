package invoke

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestIsApprovedTool(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		approvedTools []string
		expected      bool
	}{
		{
			name:          "exact match",
			toolName:      "myTool",
			approvedTools: []string{"myTool"},
			expected:      true,
		},
		{
			name:          "no match",
			toolName:      "myTool",
			approvedTools: []string{"otherTool"},
			expected:      false,
		},
		{
			name:          "empty approved list",
			toolName:      "myTool",
			approvedTools: nil,
			expected:      false,
		},
		{
			name:          "wildcard matches all",
			toolName:      "anything",
			approvedTools: []string{"*"},
			expected:      true,
		},
		{
			name:          "prefix wildcard match",
			toolName:      "fooBar",
			approvedTools: []string{"foo*"},
			expected:      true,
		},
		{
			name:          "prefix wildcard no match",
			toolName:      "barBaz",
			approvedTools: []string{"foo*"},
			expected:      false,
		},
		{
			name:          "multiple entries match later",
			toolName:      "baz",
			approvedTools: []string{"foo", "bar", "baz"},
			expected:      true,
		},
		{
			name:          "empty tool name",
			toolName:      "",
			approvedTools: []string{"foo"},
			expected:      false,
		},
		{
			name:          "wildcard with empty tool name",
			toolName:      "",
			approvedTools: []string{"*"},
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isApprovedTool(tt.toolName, tt.approvedTools))
		})
	}
}

func TestIsEphemeral(t *testing.T) {
	tests := []struct {
		name     string
		runName  string
		expected bool
	}{
		{
			name:     "ephemeral run with counter",
			runName:  "ephemeral-run-1",
			expected: true,
		},
		{
			name:     "ephemeral run with large counter",
			runName:  "ephemeral-run-99999",
			expected: true,
		},
		{
			name:     "ephemeral run exact prefix",
			runName:  "ephemeral-run",
			expected: true,
		},
		{
			name:     "non-ephemeral run with standard prefix",
			runName:  "r1abc123",
			expected: false,
		},
		{
			name:     "non-ephemeral run empty name",
			runName:  "",
			expected: false,
		},
		{
			name:     "non-ephemeral partial prefix match",
			runName:  "ephemeral-ru",
			expected: false,
		},
		{
			name:     "ephemeral with extended suffix",
			runName:  "ephemeral-runs-extra",
			expected: true, // starts with "ephemeral-run" prefix
		},
		{
			name:     "non-ephemeral different prefix",
			runName:  "eph-something",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &v1.Run{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.runName,
				},
			}
			assert.Equal(t, tt.expected, isEphemeral(run))
		})
	}
}

func TestContextCancellationPropagation(t *testing.T) {
	// This test validates the core invariant of the abort context fix:
	// runCtx created via context.WithCancelCause in Resume() is the parent of
	// both saveCtx (in stream) and timeoutCtx (in stream), so cancelling runCtx
	// cancels all children and records the cause.

	tests := []struct {
		name          string
		causeErr      error
		expectedCause error
	}{
		{
			name:          "abort error propagates to children",
			causeErr:      fmt.Errorf("thread was aborted, cancelling run"),
			expectedCause: fmt.Errorf("thread was aborted, cancelling run"),
		},
		{
			name:          "timeout error propagates to children",
			causeErr:      fmt.Errorf("run exceeded maximum time of 10m0s"),
			expectedCause: fmt.Errorf("run exceeded maximum time of 10m0s"),
		},
		{
			name:          "nil cause still cancels children",
			causeErr:      nil,
			expectedCause: context.Canceled, // WithCancelCause with nil cause reports context.Canceled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parentCtx := context.Background()
			runCtx, cancelRun := context.WithCancelCause(parentCtx)

			// Simulate the child contexts created in stream()
			saveCtx, saveCancel := context.WithCancel(runCtx)
			defer saveCancel()
			timeoutCtx, timeoutCancel := context.WithCancel(runCtx)
			defer timeoutCancel()

			// Before cancellation, all contexts should be active
			assert.NoError(t, runCtx.Err())
			assert.NoError(t, saveCtx.Err())
			assert.NoError(t, timeoutCtx.Err())

			// Cancel the run context (simulating watchThreadAbort or timeoutAfter)
			cancelRun(tt.causeErr)

			// All child contexts should now be done
			assert.Error(t, runCtx.Err())
			assert.Error(t, saveCtx.Err())
			assert.Error(t, timeoutCtx.Err())

			// Verify the cause is recorded correctly
			cause := context.Cause(runCtx)
			assert.Equal(t, tt.expectedCause.Error(), cause.Error())
		})
	}
}

func TestDoubleCancelRunSafety(t *testing.T) {
	// In the fixed code, cancelRun is called in two places:
	// 1. defer cancelRun(nil) in Resume()
	// 2. defer cancelRun(retErr) in stream()
	// The second call should be a no-op — only the first cause is recorded.

	tests := []struct {
		name          string
		firstCause    error
		secondCause   error
		expectedCause string
	}{
		{
			name:          "first real error wins over second",
			firstCause:    errors.New("thread was aborted, cancelling run"),
			secondCause:   errors.New("stream finished with error"),
			expectedCause: "thread was aborted, cancelling run",
		},
		{
			name:          "first real error wins over nil",
			firstCause:    errors.New("run exceeded maximum time of 10m0s"),
			secondCause:   nil,
			expectedCause: "run exceeded maximum time of 10m0s",
		},
		{
			name:          "first nil cause records context.Canceled",
			firstCause:    nil,
			secondCause:   errors.New("late error"),
			expectedCause: context.Canceled.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			runCtx, cancelRun := context.WithCancelCause(ctx)

			// First cancel (e.g., from watchThreadAbort or stream's defer)
			cancelRun(tt.firstCause)
			// Second cancel (e.g., from Resume's defer)
			cancelRun(tt.secondCause)

			cause := context.Cause(runCtx)
			assert.Equal(t, tt.expectedCause, cause.Error())
		})
	}
}

func TestTimeoutAfterCancelsContext(t *testing.T) {
	// Verify timeoutAfter calls cancelRun with an appropriate error message
	// when the timeout fires before context cancellation.

	ctx := context.Background()
	runCtx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(nil)

	// Use a very short timeout so the test completes quickly
	go timeoutAfter(runCtx, cancelRun, 1*time.Nanosecond)

	// Wait for context to be cancelled
	<-runCtx.Done()

	cause := context.Cause(runCtx)
	assert.Contains(t, cause.Error(), "run exceeded maximum time")
}

func TestTimeoutAfterRespectsContextCancellation(t *testing.T) {
	// Verify that timeoutAfter exits cleanly when context is cancelled
	// before the timeout fires.

	ctx := context.Background()
	runCtx, cancelRun := context.WithCancelCause(ctx)

	done := make(chan struct{})
	go func() {
		timeoutAfter(runCtx, cancelRun, 10*time.Minute) // should never fire
		close(done)
	}()

	// Cancel the context immediately
	cancelRun(errors.New("aborted"))

	// timeoutAfter should return promptly
	<-done

	cause := context.Cause(runCtx)
	assert.Equal(t, "aborted", cause.Error())
}
