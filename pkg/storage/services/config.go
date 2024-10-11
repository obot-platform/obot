package services

import (
	"github.com/acorn-io/baaah/pkg/randomtoken"
	"github.com/acorn-io/mink/pkg/db"
	"github.com/otto8-ai/otto8/pkg/storage/authn"
	"github.com/otto8-ai/otto8/pkg/storage/authz"
	"github.com/otto8-ai/otto8/pkg/storage/scheme"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type Config struct {
	StorageListenPort int    `usage:"Port to storage backend will listen on (default: random port)"`
	StorageToken      string `usage:"Token for storage access, will be generated if not passed"`
	DSN               string `usage:"Database dsn in driver://connection_string format" default:"sqlite://file:otto8.db?_journal=WAL&cache=shared&_busy_timeout=30000"`
	HelperModel       string `usage:"The model used to generate names and descriptions" default:"gpt-4o-mini"`
}

type Services struct {
	DB    *db.Factory
	Authn authenticator.Request
	Authz authorizer.Authorizer
}

func New(config Config) (_ *Services, err error) {
	if config.StorageToken == "" {
		config.StorageToken, err = randomtoken.Generate()
		if err != nil {
			return nil, err
		}
	}

	dbClient, err := db.NewFactory(scheme.Scheme, config.DSN)
	if err != nil {
		return nil, err
	}

	services := &Services{
		DB:    dbClient,
		Authn: authn.NewAuthenticator(config.StorageToken),
		Authz: &authz.Authorizer{},
	}

	return services, nil
}
