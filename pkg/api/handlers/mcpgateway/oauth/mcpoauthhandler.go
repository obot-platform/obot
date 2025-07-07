package oauth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type mcpOAuthHandler struct {
	client     kclient.Client
	gptscript  *gptscript.GPTScript
	stateCache *stateCache
	mcpID      string
	urlChan    chan string
}

func (m *mcpOAuthHandler) HandleAuthURL(ctx context.Context, _ string, authURL string) (bool, error) {
	select {
	case m.urlChan <- authURL:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	default:
		return false, nil
	}
}

func (m *mcpOAuthHandler) NewState(ctx context.Context, conf *oauth2.Config, verifier string) (string, <-chan nmcp.CallbackPayload, error) {
	state := strings.ToLower(rand.Text())

	ch := make(chan nmcp.CallbackPayload)
	return state, ch, m.stateCache.store(ctx, m.mcpID, state, verifier, conf, ch)
}

func (m *mcpOAuthHandler) Lookup(ctx context.Context, authServerURL string) (string, string, error) {
	var oauthApps v1.OAuthAppList
	if err := m.client.List(ctx, &oauthApps, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.authorizationServerURL": authServerURL,
		}),
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return "", "", err
	}

	if len(oauthApps.Items) != 1 {
		return "", "", fmt.Errorf("expected exactly one oauth app for authorization server %s, found %d", authServerURL, len(oauthApps.Items))
	}

	app := oauthApps.Items[0]

	var clientSecret string
	cred, err := m.gptscript.RevealCredential(ctx, []string{app.Name}, app.Spec.Manifest.Alias)
	if err != nil {
		var errNotFound gptscript.ErrNotFound
		if errors.As(err, &errNotFound) {
			if app.Spec.Manifest.ClientSecret != "" {
				clientSecret = app.Spec.Manifest.ClientSecret
			}
		} else {
			return "", "", err
		}
	} else {
		clientSecret = cred.Env["CLIENT_SECRET"]
	}

	return app.Spec.Manifest.ClientID, clientSecret, nil
}
