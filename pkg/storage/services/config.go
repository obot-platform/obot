package services

import (
	"log/slog"

	"github.com/obot-platform/kinm/pkg/db"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/obot/pkg/logutil"
	"github.com/obot-platform/obot/pkg/storage/authn"
	"github.com/obot-platform/obot/pkg/storage/authz"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type Config struct {
	StorageListenPort int    `usage:"Port to storage backend will listen on (default: random port)"`
	StorageToken      string `usage:"Token for storage access, will be generated if not passed"`
	DSN               string `usage:"Database dsn in driver://connection_string format" default:"sqlite://file:obot.db?_journal=WAL&cache=shared&_busy_timeout=30000"`
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

	// Sanitize DSN for logging (remove credentials)
	sanitizedDSN := logutil.SanitizeDSN(config.DSN)
	slog.Debug("Creating database factory", "dsn", sanitizedDSN)
	dbClient, err := db.NewFactory(scheme.Scheme, config.DSN)
	if err != nil {
		slog.Error("Failed to create database factory", "dsn", sanitizedDSN, "error", err)
		return nil, err
	}
	slog.Debug("Database factory created successfully", "dsn", sanitizedDSN)

	services := &Services{
		DB:    dbClient,
		Authn: authn.NewAuthenticator(config.StorageToken),
		Authz: &authz.Authorizer{},
	}

	return services, nil
}
