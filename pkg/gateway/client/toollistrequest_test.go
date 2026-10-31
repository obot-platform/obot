package client

import (
	"errors"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func TestToolListRequestLifecycle(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	request, err := c.CreateToolListRequest(ctx, "42", "default-everything")
	if err != nil {
		t.Fatalf("CreateToolListRequest() error = %v", err)
	}
	if request.Status != types.ToolListRequestPending || request.EntryName != "default-everything" {
		t.Fatalf("new request = %+v, want a pending request for default-everything", request)
	}
	if !request.ExpiresAt.After(time.Now()) {
		t.Fatalf("new request expires at %v, want later", request.ExpiresAt)
	}

	// Only the user who made a request can see or answer it.
	if _, err := c.ToolListRequest(ctx, "7", request.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("another user's ToolListRequest() error = %v, want not found", err)
	}
	if err := c.CompleteToolListRequest(ctx, "7", request.ID, nil, nil); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("another user's CompleteToolListRequest() error = %v, want not found", err)
	}

	tools := []types2.MCPServerTool{{ID: "echo", Name: "echo", Description: "Echoes its input"}}
	if err := c.CompleteToolListRequest(ctx, "42", request.ID, tools, nil); err != nil {
		t.Fatalf("CompleteToolListRequest() error = %v", err)
	}
	completed, err := c.ToolListRequest(ctx, "42", request.ID)
	if err != nil {
		t.Fatalf("ToolListRequest() error = %v", err)
	}
	if completed.Status != types.ToolListRequestCompleted || len(completed.Tools) != 1 || completed.Tools[0].Name != "echo" {
		t.Fatalf("completed request = %+v, want the echo tool", completed)
	}

	// A request is answered once.
	if err := c.CompleteToolListRequest(ctx, "42", request.ID, nil, errors.New("declined")); !errors.Is(err, ErrToolListRequestNotPending) {
		t.Fatalf("second CompleteToolListRequest() error = %v, want not pending", err)
	}
}

func TestToolListRequestFailureAndExpiry(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	failing, err := c.CreateToolListRequest(ctx, "42", "default-everything")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CompleteToolListRequest(ctx, "42", failing.ID, nil, errors.New("the user declined to launch the server")); err != nil {
		t.Fatalf("CompleteToolListRequest() error = %v", err)
	}
	failed, err := c.ToolListRequest(ctx, "42", failing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != types.ToolListRequestFailed || failed.Error != "the user declined to launch the server" || len(failed.Tools) != 0 {
		t.Fatalf("failed request = %+v", failed)
	}

	expiring, err := c.CreateToolListRequest(ctx, "42", "default-everything")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(ctx).Model(&types.ToolListRequest{}).Where("id = ?", expiring.ID).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	// An expired request can't be read or answered, and cleanup deletes it.
	if _, err := c.ToolListRequest(ctx, "42", expiring.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expired ToolListRequest() error = %v, want not found", err)
	}
	if err := c.CompleteToolListRequest(ctx, "42", expiring.ID, nil, nil); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expired CompleteToolListRequest() error = %v, want not found", err)
	}
	if err := c.CleanupExpiredToolListRequests(ctx); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := c.db.WithContext(ctx).Model(&types.ToolListRequest{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("requests after cleanup = %d, want only the failed one", count)
	}
}
