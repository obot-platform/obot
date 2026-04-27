package authn

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"github.com/obot-platform/obot/pkg/storage/authz"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	ErrInvalidServiceAccountToken = errors.New("invalid service account token")

	adminUserResponse = &authenticator.Response{
		User: &user.DefaultInfo{
			Name: authz.AdminName,
			UID:  authz.AdminName,
			Groups: []string{
				authz.AuthenticatedGroup,
				authz.AdminGroup,
			},
		},
	}
)

type Authenticator struct {
	authToken string

	lock                    sync.RWMutex
	serviceAccountValidator ServiceAccountValidator
}

type ServiceAccountValidator func(context.Context, string) (string, error)

func serviceAccountResponse(accountName string) (*authenticator.Response, bool) {
	account, ok := serviceaccounts.Get(accountName)
	if !ok {
		return nil, false
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: account.Username,
			UID:  account.UID,
			Groups: []string{
				authz.AuthenticatedGroup,
				serviceaccounts.Group,
				account.Group,
			},
		},
	}, true
}

func NewAuthenticator(authToken string) *Authenticator {
	return &Authenticator{
		authToken: authToken,
	}
}

func (a *Authenticator) SetServiceAccountValidator(validator ServiceAccountValidator) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.serviceAccountValidator = validator
}

func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	bearerToken, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	bearerToken = strings.TrimSpace(bearerToken)
	if !ok || bearerToken == "" {
		return nil, false, nil
	}

	if bearerToken == a.authToken {
		return adminUserResponse, true, nil
	}

	a.lock.RLock()
	validator := a.serviceAccountValidator
	a.lock.RUnlock()
	if validator == nil {
		return nil, false, nil
	}

	serviceAccountName, err := validator(req.Context(), bearerToken)
	if errors.Is(err, ErrInvalidServiceAccountToken) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	if resp, ok := serviceAccountResponse(serviceAccountName); ok {
		return resp, true, nil
	}

	return nil, false, nil
}
