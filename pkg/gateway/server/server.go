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
}

type Server struct {
	db             *db.DB
	baseURL, uiURL string
	tokenService   *persistent.TokenService
	dispatcher     *dispatcher.Dispatcher
	acrHelper      *accesscontrolrule.Helper
	mapHelper      *modelaccesspolicy.Helper
	llmGateway     *llmengine.Engine
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
	}

	return s, nil
}
