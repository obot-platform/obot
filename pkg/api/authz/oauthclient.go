package authz

import (
	"encoding/base64"
	"net/http"
	"strings"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"golang.org/x/crypto/bcrypt"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkOAuthClient(r *http.Request) bool {
	key, ok := strings.CutPrefix(r.URL.Path, "/oauth/register/")
	if !ok {
		return a.oauthClientBasicAuth(r)
	}

	namespace, name, ok := strings.Cut(key, ":")
	if !ok {
		return false
	}

	var oauthClient v1.OAuthClient
	err := a.storage.Get(r.Context(), kclient.ObjectKey{Namespace: namespace, Name: name}, &oauthClient)

	return err == nil && bcrypt.CompareHashAndPassword(oauthClient.Spec.RegistrationTokenHash, []byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))) == nil
}

func (a *Authorizer) oauthClientBasicAuth(r *http.Request) bool {
	if r.URL.Path != "/oauth/token" {
		return false
	}

	// Check for basic auth
	client, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Basic ")
	if !ok {
		return false
	}

	decoded, err := base64.StdEncoding.DecodeString(client)
	if err != nil {
		return false
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 3 {
		return false
	}

	var oauthClient v1.OAuthClient
	if err = a.storage.Get(r.Context(), kclient.ObjectKey{Namespace: parts[0], Name: parts[1]}, &oauthClient); err != nil {
		return false
	}

	return bcrypt.CompareHashAndPassword(oauthClient.Spec.ClientSecretHash, []byte(parts[2])) == nil
}
