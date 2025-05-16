package jwt

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/obot/pkg/api/authz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/thread"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var secret string

func init() {
	var err error
	secret, err = randomtoken.Generate()
	if err != nil {
		panic(err)
	}
}

type TokenContext struct {
	Namespace      string
	RunID          string
	ThreadID       string
	ProjectID      string
	ModelProvider  string
	Model          string
	AgentID        string
	WorkflowID     string
	WorkflowStepID string
	Scope          string
	MCPServerID    string
	UserID         string
	UserName       string
	UserEmail      string
	UserGroups     []string
}

type TokenService struct {
	client kclient.Client
	token  string
}

func NewTokenService(client kclient.Client) *TokenService {
	return &TokenService{
		client: client,
	}
}

func (t *TokenService) SetLongLivedSigningToken(token string) {
	t.token = token
}

func (t *TokenService) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	tokenContext, err := t.DecodeToken(req.Context(), token)
	if err != nil {
		return nil, false, nil
	}

	groups := append([]string{authz.AuthenticatedGroup}, tokenContext.UserGroups...)
	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:    tokenContext.UserID,
			Name:   tokenContext.Scope,
			Groups: groups,
			Extra: map[string][]string{
				"obot:runID":     {tokenContext.RunID},
				"obot:threadID":  {tokenContext.ThreadID},
				"obot:projectID": {tokenContext.ProjectID},
				"obot:agentID":   {tokenContext.AgentID},
				"obot:userID":    {tokenContext.UserID},
				"obot:userName":  {tokenContext.UserName},
				"obot:userEmail": {tokenContext.UserEmail},
			},
		},
	}, true, nil
}

func (t *TokenService) DecodeToken(ctx context.Context, token string) (*TokenContext, error) {
	var mcpToken bool
	tk, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		if t.token != "" {
			if mcp, _ := token.Claims.(jwt.MapClaims)["MCPServerID"].(string); mcp != "" {
				mcpToken = true
				return []byte(t.token), nil
			}
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := tk.Claims.(jwt.MapClaims)
	if !ok {
		return nil, err
	}

	var groups []string
	if groupsClaim, ok := claims["UserGroups"].(string); ok && groupsClaim != "" {
		groups = strings.Split(groupsClaim, ",")
	}

	modelProvider := claims["ModelProvider"].(string)
	model := claims["Model"].(string)
	projectID := claims["ProjectID"].(string)
	namespace := claims["Namespace"].(string)

	if mcpToken {
		// MCP tokens are long-lived tokens. Therefore, we don't store some of the dynamic information like run ID, model provider and model.
		// We need to get that information dynamically when the token is used.
		var projectThread v1.Thread
		if err = t.client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: projectID}, &projectThread); err != nil {
			return nil, err
		}

		modelProvider, model, err = thread.GetModelAndModelProviderForThread(ctx, t.client, &projectThread)
		if err != nil {
			return nil, fmt.Errorf("failed to get model and model provider for thread: %w", err)
		}
	}

	context := &TokenContext{
		Namespace:      namespace,
		RunID:          claims["RunID"].(string),
		ThreadID:       claims["ThreadID"].(string),
		ProjectID:      projectID,
		ModelProvider:  modelProvider,
		Model:          model,
		AgentID:        claims["AgentID"].(string),
		Scope:          claims["Scope"].(string),
		MCPServerID:    claims["MCPServerID"].(string),
		WorkflowID:     claims["WorkflowID"].(string),
		WorkflowStepID: claims["WorkflowStepID"].(string),
		UserID:         claims["UserID"].(string),
		UserName:       claims["UserName"].(string),
		UserEmail:      claims["UserEmail"].(string),
		UserGroups:     groups,
	}

	return context, nil
}

func (t *TokenService) NewToken(context TokenContext) (string, error) {
	claims := jwt.MapClaims{
		"Namespace":      context.Namespace,
		"RunID":          context.RunID,
		"ThreadID":       context.ThreadID,
		"ProjectID":      context.ProjectID,
		"ModelProvider":  context.ModelProvider,
		"Model":          context.Model,
		"AgentID":        context.AgentID,
		"Scope":          context.Scope,
		"MCPServerID":    context.MCPServerID,
		"WorkflowID":     context.WorkflowID,
		"WorkflowStepID": context.WorkflowStepID,
		"UserID":         context.UserID,
		"UserName":       context.UserName,
		"UserEmail":      context.UserEmail,
		"UserGroups":     strings.Join(context.UserGroups, ","),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	if context.MCPServerID != "" {
		return token.SignedString([]byte(t.token))
	}
	return token.SignedString([]byte(secret))
}
