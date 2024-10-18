package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/acorn-io/baaah"
	"github.com/acorn-io/baaah/pkg/leader"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/adrg/xdg"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/sdkserver"
	"github.com/otto8-ai/otto8/pkg/aihelper"
	"github.com/otto8-ai/otto8/pkg/api/authn"
	"github.com/otto8-ai/otto8/pkg/api/authz"
	"github.com/otto8-ai/otto8/pkg/api/server"
	"github.com/otto8-ai/otto8/pkg/events"
	"github.com/otto8-ai/otto8/pkg/gateway/client"
	"github.com/otto8-ai/otto8/pkg/gateway/db"
	gserver "github.com/otto8-ai/otto8/pkg/gateway/server"
	"github.com/otto8-ai/otto8/pkg/invoke"
	"github.com/otto8-ai/otto8/pkg/jwt"
	"github.com/otto8-ai/otto8/pkg/proxy"
	"github.com/otto8-ai/otto8/pkg/storage"
	"github.com/otto8-ai/otto8/pkg/storage/scheme"
	"github.com/otto8-ai/otto8/pkg/storage/services"
	"github.com/otto8-ai/otto8/pkg/system"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"
	// Setup baaah logging
	_ "github.com/acorn-io/baaah/pkg/logrus"
)

type (
	AuthConfig    proxy.Config
	GatewayConfig gserver.Options
)

type Config struct {
	HTTPListenPort int    `usage:"HTTP port to listen on" default:"8080" name:"http-listen-port"`
	DevMode        bool   `usage:"Enable development mode" default:"false" name:"dev-mode" env:"OTTO_DEV_MODE"`
	DevUIPort      int    `usage:"The port on localhost running the dev instance of the UI" default:"5173"`
	AllowedOrigin  string `usage:"Allowed origin for CORS"`
	ToolRegistry   string `usage:"The tool reference for the tool registry" default:"github.com/gptscript-ai/tools"`

	AuthConfig
	GatewayConfig
	services.Config
}

type Services struct {
	ToolRegistryURL string
	DevUIPort       int
	Events          *events.Emitter
	StorageClient   storage.Client
	Router          *router.Router
	GPTClient       *gptscript.GPTScript
	Invoker         *invoke.Invoker
	TokenServer     *jwt.TokenService
	APIServer       *server.Server
	AIHelper        *aihelper.AIHelper
	Started         chan struct{}
	ProxyServer     *proxy.Proxy
	GatewayServer   *gserver.Server
}

func newGPTScript(ctx context.Context) (*gptscript.GPTScript, error) {
	if os.Getenv("GPTSCRIPT_URL") != "" {
		return gptscript.NewGPTScript(gptscript.GlobalOptions{
			URL:           os.Getenv("GPTSCRIPT_URL"),
			WorkspaceTool: "github.com/gptscript-ai/workspace-provider",
		})
	}

	url, err := sdkserver.EmbeddedStart(ctx)
	if err != nil {
		return nil, err
	}

	if err := os.Setenv("GPTSCRIPT_URL", url); err != nil {
		return nil, err
	}
	if err = os.Setenv("WORKSPACE_PROVIDER_DATA_HOME", filepath.Join(xdg.DataHome, "otto8", "workspace-provider")); err != nil {
		return nil, err
	}

	return gptscript.NewGPTScript(gptscript.GlobalOptions{
		URL:           url,
		WorkspaceTool: "github.com/gptscript-ai/workspace-provider",
	})
}

func New(ctx context.Context, config Config) (*Services, error) {
	system.SetBinToSelf()

	devPort, config := configureDevMode(config)

	if strings.HasPrefix(config.DSN, "postgres://") {
		_ = os.Setenv("KNOW_VECTOR_DB", strings.Replace(config.DSN, "postgres://", "pgvector://", 1))
		_ = os.Setenv("KNOW_INDEX_DSN", strings.Replace(config.DSN, "postgres://", "pgvector://", 1))
	}

	storageClient, restConfig, dbAccess, err := storage.Start(ctx, config.Config)
	if err != nil {
		return nil, err
	}

	if config.DevMode {
		startDevMode(ctx, storageClient)
		config.GatewayDebug = true
	}

	c, err := newGPTScript(ctx)
	if err != nil {
		return nil, err
	}

	r, err := baaah.NewRouter("otto-controller", &baaah.Options{
		DefaultRESTConfig: restConfig,
		Scheme:            scheme.Scheme,
		ElectionConfig:    leader.NewDefaultElectionConfig("", "otto-controller", restConfig),
	})
	if err != nil {
		return nil, err
	}

	// For now, always auto-migrate.
	gatewayDB, err := db.New(dbAccess.DB, dbAccess.SQLDB, true)
	if err != nil {
		return nil, err
	}

	gatewayServer, err := gserver.New(ctx, gatewayDB, config.AuthAdminEmails, gserver.Options(config.GatewayConfig))
	if err != nil {
		return nil, err
	}

	authProviderID, err := gatewayServer.UpsertAuthProvider(ctx, config.GoogleClientID, config.GoogleClientSecret)
	if err != nil {
		return nil, err
	}

	var (
		gatewayClient                        = client.New(gatewayDB, config.AuthAdminEmails)
		authenticators authenticator.Request = gatewayServer
		proxyServer    *proxy.Proxy
	)

	if config.GoogleClientID != "" && config.GoogleClientSecret != "" {
		// "Authentication Enabled" flow
		proxyServer, err = proxy.New(authProviderID, proxy.Config(config.AuthConfig))
		if err != nil {
			return nil, fmt.Errorf("failed to start auth server: %w", err)
		}

		// Token Auth + OAuth auth
		authenticators = union.New(authenticators, proxyServer)
		// Add gateway user info
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)
		// Add anonymous user authenticator
		authenticators = union.New(authenticators, authn.Anonymous{})
	} else {
		// "Authentication Disabled" flow

		// Add gateway user info if token auth worked
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)

		// Add no auth authenticator
		authenticators = union.New(authenticators, authn.NoAuth{})
	}

	var (
		tokenServer = &jwt.TokenService{}
		events      = events.NewEmitter(storageClient)
	)

	// For now, always auto-migrate the gateway database
	return &Services{
		DevUIPort:       devPort,
		ToolRegistryURL: config.ToolRegistry,
		Events:          events,
		StorageClient:   storageClient,
		Router:          r,
		GPTClient:       c,
		APIServer:       server.NewServer(storageClient, c, authn.NewAuthenticator(authenticators), authz.NewAuthorizer()),
		TokenServer:     tokenServer,
		Invoker:         invoke.NewInvoker(storageClient, c, tokenServer, events),
		AIHelper:        aihelper.New(c, config.HelperModel),
		GatewayServer:   gatewayServer,
		ProxyServer:     proxyServer,
	}, nil
}

func configureDevMode(config Config) (int, Config) {
	if !config.DevMode {
		return 0, config
	}

	if config.StorageListenPort == 0 {
		if config.HTTPListenPort == 8080 {
			config.StorageListenPort = 8443
		} else {
			config.StorageListenPort = config.HTTPListenPort + 1
		}
	}
	if config.StorageToken == "" {
		config.StorageToken = "adminpass"
	}
	_ = os.Setenv("BAAAH_DEV_MODE", "true")
	_ = os.Setenv("WORKSPACE_PROVIDER_IGNORE_WORKSPACE_NOT_FOUND", "true")
	return config.DevUIPort, config
}

func startDevMode(ctx context.Context, storageClient storage.Client) {
	_ = storageClient.Delete(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "otto-controller",
			Namespace: "kube-system",
		},
	})
}
