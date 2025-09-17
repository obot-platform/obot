package ephemeral

import (
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
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
	Namespace         string
	RunID             string
	ThreadID          string
	ProjectID         string
	TopLevelProjectID string
	ModelProvider     string
	Model             string
	AgentID           string
	WorkflowID        string
	WorkflowStepID    string
	Scope             string
	UserID            string
	UserName          string
	UserEmail         string
	UserGroups        []string
}

type TokenService struct{}

func (t *TokenService) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	tokenContext, err := t.DecodeToken(token)
	if err != nil {
		return nil, false, nil
	}

	groups := tokenContext.UserGroups
	if !slices.Contains(groups, types.GroupAuthenticated) {
		groups = append(groups, types.GroupAuthenticated)
	}
	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:    tokenContext.UserID,
			Name:   tokenContext.Scope,
			Groups: groups,
			Extra: map[string][]string{
				"obot:runID":             {tokenContext.RunID},
				"obot:threadID":          {tokenContext.ThreadID},
				"obot:topLevelProjectID": {tokenContext.TopLevelProjectID},
				"obot:projectID":         {tokenContext.ProjectID},
				"obot:agentID":           {tokenContext.AgentID},
				"obot:userID":            {tokenContext.UserID},
				"obot:userName":          {tokenContext.UserName},
				"obot:userEmail":         {tokenContext.UserEmail},
			},
		},
	}, true, nil
}

func (*TokenService) DecodeToken(token string) (*TokenContext, error) {
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

	groups := strings.Split(claims["UserGroups"].(string), ",")
	groups = slices.DeleteFunc(groups, func(s string) bool { return s == "" })

	context := &TokenContext{
		Namespace:         claims["Namespace"].(string),
		RunID:             claims["RunID"].(string),
		ThreadID:          claims["ThreadID"].(string),
		ProjectID:         claims["ProjectID"].(string),
		TopLevelProjectID: claims["TopLevelProjectID"].(string),
		ModelProvider:     claims["ModelProvider"].(string),
		Model:             claims["Model"].(string),
		AgentID:           claims["AgentID"].(string),
		Scope:             claims["Scope"].(string),
		WorkflowID:        claims["WorkflowID"].(string),
		WorkflowStepID:    claims["WorkflowStepID"].(string),
		UserID:            claims["UserID"].(string),
		UserName:          claims["UserName"].(string),
		UserEmail:         claims["UserEmail"].(string),
		UserGroups:        groups,
	}

	return context, nil
}

func (*TokenService) NewToken(context TokenContext) (string, error) {
	claims := jwt.MapClaims{
		"Namespace":         context.Namespace,
		"RunID":             context.RunID,
		"ThreadID":          context.ThreadID,
		"ProjectID":         context.ProjectID,
		"TopLevelProjectID": context.TopLevelProjectID,
		"ModelProvider":     context.ModelProvider,
		"Model":             context.Model,
		"AgentID":           context.AgentID,
		"Scope":             context.Scope,
		"WorkflowID":        context.WorkflowID,
		"WorkflowStepID":    context.WorkflowStepID,
		"UserID":            context.UserID,
		"UserName":          context.UserName,
		"UserEmail":         context.UserEmail,
		"UserGroups":        strings.Join(context.UserGroups, ","),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
