package mcpgateway

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// hookOrigin distinguishes client-initiated calls from server-initiated calls
// that may reuse the same JSON-RPC request ID.
type hookOrigin string

const (
	hookOriginClient hookOrigin = "client"
	hookOriginServer hookOrigin = "server"
)

// hookCorrelationStore persists pending requests for responses that may reach
// another HTTP request or Obot replica.
type hookCorrelationStore struct {
	client        kclient.Client
	mcpID, userID string
}

func newHookCorrelationStore(client kclient.Client, metadata map[string]string) *hookCorrelationStore {
	if client == nil {
		return nil
	}
	return &hookCorrelationStore{client: client, mcpID: metadata["mcpID"], userID: metadata["userID"]}
}

func (s *hookCorrelationStore) save(ctx context.Context, sessionID, requestID string, origin hookOrigin, request pendingRequest) error {
	if s == nil || sessionID == "" || requestID == "" {
		return nil
	}
	key := s.key(sessionID, requestID, origin)
	spec := v1.MCPHookCorrelationSpec{
		Method: request.message.Method, Name: request.name,
		ExpiresAt: metav1.NewTime(time.Now().Add(v1.MCPHookCorrelationTTL)),
	}
	if mutation, ok := request.mutations["request"]; ok {
		spec.RequestMutation = &v1.MCPHookMutation{
			Mutated: mutation.Mutated,
			Reasons: slices.Clone(mutation.Reasons),
		}
	}
	correlation := &v1.MCPHookCorrelation{ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name}, Spec: spec}
	if err := s.client.Create(ctx, correlation); err == nil {
		return nil
	} else if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create MCP hook correlation: %w", err)
	}
	if err := s.client.Get(ctx, key, correlation); err != nil {
		return fmt.Errorf("get MCP hook correlation: %w", err)
	}
	correlation.Spec = spec
	if err := s.client.Update(ctx, correlation); err != nil {
		return fmt.Errorf("update reused MCP hook correlation: %w", err)
	}
	return nil
}

func (s *hookCorrelationStore) loadAndDelete(ctx context.Context, sessionID, requestID string, origin hookOrigin) (pendingRequest, bool, error) {
	if s == nil || sessionID == "" || requestID == "" {
		return pendingRequest{}, false, nil
	}
	key := s.key(sessionID, requestID, origin)
	var correlation v1.MCPHookCorrelation
	if err := s.client.Get(ctx, key, &correlation); apierrors.IsNotFound(err) {
		return pendingRequest{}, false, nil
	} else if err != nil {
		return pendingRequest{}, false, fmt.Errorf("get MCP hook correlation: %w", err)
	}

	uid, resourceVersion := correlation.UID, correlation.ResourceVersion
	if err := s.client.Delete(ctx, &correlation, kclient.Preconditions{UID: &uid, ResourceVersion: &resourceVersion}); apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
		return pendingRequest{}, false, nil
	} else if err != nil {
		return pendingRequest{}, false, fmt.Errorf("delete MCP hook correlation: %w", err)
	}

	if correlation.Spec.ExpiresAt.Before(new(metav1.Now())) {
		return pendingRequest{}, false, nil
	}

	request := pendingRequest{message: mcp.Message{Method: correlation.Spec.Method}, name: correlation.Spec.Name}
	if mutation := correlation.Spec.RequestMutation; mutation != nil {
		request.mutations = map[string]mcp.HookMutation{"request": {
			Mutated: mutation.Mutated,
			Reasons: slices.Clone(mutation.Reasons),
		}}
	}
	return request, true, nil
}

func (s *hookCorrelationStore) delete(ctx context.Context, sessionID, requestID string, origin hookOrigin) {
	if s == nil || sessionID == "" || requestID == "" {
		return
	}
	correlation := &v1.MCPHookCorrelation{ObjectMeta: metav1.ObjectMeta{Namespace: system.DefaultNamespace, Name: s.key(sessionID, requestID, origin).Name}}
	_ = s.client.Delete(ctx, correlation)
}

func (s *hookCorrelationStore) key(sessionID, requestID string, origin hookOrigin) kclient.ObjectKey {
	value := struct{ MCPID, UserID, SessionID, RequestID, Origin string }{s.mcpID, s.userID, sessionID, requestID, string(origin)}
	return kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "mcp-hook-" + hash.String(value)[:50]}
}
