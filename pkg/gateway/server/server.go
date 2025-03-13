package server

import (
	"context"
	"net/http"

	"github.com/gptscript-ai/go-gptscript"
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
	tokenService   *jwt.TokenService
	dispatcher     *dispatcher.Dispatcher
	gptClient      *gptscript.GPTScript
}

func New(ctx context.Context, g *gptscript.GPTScript, db *db.DB, tokenService *jwt.TokenService, modelProviderDispatcher *dispatcher.Dispatcher, adminEmails []string, opts Options) (*Server, error) {
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
		tokenService: tokenService,
		dispatcher:   modelProviderDispatcher,
		gptClient:    g,
	}

	go s.autoCleanupTokens(ctx)
	go s.oAuthCleanup(ctx)

	return s, nil
}
