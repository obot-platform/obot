package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/create"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (*Handler) ExtractID(req *http.Request) string {
	return req.Header.Get("Mcp-Session-Id")
}

func (h *Handler) Store(req *http.Request, sessionID string, sess *nmcp.ServerSession) error {
	var state nmcp.SessionState
	if sessionState, err := sess.GetSession().State(); err != nil {
		return fmt.Errorf("failed to get session state: %w", err)
	} else if sessionState != nil {
		state = *sessionState
	}

	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to encode session state: %w", err)
	}

	mcpSess, _, err := h.get(req.Context(), sessionID)
	if apierrors.IsNotFound(err) {
		mcpSess = &v1.MCPSession{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:  system.DefaultNamespace,
				Name:       sessionID,
				Finalizers: []string{v1.MCPSessionFinalizer},
			},
		}
	} else if err != nil {
		return err
	}

	if !bytes.Equal(mcpSess.Spec.State, b) {
		mcpSess.Spec.State = b
		if err = create.OrUpdate(req.Context(), h.storageClient, mcpSess); err != nil {
			return err
		}
	}

	h.mcpSessionCache.Store(sessionID, mcpSess)
	h.sessionCache.Store(sessionID, sess)

	return nil
}

func (h *Handler) Load(req *http.Request, sessionID string) (*nmcp.ServerSession, bool, error) {
	mcpSess, sess, err := h.get(req.Context(), sessionID)
	if err != nil {
		return nil, false, err
	}

	// If the session hasn't been updated in the last hour, update it.
	if time.Since(mcpSess.Status.LastUsedTime.Time) > time.Hour {
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := h.storageClient.Get(req.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, mcpSess); err != nil {
				return err
			}
			mcpSess.Status.LastUsedTime = metav1.Now()
			return h.storageClient.Status().Update(req.Context(), mcpSess)
		}); err != nil {
			return nil, false, err
		}
	}

	return sess, true, nil
}

func (h *Handler) LoadAndDelete(req *http.Request, sessionID string) (*nmcp.ServerSession, bool, error) {
	mcpSession, ok := h.mcpSessionCache.LoadAndDelete(sessionID)
	session, _ := h.sessionCache.LoadAndDelete(sessionID)

	var (
		mcpSess *v1.MCPSession
		sess    *nmcp.ServerSession
	)
	if ok {
		mcpSess = mcpSession.(*v1.MCPSession)
		sess = session.(*nmcp.ServerSession)
	} else {
		mcpSess = new(v1.MCPSession)
		err := h.storageClient.Get(req.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, mcpSess)
		if err != nil {
			return nil, false, err
		}

		var sessionState nmcp.SessionState
		if err = json.Unmarshal(mcpSess.Spec.State, &sessionState); err != nil {
			return nil, false, fmt.Errorf("failed to decode session state: %w", err)
		}

		sess, err = nmcp.NewExistingServerSession(req.Context(), sessionState, h)
		if err != nil {
			return nil, false, err
		}
	}

	return sess, ok, kclient.IgnoreNotFound(h.storageClient.Delete(req.Context(), mcpSess))
}

func (h *Handler) get(ctx context.Context, sessionID string) (*v1.MCPSession, *nmcp.ServerSession, error) {
	var (
		sess    *nmcp.ServerSession
		mcpSess *v1.MCPSession
	)
	mcpSession, ok := h.mcpSessionCache.Load(sessionID)
	session, _ := h.sessionCache.Load(sessionID)
	if ok {
		mcpSess = mcpSession.(*v1.MCPSession)
		sess = session.(*nmcp.ServerSession)
	} else {
		mcpSess = new(v1.MCPSession)
		err := h.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, mcpSess)
		if err != nil {
			return nil, nil, err
		}

		var sessionState nmcp.SessionState
		if err = json.Unmarshal(mcpSess.Spec.State, &sessionState); err != nil {
			return nil, nil, fmt.Errorf("failed to decode session state: %w", err)
		}

		sess, err = nmcp.NewExistingServerSession(ctx, sessionState, h)
		if err != nil {
			return nil, nil, err
		}

		h.mcpSessionCache.Store(sessionID, mcpSess)
		h.sessionCache.Store(sessionID, sess)
	}

	return mcpSess, sess, nil
}
