package jwt

import (
	"fmt"
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
	RunID          string
	ThreadID       string
	AgentID        string
	WorkflowID     string
	WorkflowStepID string
	Scope          string
	UserID         string
	UserName       string
	UserEmail      string
	ExtraScopes    []string
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
				"obot:runID":       {tokenContext.RunID},
				"obot:threadID":    {tokenContext.ThreadID},
				"obot:agentID":     {tokenContext.AgentID},
				"obot:userID":      {tokenContext.UserID},
				"obot:userName":    {tokenContext.UserName},
				"obot:userEmail":   {tokenContext.UserEmail},
				"obot:extraScopes": tokenContext.ExtraScopes,
			},
		},
	}, true, nil
}

func (t *TokenService) DecodeToken(token string) (*TokenContext, error) {
	tk, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := tk.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return &TokenContext{
		RunID:          getClaim[string](claims, "RunID"),
		ThreadID:       getClaim[string](claims, "ThreadID"),
		AgentID:        getClaim[string](claims, "AgentID"),
		Scope:          getClaim[string](claims, "Scope"),
		WorkflowID:     getClaim[string](claims, "WorkflowID"),
		WorkflowStepID: getClaim[string](claims, "WorkflowStepID"),
		UserID:         getClaim[string](claims, "UserID"),
		UserName:       getClaim[string](claims, "UserName"),
		UserEmail:      getClaim[string](claims, "UserEmail"),
		ExtraScopes:    getClaim[[]string](claims, "ExtraScopes"),
	}, nil
}

func getClaim[T any](claims jwt.MapClaims, key string) T {
	var zero T // Default value for type T

	if value, exists := claims[key]; exists {
		switch v := value.(type) {
		case T:
			return v
		case []interface{}:
			// Handle conversion from []interface{} to []string
			if _, ok := any(zero).([]string); ok {
				var result []string
				for _, item := range v {
					if str, ok := item.(string); ok {
						result = append(result, str)
					}
				}
				return any(result).(T) // Convert back to T
			}
		}
	}

	return zero // Return default value if missing or wrong type
}

func (t *TokenService) NewToken(context TokenContext) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"RunID":          context.RunID,
		"ThreadID":       context.ThreadID,
		"AgentID":        context.AgentID,
		"Scope":          context.Scope,
		"WorkflowID":     context.WorkflowID,
		"WorkflowStepID": context.WorkflowStepID,
		"UserID":         context.UserID,
		"UserName":       context.UserName,
		"UserEmail":      context.UserEmail,
		"ExtraScopes":    context.ExtraScopes,
	})
	return token.SignedString([]byte(secret))
}
