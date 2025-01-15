package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/jwt"
)

type Options struct {
	Hostname     string
	UIHostname   string `name:"ui-hostname" env:"OBOT_SERVER_UI_HOSTNAME"`
	GatewayDebug bool
}

type Server struct {
	adminEmails    map[string]struct{}
	db             *db.DB
	baseURL, uiURL string
	httpClient     *http.Client
	client         *client.Client
	tokenService   *jwt.TokenService
	dispatcher     *dispatcher.Dispatcher
}

func New(ctx context.Context, db *db.DB, tokenService *jwt.TokenService, modelProviderDispatcher *dispatcher.Dispatcher, adminEmails []string, opts Options) (*Server, error) {
	if err := db.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("auto migrate failed: %w", err)
	}

	adminEmailsSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminEmailsSet[email] = struct{}{}
	}

	s := &Server{
		adminEmails:  adminEmailsSet,
		db:           db,
		baseURL:      opts.Hostname,
		uiURL:        opts.UIHostname,
		httpClient:   &http.Client{},
		client:       client.New(db, adminEmails),
		tokenService: tokenService,
		dispatcher:   modelProviderDispatcher,
	}

	go s.autoCleanupTokens(ctx)
	go s.oAuthCleanup(ctx)

	return s, nil
}
