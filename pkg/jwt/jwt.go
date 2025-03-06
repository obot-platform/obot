package jwt

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/pkg/api/authz"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// yeah, duh, this isn't secure, that's not the point right now.
const secret = "this is secret"

type TokenContext struct {
	Namespace      string
	RunID          string
	ThreadID       string
	AgentID        string
	WorkflowID     string
	WorkflowStepID string
	Scope          string
	UserID         string
	UserName       string
	UserEmail      string
}

type TokenService struct{}

func (t *TokenService) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	tokenContext, err := t.DecodeToken(token)
	if err != nil {
		return nil, false, nil
	}
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: tokenContext.Scope,
			Groups: []string{
				authz.AuthenticatedGroup,
			},
			Extra: map[string][]string{
				"obot:runID":     {tokenContext.RunID},
				"obot:threadID":  {tokenContext.ThreadID},
				"obot:agentID":   {tokenContext.AgentID},
				"obot:userID":    {tokenContext.UserID},
				"obot:userName":  {tokenContext.UserName},
				"obot:userEmail": {tokenContext.UserEmail},
			},
		},
	}, true, nil
}

func (t *TokenService) DecodeToken(token string) (*TokenContext, error) {
	tk, err := jwt.Parse(token, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tk.Claims.(jwt.MapClaims)
	if !ok {
		return nil, err
	}
	return &TokenContext{
		Namespace:      claims["Namespace"].(string),
		RunID:          claims["RunID"].(string),
		ThreadID:       claims["ThreadID"].(string),
		AgentID:        claims["AgentID"].(string),
		Scope:          claims["Scope"].(string),
		WorkflowID:     claims["WorkflowID"].(string),
		WorkflowStepID: claims["WorkflowStepID"].(string),
		UserID:         claims["UserID"].(string),
		UserName:       claims["UserName"].(string),
		UserEmail:      claims["UserEmail"].(string),
	}, nil
}

func (t *TokenService) NewToken(context TokenContext) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"Namespace":      context.Namespace,
		"RunID":          context.RunID,
		"ThreadID":       context.ThreadID,
		"AgentID":        context.AgentID,
		"Scope":          context.Scope,
		"WorkflowID":     context.WorkflowID,
		"WorkflowStepID": context.WorkflowStepID,
		"UserID":         context.UserID,
		"UserName":       context.UserName,
		"UserEmail":      context.UserEmail,
	})
	return token.SignedString([]byte(secret))
}
