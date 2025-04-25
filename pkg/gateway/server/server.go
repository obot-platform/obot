package server

import (
	"context"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/jwt"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Options struct {
	Hostname     string
	UIHostname   string `name:"ui-hostname" env:"OBOT_SERVER_UI_HOSTNAME"`
	GatewayDebug bool

	DailyUserPromptTokenLimit     int `usage:"The maximum number of daily user prompt/input token to allow, <= 0 disables the limit" default:"10000000"`     // default is 10 million
	DailyUserCompletionTokenLimit int `usage:"The maximum number of daily user completion/output tokens to allow, <= 0 disables the limit" default:"100000"` // default is 100 thousand
}

type Server struct {
	db                                 *db.DB
	client                             *client.Client
	baseURL, uiURL                     string
	tokenService                       *jwt.TokenService
	dispatcher                         *dispatcher.Dispatcher
	gptClient                          *gptscript.GPTScript
	storageClient                      kclient.Client
	dailyUserTokenPromptTokenLimit     int
	dailyUserTokenCompletionTokenLimit int
}

func New(ctx context.Context, storageClient kclient.Client, g *gptscript.GPTScript, db *db.DB, tokenService *jwt.TokenService, modelProviderDispatcher *dispatcher.Dispatcher, encryptionConfig *encryptionconfig.EncryptionConfiguration, adminEmails []string, opts Options) (*Server, error) {
	s := &Server{
		db:                                 db,
		client:                             client.New(db, encryptionConfig, adminEmails),
		baseURL:                            opts.Hostname,
		uiURL:                              opts.UIHostname,
		tokenService:                       tokenService,
		dispatcher:                         modelProviderDispatcher,
		gptClient:                          g,
		storageClient:                      storageClient,
		dailyUserTokenPromptTokenLimit:     opts.DailyUserPromptTokenLimit,
		dailyUserTokenCompletionTokenLimit: opts.DailyUserCompletionTokenLimit,
	}

	go s.autoCleanupTokens(ctx)
	go s.oAuthCleanup(ctx)

	return s, nil
}
