package mcpgateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/pkg/hash"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errMCPSessionNotFound = errors.New("MCP session not found")
)

// Preserve the existing server/user/session key so matching pre-upgrade sessions can continue.
func mcpClientSessionName(mcpServerID, userID, sessionID string) string {
	return name.SafeConcatName("mcp", "client", "session", hash.String([]string{mcpServerID, userID, sessionID}))
}

// Save only server-issued sessions from successful responses. Audited initialize
// responses may also receive a virtual ID for tracking a stateless upstream.
func saveMCPClientSession(ctx context.Context, storage kclient.Client, resp *http.Response, spec v1.MCPClientSessionSpec, initialize bool) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	id := mcpSessionID(resp.Header, nil)
	spec.Virtual = id == ""
	if spec.Virtual {
		if !initialize || resp.StatusCode != http.StatusOK {
			return nil
		}

		id = uuid.NewString()
	}

	key := mcpClientSessionKey(spec, id)
	session := v1.MCPClientSession{
		Name:      key.Name,
		Namespace: key.Namespace,
		Spec:      spec,
	}

	if err := storage.Create(ctx, &session); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		if err := storage.Get(ctx, key, &session); err != nil {
			return err
		}

		if session.Spec.MCPServerID != spec.MCPServerID || session.Spec.UserID != spec.UserID || session.Spec.Virtual != spec.Virtual ||
			(session.Spec.ConnectionID != "" && session.Spec.ConnectionID != spec.ConnectionID) {
			return fmt.Errorf("inconsistent session ownership")
		}
	}

	session.Status.LastUsed = metav1.Now()
	if err := storage.Status().Update(ctx, &session); err != nil && !apierrors.IsConflict(err) {
		return err
	}

	resp.Header.Set(mcpSessionHeader, id)

	return nil
}

// loadMCPClientSession validates ownership and refreshes activity for the existing record.
func loadMCPClientSession(ctx context.Context, storage kclient.Client, spec v1.MCPClientSessionSpec, sessionID string) (*v1.MCPClientSession, error) {
	if sessionID == "" {
		return nil, nil
	}

	var session v1.MCPClientSession
	if err := storage.Get(ctx, mcpClientSessionKey(spec, sessionID), &session); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errMCPSessionNotFound
		}

		return nil, err
	}

	// Legacy records have no ConnectionID. Keep their existing user/server association;
	// newly created records additionally bind the exact resolved connection.
	if session.Spec.MCPServerID != spec.MCPServerID || session.Spec.UserID != spec.UserID ||
		(session.Spec.ConnectionID != "" && session.Spec.ConnectionID != spec.ConnectionID) {
		return nil, errMCPSessionNotFound
	}

	session.Status.LastUsed = metav1.Now()
	if err := storage.Status().Update(ctx, &session); err != nil && !apierrors.IsConflict(err) {
		return nil, err
	}

	return &session, nil
}

func mcpClientSessionKey(spec v1.MCPClientSessionSpec, sessionID string) kclient.ObjectKey {
	return kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      mcpClientSessionName(spec.MCPServerID, spec.UserID, sessionID),
	}
}

// suppliedSessionID reuses mcpSessionID for extraction and additionally rejects
// conflicting duplicate headers, which mcpSessionID's Header.Get does not check.
func suppliedSessionID(req *http.Request) (string, error) {
	// Only the header identifies a Streamable HTTP session; query parameters are upstream inputs.
	id := mcpSessionID(req.Header, nil)

	for _, value := range req.Header.Values(mcpSessionHeader) {
		if value != "" && value != id {
			return "", fmt.Errorf("conflicting MCP session identifiers")
		}
	}

	return id, nil
}

// isMCPInitializeRequest is used only for unaudited requests with a missing
// session. Inspection restores the body so a fresh initialize can be forwarded.
func isMCPInitializeRequest(req *http.Request) bool {
	if req.Method != http.MethodPost || req.Body == nil {
		return false
	}

	body, err := readMCPProxyRequest(req)
	if err != nil {
		return false
	}

	var message obotmcp.Message
	return decodeMCPHookMessage(body, &message) == nil && message.Method == "initialize"
}
