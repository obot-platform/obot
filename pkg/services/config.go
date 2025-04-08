package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adrg/xdg"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/cache"
	gptscriptai "github.com/gptscript-ai/gptscript/pkg/gptscript"
	"github.com/gptscript-ai/gptscript/pkg/runner"
	"github.com/gptscript-ai/gptscript/pkg/sdkserver"
	"github.com/obot-platform/nah"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/leader"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/runtime"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/bootstrap"
	"github.com/obot-platform/obot/pkg/credstores"
	"github.com/obot-platform/obot/pkg/encryption"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/db"
	gserver "github.com/obot-platform/obot/pkg/gateway/server"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/gemini"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/jwt"
	"github.com/obot-platform/obot/pkg/proxy"
	"github.com/obot-platform/obot/pkg/smtp"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"

	// Setup nah logging
	_ "github.com/obot-platform/nah/pkg/logrus"
)

type (
	GatewayConfig    gserver.Options
	GeminiConfig     gemini.Config
	AuditConfig      audit.Options
	EncryptionConfig encryption.Options
)

type Config struct {
	HTTPListenPort             int      `usage:"HTTP port to listen on" default:"8080" name:"http-listen-port"`
	DevMode                    bool     `usage:"Enable development mode" default:"false" name:"dev-mode" env:"OBOT_DEV_MODE"`
	DevUIPort                  int      `usage:"The port on localhost running the dev instance of the UI" default:"5173"`
	AllowedOrigin              string   `usage:"Allowed origin for CORS"`
	ToolRegistries             []string `usage:"The remote tool references to the set of tool registries to use" default:"github.com/obot-platform/tools" split:"true"`
	WorkspaceProviderType      string   `usage:"The type of workspace provider to use for non-knowledge workspaces" default:"directory" env:"OBOT_WORKSPACE_PROVIDER_TYPE"`
	HelperModel                string   `usage:"The model used to generate names and descriptions" default:"gpt-4o-mini"`
	EmailServerName            string   `usage:"The name of the email server to display for email receivers"`
	EnableSMTPServer           bool     `usage:"Enable SMTP server to receive emails" default:"false" env:"OBOT_ENABLE_SMTP_SERVER"`
	Docker                     bool     `usage:"Enable Docker support" default:"false" env:"OBOT_DOCKER"`
	EnvKeys                    []string `usage:"The environment keys to pass through to the GPTScript server" env:"OBOT_ENV_KEYS"`
	KnowledgeSetIngestionLimit int      `usage:"The maximum number of files to ingest into a knowledge set" default:"3000" name:"knowledge-set-ingestion-limit"`
	KnowledgeFileWorkers       int      `usage:"The number of workers to process knowledge files" default:"5"`
	EnableAuthentication       bool     `usage:"Enable authentication" default:"false"`
	ForceEnableBootstrap       bool     `usage:"Enables the bootstrap user even if other admin users have been created" default:"false"`
	AuthAdminEmails            []string `usage:"Emails of admin users"`
	AgentsDir                  string   `usage:"The directory to auto load agents on start (default $XDG_CONFIG_HOME/.obot/agents)"`
	StaticDir                  string   `usage:"The directory to serve static files from"`

	// Sendgrid webhook
	SendgridWebhookUsername string `usage:"The username for the sendgrid webhook to authenticate with"`
	SendgridWebhookPassword string `usage:"The password for the sendgrid webhook to authenticate with"`

	GeminiConfig
	GatewayConfig
	EncryptionConfig
	OtelOptions
	AuditConfig
	services.Config
}

type Services struct {
	ToolRegistryURLs           []string
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
	Started                    chan struct{}
	GatewayServer              *gserver.Server
	GatewayClient              *client.Client
	ProxyManager               *proxy.Manager
	ProviderDispatcher         *dispatcher.Dispatcher
	Bootstrapper               *bootstrap.Bootstrap
	KnowledgeSetIngestionLimit int
	SupportDocker              bool
	AuthEnabled                bool
	AgentsDir                  string
	GeminiClient               *gemini.Client
	Otel                       *Otel
	AuditLogger                audit.Logger
	PostgresDSN                string

	// Use basic auth for sendgrid webhook, if being set
	SendgridWebhookUsername string
	SendgridWebhookPassword string
}

const (
	datasetTool   = "github.com/gptscript-ai/datasets"
	workspaceTool = "github.com/gptscript-ai/workspace-provider"
)

var requiredEnvs = []string{
	// Standard system stuff
	"PATH", "HOME", "USER", "PWD",
	// Embedded env vars
	"OBOT_BIN", "GPTSCRIPT_BIN", "GPTSCRIPT_EMBEDDED",
	// Encryption,
	"GPTSCRIPT_ENCRYPTION_CONFIG_FILE",
	// XDG stuff
	"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
	// Don't migrate DB
	"OBOT_TOOLS_MIGRATE_DB",
}

func copyKeys(envs []string) []string {
	seen := make(map[string]struct{})
	newEnvs := make([]string, len(envs))

	for _, env := range append(envs, requiredEnvs...) {
		if env == "*" {
			return os.Environ()
		}
		if _, ok := seen[env]; ok {
			continue
		}
		v := os.Getenv(env)
		if v == "" {
			continue
		}
		seen[env] = struct{}{}
		newEnvs = append(newEnvs, fmt.Sprintf("%s=%s", env, os.Getenv(env)))
	}

	sort.Strings(newEnvs)
	return newEnvs
}

func newGPTScript(ctx context.Context,
	envPassThrough []string,
	credStore string,
	credStoreEnv []string,
) (*gptscript.GPTScript, error) {
	if os.Getenv("GPTSCRIPT_URL") != "" {
		return gptscript.NewGPTScript(gptscript.GlobalOptions{
			URL:           os.Getenv("GPTSCRIPT_URL"),
			WorkspaceTool: workspaceTool,
			DatasetTool:   datasetTool,
		})
	}

	// Don't migrate the DB in the cred tools
	os.Setenv("OBOT_TOOLS_MIGRATE_DB", "false")

	credOverrides := strings.Split(os.Getenv("GPTSCRIPT_CREDENTIAL_OVERRIDE"), ",")
	if len(credOverrides) == 1 && strings.TrimSpace(credOverrides[0]) == "" {
		credOverrides = nil
	}
	url, err := sdkserver.EmbeddedStart(ctx, sdkserver.Options{
		Options: gptscriptai.Options{
			Env: copyKeys(envPassThrough),
			Cache: cache.Options{
				CacheDir: os.Getenv("GPTSCRIPT_CACHE_DIR"),
			},
			Runner: runner.Options{
				CredentialOverrides: credOverrides,
			},
			SystemToolsDir:     os.Getenv("GPTSCRIPT_SYSTEM_TOOLS_DIR"),
			CredentialStore:    credStore,
			CredentialToolsEnv: append(copyKeys(envPassThrough), credStoreEnv...),
		},
		DatasetTool:   datasetTool,
		WorkspaceTool: workspaceTool,
	})
	if err != nil {
		return nil, err
	}

	if err := os.Setenv("GPTSCRIPT_URL", url); err != nil {
		return nil, err
	}

	if os.Getenv("WORKSPACE_PROVIDER_DATA_HOME") == "" {
		if err = os.Setenv("WORKSPACE_PROVIDER_DATA_HOME", filepath.Join(xdg.DataHome, "obot", "workspace-provider")); err != nil {
			return nil, err
		}
	}

	return gptscript.NewGPTScript(gptscript.GlobalOptions{
		Env:           copyKeys(envPassThrough),
		URL:           url,
		WorkspaceTool: workspaceTool,
		DatasetTool:   datasetTool,
	})
}

func New(ctx context.Context, config Config) (*Services, error) {
	// Setup Otel first so other services can use it.
	otel, err := newOtel(ctx, config.OtelOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to bootstrap OTel SDK: %w", err)
	}

	system.SetBinToSelf()

	devPort, config := configureDevMode(config)

	// Just a common mistake where you put the wrong prefix for the DSN. This seems to be inconsistent across things
	// that use postgres
	config.DSN = strings.Replace(config.DSN, "postgresql://", "postgres://", 1)

	if len(config.ToolRegistries) < 1 {
		config.ToolRegistries = []string{"github.com/obot-platform/tools"}
	}

	storageClient, restConfig, dbAccess, err := storage.Start(ctx, config.Config)
	if err != nil {
		return nil, err
	}

	// For now, always auto-migrate.
	gatewayDB, err := db.New(dbAccess.DB, dbAccess.SQLDB, true)
	if err != nil {
		return nil, err
	}
	// Important: the database needs to be auto-migrated before we create the cred store, so that
	// the gptscript_credentials table is available.
	if err := gatewayDB.AutoMigrate(); err != nil {
		return nil, err
	}

	encryptionConfig, encryptionConfigFile, err := encryption.Init(ctx, encryption.Options(config.EncryptionConfig))
	if err != nil {
		return nil, err
	}

	credStore, credStoreEnv, err := credstores.Init(config.ToolRegistries, config.DSN, encryptionConfigFile)
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

	gptscriptClient, err := newGPTScript(ctx, config.EnvKeys, credStore, credStoreEnv)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(config.DSN, "postgres://") {
		if err := gptscriptClient.CreateCredential(ctx, gptscript.Credential{
			Context:  system.DefaultNamespace,
			ToolName: system.KnowledgeCredID,
			Type:     gptscript.CredentialTypeTool,
			Env: map[string]string{
				"KNOW_VECTOR_DSN": strings.Replace(config.DSN, "postgres://", "pgvector://", 1),
				"KNOW_INDEX_DSN":  config.DSN,
			},
		}); err != nil {
			return nil, err
		}
	} else {
		if err := gptscriptClient.DeleteCredential(ctx, system.DefaultNamespace, system.KnowledgeCredID); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return nil, err
		}
	}

	r, err := nah.NewRouter("obot-controller", &nah.Options{
		RESTConfig:     restConfig,
		Scheme:         scheme.Scheme,
		ElectionConfig: leader.NewDefaultElectionConfig("", "obot-controller", restConfig),
		HealthzPort:    -1,
		GVKThreadiness: map[schema.GroupVersionKind]int{
			v1.SchemeGroupVersion.WithKind("KnowledgeFile"): config.KnowledgeFileWorkers,
		},
		GVKQueueSplitters: map[schema.GroupVersionKind]runtime.WorkerQueueSplitter{
			v1.SchemeGroupVersion.WithKind("Run"): (*runQueueSplitter)(nil),
		},
	})
	if err != nil {
		return nil, err
	}

	apply.AddValidOwnerChange("otto-controller", "obot-controller")

	var postgresDSN string
	if strings.HasPrefix(config.DSN, "postgres://") {
		postgresDSN = config.DSN
	}

	var (
		tokenServer   = &jwt.TokenService{}
		gatewayClient = client.New(gatewayDB, encryptionConfig, config.AuthAdminEmails)
		events        = events.NewEmitter(storageClient, gatewayClient)
		invoker       = invoke.NewInvoker(
			storageClient,
			gptscriptClient,
			gatewayClient,
			config.Hostname,
			config.HTTPListenPort,
			tokenServer,
			events,
		)
		providerDispatcher = dispatcher.New(ctx, invoker, storageClient, gptscriptClient, postgresDSN)

		proxyManager *proxy.Manager
	)

	bootstrapper, err := bootstrap.New(ctx, config.Hostname, gatewayClient, gptscriptClient, config.EnableAuthentication, config.ForceEnableBootstrap)
	if err != nil {
		return nil, err
	}

	gatewayServer, err := gserver.New(
		ctx,
		storageClient,
		gptscriptClient,
		gatewayDB,
		tokenServer,
		providerDispatcher,
		encryptionConfig,
		config.AuthAdminEmails,
		gserver.Options(config.GatewayConfig),
	)
	if err != nil {
		return nil, err
	}

	var authenticators authenticator.Request = gatewayServer
	if config.EnableAuthentication {
		proxyManager = proxy.NewProxyManager(ctx, providerDispatcher)

		// Token Auth + OAuth auth
		authenticators = union.New(authenticators, proxyManager)
		// Add gateway user info
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)
		// Add token auth
		authenticators = union.New(authenticators, tokenServer)
		// Add bootstrap auth
		authenticators = union.New(authenticators, bootstrapper)
		// Add anonymous user authenticator
		authenticators = union.New(authenticators, authn.Anonymous{})

		// Clean up "nobody" user from previous "Authentication Disabled" runs.
		// This reduces the chance that someone could authenticate as "nobody" and get admin access once authentication
		// is enabled.
		if err := gatewayClient.RemoveIdentity(ctx, &types.Identity{
			ProviderUsername: "nobody",
			ProviderUserID:   "nobody",
		}); err != nil {
			return nil, fmt.Errorf(`failed to remove "nobody" user and identity from database: %w`, err)
		}
	} else {
		// "Authentication Disabled" flow

		// Add gateway user info if token auth worked
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)

		// Add no auth authenticator
		authenticators = union.New(authenticators, authn.NewNoAuth(gatewayClient))
	}

	if config.EmailServerName != "" && config.EnableSMTPServer {
		go smtp.Start(ctx, storageClient, config.EmailServerName)
	}

	var geminiClient *gemini.Client
	if config.GeminiAPIKey != "" {
		// Enable gemini-powered image generation
		geminiClient, err = gemini.NewClient(ctx, gemini.Config{
			GeminiAPIKey:               config.GeminiAPIKey,
			GeminiImageGenerationModel: config.GeminiImageGenerationModel,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create gemini client: %w", err)
		}
	}

	run, err := gptscriptClient.Run(ctx, fmt.Sprintf("Validate Environment Variables from %s", workspaceTool), gptscript.Options{
		Input: fmt.Sprintf(`{"provider":"%s"}`, config.WorkspaceProviderType),
		GlobalOptions: gptscript.GlobalOptions{
			Env: os.Environ(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to validate environment variables: %w", err)
	}

	_, err = run.Text()
	if err != nil {
		return nil, fmt.Errorf("failed to validate environment variables: %w", err)
	}

	auditLogger, err := audit.New(ctx, audit.Options(config.AuditConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}

	// For now, always auto-migrate the gateway database
	return &Services{
		WorkspaceProviderType: config.WorkspaceProviderType,
		ServerURL:             config.Hostname,
		DevUIPort:             devPort,
		ToolRegistryURLs:      config.ToolRegistries,
		Events:                events,
		StorageClient:         storageClient,
		Router:                r,
		GPTClient:             gptscriptClient,
		APIServer: server.NewServer(
			storageClient,
			gatewayClient,
			gptscriptClient,
			authn.NewAuthenticator(authenticators),
			authz.NewAuthorizer(r.Backend(), config.DevMode),
			proxyManager,
			auditLogger,
			config.Hostname,
		),
		TokenServer:                tokenServer,
		Invoker:                    invoker,
		GatewayServer:              gatewayServer,
		GatewayClient:              gatewayClient,
		KnowledgeSetIngestionLimit: config.KnowledgeSetIngestionLimit,
		EmailServerName:            config.EmailServerName,
		SupportDocker:              config.Docker,
		AuthEnabled:                config.EnableAuthentication,
		SendgridWebhookUsername:    config.SendgridWebhookUsername,
		SendgridWebhookPassword:    config.SendgridWebhookPassword,
		ProxyManager:               proxyManager,
		ProviderDispatcher:         providerDispatcher,
		Bootstrapper:               bootstrapper,
		AgentsDir:                  config.AgentsDir,
		GeminiClient:               geminiClient,
		Otel:                       otel,
		AuditLogger:                auditLogger,
		PostgresDSN:                postgresDSN,
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
	_ = os.Setenv("NAH_DEV_MODE", "true")
	_ = os.Setenv("WORKSPACE_PROVIDER_IGNORE_WORKSPACE_NOT_FOUND", "true")
	return config.DevUIPort, config
}

func startDevMode(ctx context.Context, storageClient storage.Client) {
	_ = storageClient.Delete(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obot-controller",
			Namespace: "kube-system",
		},
	})
}
