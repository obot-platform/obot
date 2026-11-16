package server

import (
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
)

type Options struct {
	Hostname   string
	UIHostname string `name:"ui-hostname" env:"OBOT_SERVER_UI_HOSTNAME"`

	// The daily token limits seed the LLM gateway's "Daily token limit" budget on every profile
	// the first time it starts.
	DailyUserInputTokenLimit  int `usage:"Daily input token limit per user in the LLM gateway's Daily token limit budget, put on every profile when the gateway first starts; < 0 for no limit" default:"10000000"` // default is 10 million
	DailyUserOutputTokenLimit int `usage:"Daily output token limit per user in the LLM gateway's Daily token limit budget, put on every profile when the gateway first starts; < 0 for no limit" default:"100000"`  // default is 100 thousand

	// AuthEnabled is whether the server authenticates callers. The API key webhook answers for agent keys only when
	// it is set, because agents are not supported without authentication. It is not a flag of its own: the server
	// sets it from its EnableAuthentication option.
	AuthEnabled bool `usage:"-"`
}

type Server struct {
	db             *db.DB
	baseURL, uiURL string
	tokenService   *persistent.TokenService
	dispatcher     *dispatcher.Dispatcher
	acrHelper      *accesscontrolrule.Helper
	mapHelper      *modelaccesspolicy.Helper
	llmGateway     *llmengine.Engine
	// authEnabled lets the API key webhook answer for agent keys.
	authEnabled bool
}

func New(db *db.DB, tokenService *persistent.TokenService, modelProviderDispatcher *dispatcher.Dispatcher, acrHelper *accesscontrolrule.Helper, mapHelper *modelaccesspolicy.Helper, opts Options) (*Server, error) {
	s := &Server{
		db:           db,
		baseURL:      opts.Hostname,
		uiURL:        opts.UIHostname,
		tokenService: tokenService,
		dispatcher:   modelProviderDispatcher,
		acrHelper:    acrHelper,
		mapHelper:    mapHelper,

		authEnabled: opts.AuthEnabled,
	}

	return s, nil
}
