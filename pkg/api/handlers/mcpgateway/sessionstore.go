package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/create"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func newSessionStore(h *Handler, mcpID, userID string) *sessionStore {
	return &sessionStore{
		storageClient:   h.storageClient,
		messageHandler:  h,
		pendingRequests: &h.pendingRequests,
		mcpSessionCache: &h.mcpSessionCache,
		sessionCache:    &h.sessionCache,
		mcpID:           mcpID,
		userID:          userID,
	}
}

type sessionStore struct {
	storageClient   kclient.Client
	messageHandler  nmcp.MessageHandler
	pendingRequests *sync.Map
	mcpSessionCache *sync.Map
	sessionCache    *sync.Map
	mcpID           string
	userID          string
}

func (*sessionStore) ExtractID(req *http.Request) string {
	return req.Header.Get("Mcp-Session-Id")
}

func (ss *sessionStore) Store(ctx context.Context, sessionID string, sess *nmcp.ServerSession) error {
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

	mcpSess, _, err := ss.get(ctx, ss.messageHandler, sessionID)
	if mcpSess == nil {
		// The session doesn't exist, create a new one.
		mcpSess = &v1.MCPSession{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:  system.DefaultNamespace,
				Name:       sessionID,
				Finalizers: []string{v1.MCPSessionFinalizer},
			},
			Spec: v1.MCPSessionSpec{
				MCPID:  ss.mcpID,
				UserID: ss.userID,
			},
		}
	} else if err != nil {
		return err
	}

	if !bytes.Equal(mcpSess.Spec.State, b) {
		mcpSess.Spec.State = b
		if err = create.OrUpdate(ctx, ss.storageClient, mcpSess); err != nil {
			return err
		}
	}

	ss.mcpSessionCache.Store(sessionID, mcpSess)
	ss.sessionCache.Store(sessionID, sess)

	return nil
}

func (ss *sessionStore) Acquire(ctx context.Context, server nmcp.MessageHandler, sessionID string) (*nmcp.ServerSession, bool, error) {
	mcpSess, sess, err := ss.get(ctx, server, sessionID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get session %s from cache: %w", sessionID, err)
	} else if mcpSess == nil {
		return nil, false, nil
	}

	// If the session hasn't been updated in the last hour, update it.
	if time.Since(mcpSess.Status.LastUsedTime.Time) > time.Hour {
		// Get the latest version of the session from storage.
		s := new(v1.MCPSession)
		if err := ss.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, s); apierrors.IsNotFound(err) {
			// The session is still in memory but not in storage.
			// Remove it from the cache and indicate that the session was not found.
			ss.mcpSessionCache.Delete(sessionID)
			ss.sessionCache.Delete(sessionID)

			return nil, false, nil
		} else if err != nil {
			return nil, false, fmt.Errorf("failed to get session %s: %w", sessionID, err)
		}

		s.Status.LastUsedTime = metav1.Now()
		if err := ss.storageClient.Status().Update(ctx, s); err == nil {
			// Best effort update of session status access time.
			// If there are multiple concurrent requests, there may be conflicts.
			ss.mcpSessionCache.Store(sessionID, s)
		}
	}

	return sess, true, nil
}

func (*sessionStore) Release(*nmcp.ServerSession) {}

func (ss *sessionStore) LoadAndDelete(ctx context.Context, server nmcp.MessageHandler, sessionID string) (*nmcp.ServerSession, bool, error) {
	mcpSession, ok := ss.mcpSessionCache.LoadAndDelete(sessionID)
	session, _ := ss.sessionCache.LoadAndDelete(sessionID)

	var (
		mcpSess *v1.MCPSession
		sess    *nmcp.ServerSession
	)
	if ok {
		mcpSess = mcpSession.(*v1.MCPSession)
		sess = session.(*nmcp.ServerSession)
	}

	// There is a strange issue where the mcpSess's name goes empty.
	// It was likely a race condition that has since been fixed, but it is straightforward to protect against here.
	if !ok || mcpSess == nil || mcpSess.Name == "" {
		mcpSess = new(v1.MCPSession)
		err := ss.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, mcpSess)
		if apierrors.IsNotFound(err) {
			ss.pendingRequests.Delete(sessionID)
			return nil, false, nil
		} else if err != nil {
			return nil, false, err
		}

		var sessionState nmcp.SessionState
		if err = json.Unmarshal(mcpSess.Spec.State, &sessionState); err != nil {
			return nil, false, fmt.Errorf("failed to decode session state: %w", err)
		}

		sess, err = nmcp.NewExistingServerSession(ctx, sessionState, server)
		if err != nil {
			return nil, false, err
		}
	}

	ss.pendingRequests.Delete(sessionID)

	return sess, ok, kclient.IgnoreNotFound(ss.storageClient.Delete(ctx, mcpSess))
}

func (ss *sessionStore) get(ctx context.Context, messageHandler nmcp.MessageHandler, sessionID string) (*v1.MCPSession, *nmcp.ServerSession, error) {
	var (
		sess    *nmcp.ServerSession
		mcpSess *v1.MCPSession
	)
	mcpSession, mcpOK := ss.mcpSessionCache.Load(sessionID)
	session, ok := ss.sessionCache.Load(sessionID)
	if mcpOK && ok {
		mcpSess = mcpSession.(*v1.MCPSession)
		sess = session.(*nmcp.ServerSession)
	} else {
		mcpSess = new(v1.MCPSession)
		err := ss.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, mcpSess)
		if apierrors.IsNotFound(err) {
			return nil, nil, nil
		} else if err != nil {
			return nil, nil, err
		}

		var sessionState nmcp.SessionState
		if err = json.Unmarshal(mcpSess.Spec.State, &sessionState); err != nil {
			return nil, nil, fmt.Errorf("failed to decode session state: %w", err)
		}

		sess, err = nmcp.NewExistingServerSession(ctx, sessionState, messageHandler)
		if err != nil {
			return nil, nil, err
		}

		ss.mcpSessionCache.Store(sessionID, mcpSess)
		ss.sessionCache.Store(sessionID, sess)
	}

	return mcpSess, sess, nil
}
