package mcpgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/create"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type sessionStoreFactory struct {
	client          kclient.Client
	sessionCache    sync.Map
	mcpSessionCache sync.Map
}

func (sf *sessionStoreFactory) NewStore(handler *messageHandler) nmcp.SessionStore {
	return &sessionStore{
		sessionStoreFactory: sf,
		handler:             handler,
	}
}

type sessionStore struct {
	*sessionStoreFactory
	handler *messageHandler
}

func (s *sessionStore) Store(req *http.Request, sessionID string, session *nmcp.ServerSession) error {
	var state nmcp.SessionState
	if sessionState := session.GetSession().State(); sessionState != nil {
		state = *sessionState
	}

	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to encode session state: %w", err)
	}

	mcpSession := &v1.MCPSession{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  system.DefaultNamespace,
			Name:       sessionID,
			Finalizers: []string{v1.MCPSessionFinalizer},
		},
		Spec: v1.MCPSessionSpec{
			State: b,
		},
	}

	if err = create.IfNotExists(req.Context(), s.client, mcpSession); err != nil {
		return err
	}

	s.sessionCache.Store(sessionID, session)
	s.mcpSessionCache.Store(sessionID, mcpSession)

	return nil
}

func (s *sessionStore) Load(req *http.Request, sessionID string) (*nmcp.ServerSession, bool, error) {
	session, ok := s.sessionCache.Load(sessionID)
	mcpSession, _ := s.mcpSessionCache.Load(sessionID)
	var (
		sess    *nmcp.ServerSession
		mcpSess *v1.MCPSession
	)
	if ok {
		sess = session.(*nmcp.ServerSession)
		mcpSess = mcpSession.(*v1.MCPSession)
	} else {
		mcpSess = new(v1.MCPSession)
		err := s.client.Get(req.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, mcpSess)
		if err != nil {
			return nil, false, err
		}

		var sessionState nmcp.SessionState
		if err = json.Unmarshal(mcpSess.Spec.State, &sessionState); err != nil {
			return nil, false, fmt.Errorf("failed to decode session state: %w", err)
		}

		sess, err = nmcp.NewExistingServerSession(req.Context(), sessionState, s.handler)
		if err != nil {
			return nil, false, err
		}

		s.sessionCache.Store(sessionID, sess)
		s.mcpSessionCache.Store(sessionID, mcpSess)
	}

	// If the session hasn't been updated in the last hour, update it.
	if time.Since(mcpSess.Status.LastUsedTime.Time) > time.Hour {
		mcpSess.Status.LastUsedTime = metav1.Now()
		if err := s.client.Status().Update(req.Context(), mcpSess); err != nil {
			return nil, false, err
		}
	}

	return sess, true, nil
}

func (s *sessionStore) LoadAndDelete(req *http.Request, sessionID string) (*nmcp.ServerSession, bool, error) {
	session, ok := s.sessionCache.LoadAndDelete(sessionID)
	var (
		sess *nmcp.ServerSession
		err  error
	)
	if !ok {
		var mcpSession v1.MCPSession
		if err = s.client.Get(req.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: sessionID}, &mcpSession); err != nil {
			return nil, false, err
		}

		var sessionState nmcp.SessionState
		if err = json.Unmarshal(mcpSession.Spec.State, &sessionState); err != nil {
			return nil, false, fmt.Errorf("failed to decode session state: %w", err)
		}

		sess, err = nmcp.NewExistingServerSession(req.Context(), sessionState, nil)
		if err != nil {
			return nil, false, err
		}
	} else {
		sess, ok = session.(*nmcp.ServerSession)
	}

	return sess, ok, kclient.IgnoreNotFound(s.client.Delete(req.Context(), &v1.MCPSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sessionID,
			Namespace: system.DefaultNamespace,
		},
	}))
}
