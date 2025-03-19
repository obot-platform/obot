package server

import (
	"context"

	"github.com/gptscript-ai/go-gptscript"
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
	db             *db.DB
	client         *client.Client
	baseURL, uiURL string
	tokenService   *jwt.TokenService
	dispatcher     *dispatcher.Dispatcher
	gptClient      *gptscript.GPTScript
}

func New(ctx context.Context, g *gptscript.GPTScript, db *db.DB, tokenService *jwt.TokenService, modelProviderDispatcher *dispatcher.Dispatcher, adminEmails []string, opts Options) (*Server, error) {
	s := &Server{
		db:           db,
		client:       client.New(db, adminEmails),
		baseURL:      opts.Hostname,
		uiURL:        opts.UIHostname,
		tokenService: tokenService,
		dispatcher:   modelProviderDispatcher,
		gptClient:    g,
	}

	go s.autoCleanupTokens(ctx)
	go s.oAuthCleanup(ctx)

	return s, nil
}
