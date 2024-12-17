package webhook

import (
	"net/url"
	"time"

	"github.com/obot-platform/nah/pkg/ratelimit"
	"github.com/obot-platform/nah/pkg/restconfig"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	"k8s.io/apiserver/pkg/authentication/token/cache"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/plugin/pkg/authenticator/token/webhook"
	"k8s.io/client-go/rest"
)

func New(scheme *runtime.Scheme, webhookURL string) (authenticator.Request, error) {
	restConfig, err := restCfg(webhookURL, scheme)
	if err != nil {
		return nil, err
	}

	wh, err := webhook.New(restConfig, authenticationv1.SchemeGroupVersion.Version, nil, *options.DefaultAuthWebhookRetryBackoff())
	if err != nil {
		return nil, err
	}

	tokenCache := cache.New(wh, false, 10*time.Second, 10*time.Second)
	return bearertoken.New(tokenCache), nil
}

func restCfg(serverURL string, scheme *runtime.Scheme) (*rest.Config, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, err
	}
	insecure := false
	if u.Scheme == "https" && u.Host == "localhost" {
		insecure = true
	}

	cfg := &rest.Config{
		Host: serverURL,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: insecure,
		},
		RateLimiter: ratelimit.None,
	}

	restconfig.SetScheme(cfg, scheme)
	return cfg, nil
}
