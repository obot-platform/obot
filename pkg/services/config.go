package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/acorn-io/acorn/pkg/aihelper"
	"github.com/acorn-io/acorn/pkg/api/authn"
	"github.com/acorn-io/acorn/pkg/api/authz"
	"github.com/acorn-io/acorn/pkg/api/server"
	"github.com/acorn-io/acorn/pkg/credstores"
	"github.com/acorn-io/acorn/pkg/events"
	"github.com/acorn-io/acorn/pkg/gateway/client"
	"github.com/acorn-io/acorn/pkg/gateway/db"
	gserver "github.com/acorn-io/acorn/pkg/gateway/server"
	"github.com/acorn-io/acorn/pkg/gateway/server/dispatcher"
	"github.com/acorn-io/acorn/pkg/invoke"
	"github.com/acorn-io/acorn/pkg/jwt"
	"github.com/acorn-io/acorn/pkg/proxy"
	"github.com/acorn-io/acorn/pkg/smtp"
	"github.com/acorn-io/acorn/pkg/storage"
	"github.com/acorn-io/acorn/pkg/storage/scheme"
	"github.com/acorn-io/acorn/pkg/storage/services"
	"github.com/acorn-io/acorn/pkg/system"
	baaah "github.com/acorn-io/nah"
	"github.com/acorn-io/nah/pkg/leader"
	"github.com/acorn-io/nah/pkg/router"
	"github.com/adrg/xdg"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/cache"
	gptscriptai "github.com/gptscript-ai/gptscript/pkg/gptscript"
	"github.com/gptscript-ai/gptscript/pkg/loader"
	"github.com/gptscript-ai/gptscript/pkg/sdkserver"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"

	// Setup nah logging
	_ "github.com/acorn-io/nah/pkg/logrus"
)

type (
	AuthConfig    proxy.Config
	GatewayConfig gserver.Options
)

type Config struct {
	HTTPListenPort             int    `usage:"HTTP port to listen on" default:"8080" name:"http-listen-port"`
	DevMode                    bool   `usage:"Enable development mode" default:"false" name:"dev-mode" env:"ACORN_DEV_MODE"`
	DevUIPort                  int    `usage:"The port on localhost running the dev instance of the UI" default:"5173"`
	AllowedOrigin              string `usage:"Allowed origin for CORS"`
	ToolRegistry               string `usage:"The tool reference for the tool registry" default:"github.com/acorn-io/tools"`
	WorkspaceProviderType      string `usage:"The type of workspace provider to use for non-knowledge workspaces" default:"directory" env:"ACORN_WORKSPACE_PROVIDER_TYPE"`
	WorkspaceTool              string `usage:"The tool reference for the workspace provider" default:"github.com/gptscript-ai/workspace-provider"`
	DatasetsTool               string `usage:"The tool reference for the dataset provider" default:"github.com/gptscript-ai/datasets"`
	HelperModel                string `usage:"The model used to generate names and descriptions" default:"gpt-4o-mini"`
	AWSKMSKeyARN               string `usage:"The ARN of the AWS KMS key to use for encrypting credential storage" env:"ACORN_AWS_KMS_KEY_ARN" name:"aws-kms-key-arn"`
	EncryptionConfigFile       string `usage:"The path to the encryption configuration file" default:"./encryption.yaml"`
	KnowledgeSetIngestionLimit int    `usage:"The maximum number of files to ingest into a knowledge set" default:"1000" env:"ACORN_KNOWLEDGESET_INGESTION_LIMIT" name:"knowledge-set-ingestion-limit"`
	EmailServerName            string `usage:"The name of the email server to display for email receivers (default: ui-hostname value)"`

	AuthConfig
	GatewayConfig
	services.Config
}

type Services struct {
	ToolRegistryURL            string
	WorkspaceProviderType      string
	ServerURL                  string
	EmailServerName            string
	DevUIPort                  int
	Events                     *events.Emitter
	StorageClient              storage.Client
	Router                     *router.Router
	GPTClient                  *gptscript.GPTScript
	Invoker                    *invoke.Invoker
	TokenServer                *jwt.TokenService
	APIServer                  *server.Server
	AIHelper                   *aihelper.AIHelper
	Started                    chan struct{}
	ProxyServer                *proxy.Proxy
	GatewayServer              *gserver.Server
	ModelProviderDispatcher    *dispatcher.Dispatcher
	KnowledgeSetIngestionLimit int
}

const (
	defaultDatasetsTool  = "github.com/gptscript-ai/datasets"
	defaultToolsRegistry = "github.com/acorn-io/tools"
)

func newGPTScript(ctx context.Context, workspaceTool, datasetsTool, toolsRegistry string) (*gptscript.GPTScript, error) {
	if datasetsTool != defaultDatasetsTool {
		loader.Remap[defaultDatasetsTool] = datasetsTool
	}
	if toolsRegistry != defaultToolsRegistry {
		loader.Remap[defaultToolsRegistry] = toolsRegistry
	}

	if os.Getenv("GPTSCRIPT_URL") != "" {
		return gptscript.NewGPTScript(gptscript.GlobalOptions{
			URL:           os.Getenv("GPTSCRIPT_URL"),
			WorkspaceTool: workspaceTool,
			DatasetTool:   datasetsTool,
		})
	}

	url, err := sdkserver.EmbeddedStart(ctx, sdkserver.Options{
		Options: gptscriptai.Options{
			Cache: cache.Options{
				CacheDir: os.Getenv("GPTSCRIPT_CACHE_DIR"),
			},
			SystemToolsDir: os.Getenv("GPTSCRIPT_SYSTEM_TOOLS_DIR"),
		},
		DatasetTool:   datasetsTool,
		WorkspaceTool: workspaceTool,
	})
	if err != nil {
		return nil, err
	}

	if err := os.Setenv("GPTSCRIPT_URL", url); err != nil {
		return nil, err
	}

	if os.Getenv("WORKSPACE_PROVIDER_DATA_HOME") == "" {
		if err = os.Setenv("WORKSPACE_PROVIDER_DATA_HOME", filepath.Join(xdg.DataHome, "acorn", "workspace-provider")); err != nil {
			return nil, err
		}
	}

	return gptscript.NewGPTScript(gptscript.GlobalOptions{
		URL:           url,
		WorkspaceTool: workspaceTool,
		DatasetTool:   datasetsTool,
	})
}

func New(ctx context.Context, config Config) (*Services, error) {
	system.SetBinToSelf()

	devPort, config := configureDevMode(config)

	// Just a common mistake where you put the wrong prefix for the DSN. This seems to be inconsistent across things
	// that use postgres
	config.DSN = strings.Replace(config.DSN, "postgresql://", "postgres://", 1)
	if strings.HasPrefix(config.DSN, "postgres://") {
		_ = os.Setenv("KNOW_VECTOR_DSN", strings.Replace(config.DSN, "postgres://", "pgvector://", 1))
		_ = os.Setenv("KNOW_INDEX_DSN", config.DSN)
	}

	if err := credstores.Init(ctx, config.ToolRegistry, config.DSN, credstores.Options{
		AWSKMSKeyARN:         config.AWSKMSKeyARN,
		EncryptionConfigFile: config.EncryptionConfigFile,
	}); err != nil {
		return nil, err
	}

	storageClient, restConfig, dbAccess, err := storage.Start(ctx, config.Config)
	if err != nil {
		return nil, err
	}

	if config.DevMode {
		startDevMode(ctx, storageClient)
		config.GatewayDebug = true
	}

	if config.GatewayDebug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	if config.Hostname == "" {
		config.Hostname = "http://localhost:8080"
	}
	if config.UIHostname == "" {
		config.UIHostname = config.Hostname
	}

	if strings.HasPrefix(config.Hostname, "localhost") || strings.HasPrefix(config.Hostname, "127.0.0.1") {
		config.Hostname = "http://" + config.Hostname
	} else if !strings.HasPrefix(config.Hostname, "http") {
		config.Hostname = "https://" + config.Hostname
	}
	if !strings.HasPrefix(config.UIHostname, "http") {
		config.UIHostname = "https://" + config.UIHostname
	}

	c, err := newGPTScript(ctx, config.WorkspaceTool, config.DatasetsTool, config.ToolRegistry)
	if err != nil {
		return nil, err
	}

	r, err := baaah.NewRouter("otto-controller", &baaah.Options{
		DefaultRESTConfig: restConfig,
		Scheme:            scheme.Scheme,
		ElectionConfig:    leader.NewDefaultElectionConfig("", "otto-controller", restConfig),
		HealthzPort:       -1,
	})
	if err != nil {
		return nil, err
	}

	// For now, always auto-migrate.
	gatewayDB, err := db.New(dbAccess.DB, dbAccess.SQLDB, true)
	if err != nil {
		return nil, err
	}

	var (
		tokenServer             = &jwt.TokenService{}
		events                  = events.NewEmitter(storageClient)
		gatewayClient           = client.New(gatewayDB, config.AuthAdminEmails)
		invoker                 = invoke.NewInvoker(storageClient, c, config.Hostname, config.WorkspaceProviderType, tokenServer, events)
		modelProviderDispatcher = dispatcher.New(invoker, storageClient, c)

		proxyServer *proxy.Proxy
	)

	gatewayServer, err := gserver.New(ctx, gatewayDB, tokenServer, modelProviderDispatcher, config.AuthAdminEmails, gserver.Options(config.GatewayConfig))
	if err != nil {
		return nil, err
	}

	authProviderID, err := gatewayServer.UpsertAuthProvider(ctx, config.AuthConfigType, config.AuthClientID, config.AuthClientSecret)
	if err != nil {
		return nil, err
	}

	var authenticators authenticator.Request = gatewayServer
	if config.AuthClientID != "" && config.AuthClientSecret != "" {
		// "Authentication Enabled" flow
		proxyServer, err = proxy.New(config.Hostname, authProviderID, proxy.Config(config.AuthConfig))
		if err != nil {
			return nil, fmt.Errorf("failed to start auth server: %w", err)
		}

		// Token Auth + OAuth auth
		authenticators = union.New(authenticators, proxyServer)
		// Add gateway user info
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)
		// Add token auth
		authenticators = union.New(authenticators, tokenServer)
		// Add anonymous user authenticator
		authenticators = union.New(authenticators, authn.Anonymous{})
	} else {
		// "Authentication Disabled" flow

		// Add gateway user info if token auth worked
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)

		// Add no auth authenticator
		authenticators = union.New(authenticators, authn.NoAuth{})
	}

	if config.EmailServerName != "" {
		go smtp.Start(ctx, storageClient, config.EmailServerName)
	}

	// For now, always auto-migrate the gateway database
	return &Services{
		WorkspaceProviderType:      config.WorkspaceProviderType,
		ServerURL:                  config.Hostname,
		DevUIPort:                  devPort,
		ToolRegistryURL:            config.ToolRegistry,
		Events:                     events,
		StorageClient:              storageClient,
		Router:                     r,
		GPTClient:                  c,
		APIServer:                  server.NewServer(storageClient, c, authn.NewAuthenticator(authenticators), authz.NewAuthorizer(), proxyServer, config.Hostname),
		TokenServer:                tokenServer,
		Invoker:                    invoker,
		AIHelper:                   aihelper.New(c, config.HelperModel),
		GatewayServer:              gatewayServer,
		ProxyServer:                proxyServer,
		KnowledgeSetIngestionLimit: config.KnowledgeSetIngestionLimit,
		EmailServerName:            config.EmailServerName,
		ModelProviderDispatcher:    modelProviderDispatcher,
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
