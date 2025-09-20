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
	"time"

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
	apiclienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
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
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/jwt/ephemeral"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
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
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/client-go/rest"
	gocache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	// Setup nah logging
	_ "github.com/obot-platform/nah/pkg/logrus"
)

type (
	GatewayConfig     gserver.Options
	GeminiConfig      gemini.Config
	AuditConfig       audit.Options
	RateLimiterConfig ratelimiter.Options
	EncryptionConfig  encryption.Options
	MCPConfig         mcp.Options
)

type Config struct {
	HTTPListenPort             int      `usage:"HTTP port to listen on" default:"8080" name:"http-listen-port"`
	DevMode                    bool     `usage:"Enable development mode" default:"false" name:"dev-mode" env:"OBOT_DEV_MODE"`
	DevUIPort                  int      `usage:"The port on localhost running the dev instance of the UI" default:"5173"`
	UserUIPort                 int      `usage:"The port on localhost running the user production instance of the UI" env:"OBOT_SERVER_USER_UI_PORT"`
	AllowedOrigin              string   `usage:"Allowed origin for CORS"`
	ToolRegistries             []string `usage:"The remote tool references to the set of gptscript tool registries to use" default:"github.com/obot-platform/tools"`
	WorkspaceProviderType      string   `usage:"The type of workspace provider to use for non-knowledge workspaces" default:"directory" env:"OBOT_WORKSPACE_PROVIDER_TYPE"`
	HelperModel                string   `usage:"The model used to generate names and descriptions" default:"gpt-4.1-mini"`
	EmailServerName            string   `usage:"The name of the email server to display for email receivers"`
	EnableSMTPServer           bool     `usage:"Enable SMTP server to receive emails" default:"false" env:"OBOT_ENABLE_SMTP_SERVER"`
	Docker                     bool     `usage:"Enable Docker support" default:"false" env:"OBOT_DOCKER"`
	EnvKeys                    []string `usage:"The environment keys to pass through to the GPTScript server" env:"OBOT_ENV_KEYS"`
	KnowledgeSetIngestionLimit int      `usage:"The maximum number of files to ingest into a knowledge set" default:"3000" name:"knowledge-set-ingestion-limit"`
	KnowledgeFileWorkers       int      `usage:"The number of workers to process knowledge files" default:"5"`
	RunWorkers                 int      `usage:"The number of workers to process runs" default:"1000"`
	ElectionFile               string   `usage:"Use this file for leader election instead of database leases"`
	EnableAuthentication       bool     `usage:"Enable authentication" default:"false"`
	ForceEnableBootstrap       bool     `usage:"Enables the bootstrap user even if other admin users have been created" default:"false"`
	AuthAdminEmails            []string `usage:"Emails of admin users"`
	AuthOwnerEmails            []string `usage:"Emails of owner users"`
	AgentsDir                  string   `usage:"The directory to auto load agents on start (default $XDG_CONFIG_HOME/.obot/agents)"`
	StaticDir                  string   `usage:"The directory to serve static files from"`
	RetentionPolicyHours       int      `usage:"The retention policy for the system. Set to 0 to disable retention." default:"2160"` // default 90 days
	DefaultMCPCatalogPath      string   `usage:"The path to the default MCP catalog (accessible to all users)" default:""`
	// Sendgrid webhook
	SendgridWebhookUsername           string `usage:"The username for the sendgrid webhook to authenticate with"`
	SendgridWebhookPassword           string `usage:"The password for the sendgrid webhook to authenticate with"`
	MCPAuditLogPersistIntervalSeconds int    `usage:"The interval in seconds to persist MCP audit logs to the database" default:"5"`
	MCPAuditLogsPersistBatchSize      int    `usage:"The number of MCP audit logs to persist in a single batch" default:"1000"`

	GeminiConfig
	GatewayConfig
	EncryptionConfig
	OtelOptions
	AuditConfig
	RateLimiterConfig
	MCPConfig
	services.Config
}

type Services struct {
	ToolRegistryURLs           []string
	WorkspaceProviderType      string
	ServerURL                  string
	EmailServerName            string
	DevUIPort                  int
	UserUIPort                 int
	Events                     *events.Emitter
	StorageClient              storage.Client
	Router                     *router.Router
	GPTClient                  *gptscript.GPTScript
	Invoker                    *invoke.Invoker
	EphemeralTokenServer       *ephemeral.TokenService
	PersistentTokenServer      *persistent.TokenService
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
	DefaultMCPCatalogPath      string
	AgentsDir                  string
	GeminiClient               *gemini.Client
	Otel                       *Otel
	AuditLogger                audit.Logger
	PostgresDSN                string
	RetentionPolicy            time.Duration
	// Use basic auth for sendgrid webhook, if being set
	SendgridWebhookUsername string
	SendgridWebhookPassword string

	// Used for indexed lookups of access control rules.
	AccessControlRuleHelper *accesscontrolrule.Helper

	WebhookHelper *mcp.WebhookHelper

	// Used for loading and running MCP servers with GPTScript.
	MCPLoader *mcp.SessionManager

	// Global token storage client for MCP OAuth
	MCPOAuthTokenStorage mcp.GlobalTokenStore

	// OAuth configuration
	OAuthServerConfig OAuthAuthorizationServerConfig

	// Local Kubernetes configuration for deployment monitoring
	LocalK8sConfig     *rest.Config
	MCPServerNamespace string
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

// buildLocalK8sConfig creates a Kubernetes config for local cluster access
func buildLocalK8sConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	if k := os.Getenv("KUBECONFIG"); k != "" {
		kubeconfig = k
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func newGPTScript(ctx context.Context,
	envPassThrough []string,
	credStore string,
	credStoreEnv []string,
	mcpSessionManager *mcp.SessionManager,
) (*gptscript.GPTScript, error) {
	if os.Getenv("GPTSCRIPT_URL") != "" {
		return gptscript.NewGPTScript(gptscript.GlobalOptions{
			URL:           os.Getenv("GPTSCRIPT_URL"),
			WorkspaceTool: workspaceTool,
			DatasetTool:   datasetTool,
		})
	}

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
				MCPRunner:           mcpSessionManager,
			},
			SystemToolsDir:     os.Getenv("GPTSCRIPT_SYSTEM_TOOLS_DIR"),
			CredentialStore:    credStore,
			CredentialToolsEnv: append(copyKeys(envPassThrough), credStoreEnv...),
		},
		DatasetTool:   datasetTool,
		WorkspaceTool: workspaceTool,
		MCPLoader:     mcpSessionManager,
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

	var electionConfig *leader.ElectionConfig
	if config.ElectionFile != "" {
		electionConfig = leader.NewFileElectionConfig(config.ElectionFile)
	} else {
		electionConfig = leader.NewDefaultElectionConfig("", "obot-controller", restConfig)
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
		config.Hostname = fmt.Sprintf("http://localhost:%d", config.HTTPListenPort)
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

	gatewayClient := client.New(
		ctx,
		gatewayDB,
		storageClient,
		encryptionConfig,
		config.AuthOwnerEmails,
		config.AuthAdminEmails,
		time.Duration(config.MCPAuditLogPersistIntervalSeconds)*time.Second,
		config.MCPAuditLogsPersistBatchSize)
	mcpOAuthTokenStorage := mcpgateway.NewGlobalTokenStore(gatewayClient)

	// Build local Kubernetes config for deployment monitoring (optional)
	var localK8sConfig *rest.Config
	if config.MCPRuntimeBackend == "kubernetes" {
		localK8sConfig, err = buildLocalK8sConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build local Kubernetes config: %w", err)
		}
	}

	mcpLoader, err := mcp.NewSessionManager(ctx, mcpOAuthTokenStorage, config.Hostname, mcp.Options(config.MCPConfig), localK8sConfig)
	if err != nil {
		return nil, err
	}

	gptscriptClient, err := newGPTScript(ctx, config.EnvKeys, credStore, credStoreEnv, mcpLoader)
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
		ElectionConfig: electionConfig,
		HealthzPort:    -1,
		GVKThreadiness: map[schema.GroupVersionKind]int{
			v1.SchemeGroupVersion.WithKind("KnowledgeFile"): config.KnowledgeFileWorkers,
			v1.SchemeGroupVersion.WithKind("Run"):           config.RunWorkers,
		},
		GVKQueueSplitters: map[schema.GroupVersionKind]runtime.WorkerQueueSplitter{
			v1.SchemeGroupVersion.WithKind("Run"): (*runQueueSplitter)(nil),
		},
	})
	if err != nil {
		return nil, err
	}

	acrGVK, err := r.Backend().GroupVersionKindFor(&v1.AccessControlRule{})
	if err != nil {
		return nil, err
	}

	acrInformer, err := r.Backend().GetInformerForKind(ctx, acrGVK)
	if err != nil {
		return nil, err
	}

	if err = acrInformer.AddIndexers(map[string]gocache.IndexFunc{
		"user-ids": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, subject := range acr.Spec.Manifest.Subjects {
				if subject.Type == apiclienttypes.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		"catalog-entry-names": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.ResourceTypeMCPServerCatalogEntry {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"server-names": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.ResourceTypeMCPServer {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"selectors": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.ResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
	}); err != nil {
		return nil, err
	}

	acrHelper := accesscontrolrule.NewAccessControlRuleHelper(acrInformer.GetIndexer())

	// Set up MCPWebhookValidation indexer
	mcpWebhookValidationGVK, err := r.Backend().GroupVersionKindFor(&v1.MCPWebhookValidation{})
	if err != nil {
		return nil, err
	}

	mcpWebhookValidationInformer, err := r.Backend().GetInformerForKind(ctx, mcpWebhookValidationGVK)
	if err != nil {
		return nil, err
	}

	if err = mcpWebhookValidationInformer.AddIndexers(map[string]gocache.IndexFunc{
		"server-names": func(obj any) ([]string, error) {
			mcpWebhookValidation := obj.(*v1.MCPWebhookValidation)
			var results []string
			for _, resource := range mcpWebhookValidation.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.ResourceTypeMCPServer {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"selectors": func(obj any) ([]string, error) {
			mcpWebhookValidation := obj.(*v1.MCPWebhookValidation)
			var results []string
			for _, resource := range mcpWebhookValidation.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.ResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"catalog-entry-names": func(obj any) ([]string, error) {
			mcpWebhookValidation := obj.(*v1.MCPWebhookValidation)
			var results []string
			for _, resource := range mcpWebhookValidation.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.ResourceTypeMCPServerCatalogEntry {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"catalog-names": func(obj any) ([]string, error) {
			mcpWebhookValidation := obj.(*v1.MCPWebhookValidation)
			var results []string
			for _, resource := range mcpWebhookValidation.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.ResourceTypeMcpCatalog {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
	}); err != nil {
		return nil, err
	}

	apply.AddValidOwnerChange("otto-controller", "obot-controller")
	apply.AddValidOwnerChange("mcpcatalogentries", "catalog-default")

	var postgresDSN string
	if strings.HasPrefix(config.DSN, "postgres://") {
		postgresDSN = config.DSN
	}

	var (
		ephemeralTokenServer = &ephemeral.TokenService{}
		events               = events.NewEmitter(storageClient, gatewayClient)
		invoker              = invoke.NewInvoker(
			storageClient,
			gptscriptClient,
			gatewayClient,
			mcpLoader,
			config.Hostname,
			config.HTTPListenPort,
			ephemeralTokenServer,
			events,
		)
		providerDispatcher = dispatcher.New(ctx, invoker, storageClient, gptscriptClient, gatewayClient, postgresDSN)

		proxyManager *proxy.Manager
	)

	persistentTokenServer, err := persistent.NewTokenService(ctx, config.Hostname, gatewayClient, providerDispatcher, gptscriptClient)
	if err != nil {
		return nil, fmt.Errorf("failed to setup persistent token service: %w", err)
	}

	bootstrapper, err := bootstrap.New(ctx, config.Hostname, gatewayClient, gptscriptClient, config.EnableAuthentication, config.ForceEnableBootstrap)
	if err != nil {
		return nil, err
	}

	gatewayServer, err := gserver.New(ctx, gatewayDB, ephemeralTokenServer, providerDispatcher, gserver.Options(config.GatewayConfig))
	if err != nil {
		return nil, err
	}

	authenticators := gserver.NewGatewayTokenReviewer(gatewayClient, providerDispatcher)
	authenticators = union.New(authenticators, persistentTokenServer)
	if config.EnableAuthentication {
		proxyManager = proxy.NewProxyManager(ctx, providerDispatcher)

		// Token Auth + OAuth auth
		authenticators = union.New(authenticators, proxyManager)
		// Add gateway user info
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)
		// Add token auth
		authenticators = union.New(authenticators, ephemeralTokenServer)
		// Add bootstrap auth
		authenticators = union.New(authenticators, bootstrapper)
		if config.BearerToken != "" {
			// Add otel metrics auth
			authenticators = union.New(authenticators, authn.NewToken(config.BearerToken, "metrics", authz.MetricsGroup))
		}
		// Add anonymous user authenticator
		authenticators = union.New(authenticators, authn.Anonymous{})

		// Clean up "nobody" user from previous "Authentication Disabled" runs.
		// This reduces the chance that someone could authenticate as "nobody" and get admin access once authentication
		// is enabled.
		if err := gatewayClient.RemoveIdentityAndUser(ctx, &types.Identity{
			ProviderUsername:     "nobody",
			ProviderUserID:       "nobody",
			HashedProviderUserID: hash.String("nobody"),
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
		geminiClient, err = gemini.NewClient(ctx, gemini.Config(config.GeminiConfig))
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

	rateLimiter, err := ratelimiter.New(ratelimiter.Options(config.RateLimiterConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limiter: %w", err)
	}

	retentionPolicy := time.Duration(config.RetentionPolicyHours) * time.Hour

	// For now, always auto-migrate the gateway database
	return &Services{
		WorkspaceProviderType: config.WorkspaceProviderType,
		ServerURL:             config.Hostname,
		DevUIPort:             devPort,
		UserUIPort:            config.UserUIPort,
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
			authz.NewAuthorizer(r.Backend(), storageClient, config.DevMode, acrHelper),
			proxyManager,
			auditLogger,
			rateLimiter,
			config.Hostname,
		),
		EphemeralTokenServer:       ephemeralTokenServer,
		PersistentTokenServer:      persistentTokenServer,
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
		RetentionPolicy:            retentionPolicy,
		DefaultMCPCatalogPath:      config.DefaultMCPCatalogPath,
		MCPLoader:                  mcpLoader,
		MCPOAuthTokenStorage:       mcpOAuthTokenStorage,
		OAuthServerConfig: OAuthAuthorizationServerConfig{
			JWKSURI:                           config.Hostname + "/oauth/jwks.json",
			ResponseTypesSupported:            []string{"code"},
			GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
			CodeChallengeMethodsSupported:     []string{"S256", "plain"},
			TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "none"},
		},
		AccessControlRuleHelper: acrHelper,
		WebhookHelper:           mcp.NewWebhookHelper(mcpWebhookValidationInformer.GetIndexer()),
		LocalK8sConfig:          localK8sConfig,
		MCPServerNamespace:      config.MCPNamespace,
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
