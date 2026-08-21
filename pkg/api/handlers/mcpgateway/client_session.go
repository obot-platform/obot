package mcpgateway

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/mcp/auditlogs"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func mcpClientSessionName(metadata map[string]string, sessionID string) string {
	return name.SafeConcatName("mcp", "client", "session", hash.String([]string{
		metadata["mcpID"],
		metadata["userID"],
		sessionID,
	}))
}

func saveMCPClientSession(ctx context.Context, client kclient.Client, entry *auditlogs.MCPAuditLog, virtual bool) (bool, error) {
	if entry.SessionID == "" {
		return false, nil
	}

	key := kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      mcpClientSessionName(entry.Metadata, entry.SessionID),
	}

	var session v1.MCPClientSession
	if err := client.Get(ctx, key, &session); apierrors.IsNotFound(err) {
		if entry.ClientName == "" && entry.ClientVersion == "" {
			return false, nil
		}
		session = v1.MCPClientSession{
			Namespace: key.Namespace,
			Name:      key.Name,
			Spec: v1.MCPClientSessionSpec{
				MCPServerID:   entry.Metadata["mcpID"],
				UserID:        entry.Metadata["userID"],
				ClientName:    entry.ClientName,
				ClientVersion: entry.ClientVersion,
				Virtual:       virtual,
			},
		}
		if err := client.Create(ctx, &session); err != nil && !apierrors.IsAlreadyExists(err) {
			return false, fmt.Errorf("failed to create client session: %w", err)
		} else if apierrors.IsAlreadyExists(err) {
			if err := client.Get(ctx, key, &session); err != nil {
				return false, fmt.Errorf("get concurrently created MCP client session: %w", err)
			}
		}
	} else if err != nil {
		return false, fmt.Errorf("get MCP client session: %w", err)
	}

	if session.Spec.MCPServerID != entry.Metadata["mcpID"] || session.Spec.UserID != entry.Metadata["userID"] {
		return false, fmt.Errorf("inconsistent MCP client session data")
	}
	if entry.ClientName == "" {
		entry.ClientName = session.Spec.ClientName
	} else if session.Spec.ClientName != entry.ClientName {
		return false, fmt.Errorf("inconsistent MCP client session data")
	}
	if entry.ClientVersion == "" {
		entry.ClientVersion = session.Spec.ClientVersion
	} else if session.Spec.ClientVersion != entry.ClientVersion {
		return false, fmt.Errorf("inconsistent MCP client session data")
	}
	session.Status.LastUsed = metav1.Now()

	// Ignore conflict errors. That just means the time was updated between get and update, and that's fine.
	if err := client.Status().Update(ctx, &session); err != nil && !apierrors.IsConflict(err) {
		return false, fmt.Errorf("update MCP client session: %w", err)
	}
	return session.Spec.Virtual, nil
}
