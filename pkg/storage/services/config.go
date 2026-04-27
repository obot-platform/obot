package services

import (
	"github.com/obot-platform/kinm/pkg/db"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/logutil"
	"github.com/obot-platform/obot/pkg/storage/authn"
	"github.com/obot-platform/obot/pkg/storage/authz"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

var log = logger.Package()

type Config struct {
	StorageListenPort int    `usage:"Port to storage backend will listen on (default: random port)"`
	StorageToken      string `usage:"Token for storage access, will be generated if not passed"`
	DSN               string `usage:"Database dsn in driver://connection_string format" default:"sqlite://file:obot.db?_journal=WAL&cache=shared&_busy_timeout=30000"`
}

type Services struct {
	DB    *db.Factory
	Authn *authn.Authenticator
	Authz authorizer.Authorizer
}

func New(config Config) (_ *Services, err error) {
	if config.StorageToken == "" {
		config.StorageToken, err = randomtoken.Generate()
		if err != nil {
			return nil, err
		}
	}

	// Sanitize DSN for logging (remove credentials)
	sanitizedDSN := logutil.SanitizeDSN(config.DSN)
	log.Debugf("Creating database factory. dsn: %v", sanitizedDSN)
	dbClient, err := db.NewFactory(scheme.Scheme, config.DSN)
	if err != nil {
		log.Errorf("Failed to create database factory: dsn=%s error=%v", sanitizedDSN, err)
		return nil, err
	}
	log.Debugf("Database factory created successfully. dsn: %v", sanitizedDSN)

	services := &Services{
		DB:    dbClient,
		Authn: authn.NewAuthenticator(config.StorageToken),
		Authz: &authz.Authorizer{},
	}

	return services, nil
}
