package authz

import (
	"net/http"
	"strings"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"golang.org/x/crypto/bcrypt"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkOAuthClient(r *http.Request) bool {
	key, ok := strings.CutPrefix(r.URL.Path, "/oauth/register/")
	if !ok {
		return false
	}

	namespace, name, ok := strings.Cut(key, "/")
	if !ok {
		return false
	}

	var oauthClient v1.OAuthClient
	err := a.storage.Get(r.Context(), kclient.ObjectKey{Namespace: namespace, Name: name}, &oauthClient)

	return err == nil && bcrypt.CompareHashAndPassword(oauthClient.Spec.RegistrationTokenHash, []byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))) == nil
}
