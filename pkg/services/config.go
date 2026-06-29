package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/glebarez/sqlite"
	"github.com/obot-platform/nah"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/leader"
	"github.com/obot-platform/nah/pkg/router"
	apiclienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	"github.com/obot-platform/obot/pkg/bootstrap"
	"github.com/obot-platform/obot/pkg/encryption"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/db"
	gserver "github.com/obot-platform/obot/pkg/gateway/server"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	otime "github.com/obot-platform/obot/pkg/gateway/time"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/imagepullsecrets"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/logutil"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/messagepolicy"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/proxy"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageauthn "github.com/obot-platform/obot/pkg/storage/authn"
	"github.com/obot-platform/obot/pkg/storage/blob"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	gocache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	// Setup nah logging
	_ "github.com/obot-platform/nah/pkg/logrus"
)

var pkgLog = logger.Package()

type (
	GatewayConfig     gserver.Options
	AuditConfig       audit.Options
	RateLimiterConfig ratelimiter.Options
	EncryptionConfig  encryption.Options
	MCPConfig         mcp.Options
	LicenseConfig     license.Config
)

type MetricsAuthConfig struct {
	MetricsBearerToken string `usage:"Bearer token for metrics endpoint authentication" name:"metrics-bearer-token"`
}

type Config struct {
	HTTPListenPort       int      `usage:"HTTP port to listen on" default:"8080" name:"http-listen-port"`
	DevMode              bool     `usage:"Enable development mode" default:"false" name:"dev-mode" env:"OBOT_DEV_MODE"`
	DevUIPort            int      `usage:"The port on localhost running the dev instance of the UI" default:"5174"`
	UserUIPort           int      `usage:"The port on localhost running the user production instance of the UI" env:"OBOT_SERVER_USER_UI_PORT"`
	AllowedOrigin        string   `usage:"Allowed origin for CORS"`
	ProviderRegistries   []string `usage:"Local filesystem paths to provider registries (directories) to load providers from"`
	ElectionFile         string   `usage:"Use this file for leader election instead of database leases"`
	EnableAuthentication bool     `usage:"Enable authentication" default:"false"`
	ForceEnableBootstrap bool     `usage:"Enables the bootstrap user even if other admin users have been created" default:"false"`
	AuthAdminEmails      []string `usage:"Emails of admin users"`
	AuthOwnerEmails      []string `usage:"Emails of owner users"`
	StaticDir            string   `usage:"The directory to serve static files from"`

	DefaultMCPCatalogPath                string `usage:"The path to the default MCP catalog (accessible to all users)" default:""`
	DefaultSystemMCPCatalogPath          string `usage:"The path to the default System MCP catalog" default:""`
	DefaultSkillRepoURL                  string `usage:"The default skill repository URL (must be HTTPS GitHub URL)" default:"https://github.com/obot-platform/skills" env:"OBOT_DEFAULT_SKILL_REPO_URL"`
	DefaultSkillRepoRef                  string `usage:"The ref (branch/tag) for the default skill repository" default:"" env:"OBOT_DEFAULT_SKILL_REPO_REF"`
	ModelInfoSourceURL                   string `usage:"Authoritative URL for the model info (pricing) source synced into model costs; changes take effect on restart, empty disables it" default:"https://models.dev/api.json"`
	DisableUpdateCheck                   bool   `usage:"Disable Obot server update checks"`
	HideK8sDetails                       bool   `usage:"Hide Kubernetes configuration details such as the Server Scheduling page from the UI" default:"false"`
	EnableRegistryAuth                   bool   `usage:"Enable authentication for the MCP registry API" default:"false" env:"OBOT_SERVER_ENABLE_REGISTRY_AUTH"`
	EnableMessagePolicies                bool   `usage:"Enable message policies for LLM proxy content enforcement" default:"false"`
	EnableAgents                         *bool  `usage:"Enable Obot Agent features. When unset, agents are disabled for new deployments but grandfathered in for deployments that already have agents. Explicitly set to true to force-enable, or false to force-disable, regardless of grandfathering." env:"OBOT_ENABLE_AGENTS"`
	MCPOAuthClientExpiration             string `usage:"The expiration time in dynamically registered MCP OAuth clients, must be a valid duration string and may include days, hours, or minutes" default:"30d"`
	MCPServerSearchImage                 string `usage:"Container image for the obot MCP server" default:"ghcr.io/obot-platform/obot-mcp-server:v0.2.0"`
	NanobotAgentImage                    string `usage:"Container image for the Nanobot agent MCP server" default:"ghcr.io/obot-platform/nanobot-agent:v0.0.87"`
	MCPNetworkPolicyProviderChartRepo    string `usage:"Helm repository URL for the network policy provider chart"`
	MCPNetworkPolicyProviderChartName    string `usage:"Helm chart name for the network policy provider chart"`
	MCPNetworkPolicyProviderChartVersion string `usage:"Helm chart version for the network policy provider chart"`
	MCPNetworkPolicyProviderChartPath    string `usage:"Local filesystem path to the network policy provider chart"`
	MCPNetworkPolicyProviderValues       string `usage:"YAML or JSON values blob merged into the network policy provider chart values"`
	MCPDefaultDenyAllEgress              bool   `usage:"Default new MCP servers to deny all egress when network policy enforcement is enabled" default:"false"`

	// Published artifact storage
	ArtifactStorageProvider       string `usage:"Storage provider for published artifacts (s3, gcs, azure, custom)" name:"artifact-storage-provider" env:"OBOT_ARTIFACT_STORAGE_PROVIDER"`
	ArtifactStorageBucket         string `usage:"Bucket for published artifacts" name:"artifact-storage-bucket" env:"OBOT_ARTIFACT_STORAGE_BUCKET"`
	ArtifactS3Region              string `usage:"S3 region for artifact storage" name:"artifact-s3-region" env:"OBOT_ARTIFACT_S3_REGION"`
	ArtifactS3AccessKeyID         string `usage:"S3 access key ID for artifact storage" name:"artifact-s3-access-key-id" env:"OBOT_ARTIFACT_S3_ACCESS_KEY_ID"`
	ArtifactS3SecretAccessKey     string `usage:"S3 secret access key for artifact storage" name:"artifact-s3-secret-access-key" env:"OBOT_ARTIFACT_S3_SECRET_ACCESS_KEY"`
	ArtifactS3Endpoint            string `usage:"Custom S3 endpoint for artifact storage" name:"artifact-s3-endpoint" env:"OBOT_ARTIFACT_S3_ENDPOINT"`
	ArtifactGCSServiceAccountJSON string `usage:"GCS service account JSON for artifact storage (omit to use Application Default Credentials)" name:"artifact-gcs-service-account-json" env:"OBOT_ARTIFACT_GCS_SERVICE_ACCOUNT_JSON"`
	ArtifactAzureStorageAccount   string `usage:"Azure storage account name for artifact storage" name:"artifact-azure-storage-account" env:"OBOT_ARTIFACT_AZURE_STORAGE_ACCOUNT"`
	ArtifactAzureTenantID         string `usage:"Azure tenant ID for artifact storage" name:"artifact-azure-tenant-id" env:"OBOT_ARTIFACT_AZURE_TENANT_ID"`
	ArtifactAzureClientID         string `usage:"Azure client ID for artifact storage" name:"artifact-azure-client-id" env:"OBOT_ARTIFACT_AZURE_CLIENT_ID"`
	ArtifactAzureClientSecret     string `usage:"Azure client secret for artifact storage" name:"artifact-azure-client-secret" env:"OBOT_ARTIFACT_AZURE_CLIENT_SECRET"`

	GatewayConfig
	EncryptionConfig
	MetricsAuthConfig
	AuditConfig
	RateLimiterConfig
	MCPConfig
	LicenseConfig
	services.Config
}

type Services struct {
	EncryptionConfig      *encryptionconfig.EncryptionConfiguration
	ProviderRegistryPaths []string

	ServerURL             string
	InternalServerURL     string
	DevUIPort             int
	UserUIPort            int
	HTTPListenPort        int
	StorageClient         storage.Client
	Router                *router.Router
	PersistentTokenServer *persistent.TokenService
	APIServer             *server.Server

	GatewayServer               *gserver.Server
	GatewayClient               *client.Client
	ProxyManager                *proxy.Manager
	ProviderDispatcher          *dispatcher.Dispatcher
	Bootstrapper                *bootstrap.Bootstrap
	AuthEnabled                 bool
	DefaultMCPCatalogPath       string
	DefaultSystemMCPCatalogPath string
	DefaultSkillRepoURL         string
	DefaultSkillRepoRef         string
	ModelInfoSourceURL          string

	Otel        *Otel
	AuditLogger audit.Logger
	PostgresDSN string

	// Used for indexed lookups of access control rules.
	AccessControlRuleHelper *accesscontrolrule.Helper

	// Used for indexed lookups of model access policies.
	ModelAccessPolicyHelper *modelaccesspolicy.Helper

	// Used for indexed lookups of skill access rules.
	SkillAccessRuleHelper *skillaccessrule.Helper

	WebhookHelper *mcp.WebhookHelper

	MCPSessionManager *mcp.SessionManager

	// Global token storage client for MCP OAuth
	MCPOAuthTokenStorage mcp.GlobalTokenStore

	// OAuth configuration
	OAuthServerConfig              handlers.OAuthAuthorizationServerConfig
	MCPOAuthClientSecretExpiration time.Duration

	// LocalK8sClient is a kclient for the local Kubernetes cluster — the
	// cluster the obot pod runs in, where source Secrets for
	// secretBindings live. Nil on the docker backend.
	LocalK8sClient kclient.Client
	// LocalK8sConfig is the Kubernetes config for the MCP runtime cluster.
	LocalK8sConfig            *rest.Config
	LocalRouter               *router.Router
	MCPServerNamespace        string
	ServiceAccountIssuerURL   string
	ServiceAccountIssuerError string
	MCPClusterDomain          string
	ServiceName               string
	ServiceNamespace          string
	ServiceAccountName        string
	StorageListenPort         int

	// ObotNamespace is the Kubernetes namespace in which the obot server
	// runs; mcp.MergeBoundCreds reads source Secrets from here.
	ObotNamespace string

	// Parsed settings from Helm for k8s to pass to controller
	// PodSchedulingSettingsFromHelm contains affinity, tolerations, resources, runtimeClassName
	// when explicitly set via Helm. If non-nil, SetViaHelm=true and UI cannot modify these.
	PodSchedulingSettingsFromHelm *v1.K8sSettingsSpec
	// PSASettingsFromHelm contains Pod Security Admission settings, always sourced from
	// environment/Helm config and not modifiable via UI.
	PSASettingsFromHelm *v1.PodSecurityAdmissionSettings

	DisableUpdateCheck                   bool
	HideK8sDetails                       bool
	MCPRuntimeBackend                    string
	MCPSecretBindingAllowedLabel         string
	MCPImagePullSecrets                  []string
	MCPRemoteShimBaseImage               string
	MCPHTTPWebhookBaseImage              string
	RegistryNoAuth                       bool
	MessagePoliciesEnabled               bool
	EnableAgents                         *bool
	MCPNetworkPolicyEnabled              bool
	MCPDefaultDenyAllEgress              bool
	MCPServerSearchImage                 string
	NanobotAgentImage                    string
	MCPNetworkPolicyProviderChartRepo    string
	MCPNetworkPolicyProviderChartName    string
	MCPNetworkPolicyProviderChartVersion string
	MCPNetworkPolicyProviderChartPath    string
	MCPNetworkPolicyProviderValues       string
	SingleUserIdleServerShutdownInterval time.Duration
	MultiUserIdleServerShutdownInterval  time.Duration
	AgentIdleServerShutdownInterval      time.Duration

	// Published artifact blob storage
	ArtifactBlobStore  blob.BlobStore
	ArtifactBlobBucket string

	// License provider
	LicenseProvider *license.Provider
}

// BuildLocalK8sConfig creates a Kubernetes config for local cluster access
func BuildLocalK8sConfig() (*rest.Config, error) {
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

// unmarshalJSONStrict unmarshals JSON with strict validation that rejects unknown fields
func unmarshalJSONStrict(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// parsePSASettingsFromHelm parses Pod Security Admission settings from environment/Helm options.
// PSA settings are always managed via Helm/environment and cannot be modified via UI.
func parsePSASettingsFromHelm(opts mcp.Options) (*v1.PodSecurityAdmissionSettings, error) {
	// Check if any PSA options were explicitly set via Helm/environment
	hasPSASettings := opts.MCPPodSecurityEnabled ||
		opts.MCPPodSecurityEnforce != "" ||
		opts.MCPPodSecurityAudit != "" ||
		opts.MCPPodSecurityWarn != ""

	if !hasPSASettings {
		return nil, nil
	}

	// Validate PSA level values early to fail fast with clear error messages
	if opts.MCPPodSecurityEnforce != "" && !mcp.ValidatePSALevel(opts.MCPPodSecurityEnforce) {
		return nil, fmt.Errorf("invalid PSA enforce level %q: must be one of %v", opts.MCPPodSecurityEnforce, mcp.ValidPSALevels)
	}
	if opts.MCPPodSecurityAudit != "" && !mcp.ValidatePSALevel(opts.MCPPodSecurityAudit) {
		return nil, fmt.Errorf("invalid PSA audit level %q: must be one of %v", opts.MCPPodSecurityAudit, mcp.ValidPSALevels)
	}
	if opts.MCPPodSecurityWarn != "" && !mcp.ValidatePSALevel(opts.MCPPodSecurityWarn) {
		return nil, fmt.Errorf("invalid PSA warn level %q: must be one of %v", opts.MCPPodSecurityWarn, mcp.ValidPSALevels)
	}

	return &v1.PodSecurityAdmissionSettings{
		Enabled:        opts.MCPPodSecurityEnabled,
		Enforce:        opts.MCPPodSecurityEnforce,
		EnforceVersion: opts.MCPPodSecurityEnforceVersion,
		Audit:          opts.MCPPodSecurityAudit,
		AuditVersion:   opts.MCPPodSecurityAuditVersion,
		Warn:           opts.MCPPodSecurityWarn,
		WarnVersion:    opts.MCPPodSecurityWarnVersion,
	}, nil
}

// parsePodSchedulingSettingsFromHelm parses pod scheduling settings (affinity, tolerations, resources,
// runtimeClassName) from Helm options. These settings can be managed via Helm OR UI.
// If this returns non-nil, SetViaHelm will be true and UI cannot modify these settings.
func parsePodSchedulingSettingsFromHelm(opts mcp.Options) (*v1.K8sSettingsSpec, error) {
	hasPodSettings := (opts.MCPK8sSettingsAffinity != "" && opts.MCPK8sSettingsAffinity != "{}") ||
		(opts.MCPK8sSettingsTolerations != "" && opts.MCPK8sSettingsTolerations != "[]") ||
		(opts.MCPK8sSettingsResources != "" && opts.MCPK8sSettingsResources != "{}") ||
		(opts.MCPK8sSettingsNanobotAgentResources != "" && opts.MCPK8sSettingsNanobotAgentResources != "{}") ||
		opts.MCPK8sSettingsRuntimeClassName != "" ||
		opts.MCPK8sSettingsStorageClassName != "" ||
		opts.MCPK8sSettingsNanobotWorkspaceSize != ""

	if !hasPodSettings {
		return nil, nil
	}

	spec := &v1.K8sSettingsSpec{}

	if opts.MCPK8sSettingsAffinity != "" && opts.MCPK8sSettingsAffinity != "{}" {
		var affinity corev1.Affinity
		if err := unmarshalJSONStrict([]byte(opts.MCPK8sSettingsAffinity), &affinity); err != nil {
			return nil, fmt.Errorf("failed to parse affinity from Helm: %w", err)
		}
		spec.Affinity = &affinity
	}

	if opts.MCPK8sSettingsTolerations != "" && opts.MCPK8sSettingsTolerations != "[]" {
		var tolerations []corev1.Toleration
		if err := unmarshalJSONStrict([]byte(opts.MCPK8sSettingsTolerations), &tolerations); err != nil {
			return nil, fmt.Errorf("failed to parse tolerations from Helm: %w", err)
		}
		spec.Tolerations = tolerations
	}

	if opts.MCPK8sSettingsResources != "" && opts.MCPK8sSettingsResources != "{}" {
		var resources corev1.ResourceRequirements
		if err := unmarshalJSONStrict([]byte(opts.MCPK8sSettingsResources), &resources); err != nil {
			return nil, fmt.Errorf("failed to parse resources from Helm: %w", err)
		}
		spec.Resources = &resources
	}

	if opts.MCPK8sSettingsNanobotAgentResources != "" && opts.MCPK8sSettingsNanobotAgentResources != "{}" {
		var resources corev1.ResourceRequirements
		if err := unmarshalJSONStrict([]byte(opts.MCPK8sSettingsNanobotAgentResources), &resources); err != nil {
			return nil, fmt.Errorf("failed to parse nanobot agent resources from Helm: %w", err)
		}
		spec.NanobotAgentResources = &resources
	}

	if opts.MCPK8sSettingsRuntimeClassName != "" {
		spec.RuntimeClassName = &opts.MCPK8sSettingsRuntimeClassName
	}

	if opts.MCPK8sSettingsStorageClassName != "" {
		storageClassName := opts.MCPK8sSettingsStorageClassName
		spec.StorageClassName = &storageClassName
	}

	if opts.MCPK8sSettingsNanobotWorkspaceSize != "" {
		if _, err := resource.ParseQuantity(opts.MCPK8sSettingsNanobotWorkspaceSize); err != nil {
			return nil, fmt.Errorf("invalid nanobot workspace size from Helm: %w", err)
		}
		spec.NanobotWorkspaceSize = opts.MCPK8sSettingsNanobotWorkspaceSize
	}

	return spec, nil
}

func New(ctx context.Context, config Config) (*Services, error) {
	// Setup Otel first so other services can use it.
	otel, err := newOtel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to bootstrap OTel SDK: %w", err)
	}

	devPort, config := configureDevMode(config)

	// Just a common mistake where you put the wrong prefix for the DSN. This seems to be inconsistent across things
	// that use postgres
	config.DSN = strings.Replace(config.DSN, "postgresql://", "postgres://", 1)

	oauthClientExpiration, err := otime.ParseDuration(config.MCPOAuthClientExpiration)
	if err != nil {
		return nil, fmt.Errorf("invalid MCP OAuth client expiration: %w", err)
	}
	if oauthClientExpiration < time.Minute {
		return nil, fmt.Errorf("invalid MCP OAuth client expiration: must be at least 1 minute")
	}

	runtimeIsK8s := mcp.IsKubernetesBackend(config.MCPRuntimeBackend)
	if runtimeIsK8s && config.StorageListenPort == 0 {
		config.StorageListenPort = 8443
	}

	// Validate network policy provider configuration
	mcpNetworkPolicyEnabled := config.MCPNetworkPolicyProviderChartPath != "" || config.MCPNetworkPolicyProviderChartName != ""
	if mcpNetworkPolicyEnabled && !runtimeIsK8s {
		return nil, fmt.Errorf("network policy provider requires MCP runtime backend to be kubernetes")
	}
	if !mcpNetworkPolicyEnabled {
		config.MCPNetworkPolicyProviderChartRepo = ""
		config.MCPNetworkPolicyProviderChartName = ""
		config.MCPNetworkPolicyProviderChartVersion = ""
		config.MCPNetworkPolicyProviderChartPath = ""
		config.MCPNetworkPolicyProviderValues = ""
	} else {
		if config.MCPNetworkPolicyProviderChartPath != "" &&
			(config.MCPNetworkPolicyProviderChartRepo != "" ||
				config.MCPNetworkPolicyProviderChartName != "" ||
				config.MCPNetworkPolicyProviderChartVersion != "") {
			return nil, fmt.Errorf("network policy provider chart path cannot be combined with chart repo, name, or version")
		}
		if config.MCPNetworkPolicyProviderChartPath == "" && config.MCPNetworkPolicyProviderChartRepo == "" {
			return nil, fmt.Errorf("network policy provider requires chart repo when using a remote chart")
		}
	}

	// Sanitize DSN for logging (remove credentials)
	sanitizedDSN := logutil.SanitizeDSN(config.DSN)
	pkgLog.Infof("Connecting to database: dsn=%s", sanitizedDSN)
	storageClient, restConfig, dbAccess, storageServices, err := storage.Start(ctx, config.Config)
	if err != nil {
		pkgLog.Errorf("Failed to connect to database: dsn=%s error=%v", sanitizedDSN, err)
		return nil, err
	}
	pkgLog.Infof("Successfully connected to database: dsn=%s", sanitizedDSN)

	var electionConfig *leader.ElectionConfig
	if config.ElectionFile != "" {
		electionConfig = leader.NewFileElectionConfig(config.ElectionFile)
	} else {
		electionConfig = leader.NewDefaultElectionConfig("", "obot-controller", restConfig)
	}

	// For now, always auto-migrate.
	pkgLog.Infof("Initializing gateway database connection")
	gatewayDB, err := db.New(dbAccess.DB, dbAccess.SQLDB, true)
	if err != nil {
		pkgLog.Errorf("Failed to initialize gateway database: error=%v", err)
		return nil, err
	}
	pkgLog.Infof("Running database migrations")
	if err := gatewayDB.AutoMigrate(); err != nil {
		pkgLog.Errorf("Failed to run database migrations: error=%v", err)
		return nil, err
	}
	pkgLog.Infof("Database migrations completed successfully")

	encryptionConfig, err := encryption.Init(ctx, encryption.Options(config.EncryptionConfig))
	if err != nil {
		return nil, err
	}

	if config.DevMode {
		startDevMode(ctx, storageClient)
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
		config.MCPAuditLogsPersistBatchSize,
		config.MCPAuditLogRetentionDays,
	)

	if err := migrateGPTScriptCredentials(ctx, gatewayClient, gatewayDB, config.DSN); err != nil {
		return nil, fmt.Errorf("failed to migrate GPTScript credentials: %w", err)
	}
	if err := gatewayClient.MigrateToolReferenceCredentialContexts(ctx); err != nil {
		return nil, fmt.Errorf("failed to migrate ToolReference credential contexts: %w", err)
	}

	storageServices.Authn.SetServiceAccountValidator(func(ctx context.Context, token string) (string, error) {
		apiKey, err := gatewayClient.ValidateStorageServiceAccountToken(ctx, token)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", storageauthn.ErrInvalidServiceAccountToken
			}
			return "", err
		}
		account, ok := serviceaccounts.Get(apiKey.ServiceAccountName)
		if !ok || !serviceaccounts.Enabled(account, config.MCPRuntimeBackend, mcpNetworkPolicyEnabled) {
			return "", fmt.Errorf("%w: service account %q disabled for backend %q or network policy provider enabled=%t", storageauthn.ErrInvalidServiceAccountToken, apiKey.ServiceAccountName, config.MCPRuntimeBackend, mcpNetworkPolicyEnabled)
		}
		return apiKey.ServiceAccountName, nil
	})
	mcpOAuthTokenStorage := mcpgateway.NewGlobalTokenStore(gatewayClient)

	// Build local Kubernetes config for deployment monitoring (optional)
	var (
		localK8sConfig            *rest.Config
		serviceAccountIssuerURL   string
		serviceAccountIssuerError string
	)
	if mcp.IsKubernetesBackend(config.MCPRuntimeBackend) {
		localK8sConfig, err = BuildLocalK8sConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build local Kubernetes config: %w", err)
		}
		serviceAccountIssuerURL, err = imagepullsecrets.DiscoverServiceAccountIssuer(ctx, localK8sConfig)
		if err != nil {
			serviceAccountIssuerError = err.Error()
			pkgLog.Warnf("Failed to discover Kubernetes service account issuer URL: %v", err)
		}
	}

	// Parse Helm K8s settings - PSA settings and pod scheduling settings are handled separately
	// PSA settings are always sourced from Helm/environment and cannot be modified via UI
	psaSettings, err := parsePSASettingsFromHelm(mcp.Options(config.MCPConfig))
	if err != nil {
		return nil, err
	}
	// Pod scheduling settings (affinity, tolerations, resources, runtimeClassName) can be managed
	// via Helm OR UI. If set via Helm, SetViaHelm=true and UI cannot modify them.
	podSchedulingSettings, err := parsePodSchedulingSettingsFromHelm(mcp.Options(config.MCPConfig))
	if err != nil {
		return nil, err
	}

	var postgresDSN string
	if strings.HasPrefix(config.DSN, "postgres://") {
		postgresDSN = config.DSN
	}

	persistentTokenServer, err := persistent.NewTokenService(config.Hostname, gatewayClient)
	if err != nil {
		return nil, fmt.Errorf("failed to setup persistent token service: %w", err)
	}

	r, err := nah.NewRouter("obot-controller", &nah.Options{
		RESTConfig:     restConfig,
		Scheme:         scheme.Scheme,
		ElectionConfig: electionConfig,
		HealthzPort:    -1,
	})
	if err != nil {
		return nil, err
	}
	gatewayClient.SetMCPOAuthTokenTrigger(func(ctx context.Context, mcpID string) error {
		return r.Backend().Trigger(ctx, v1.SchemeGroupVersion.WithKind("MCPServer"), mcpID, 0)
	})

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

	var (
		apiLocalK8sClient kclient.WithWatch
		localCacheClient  kclient.WithWatch
		localRouter       *router.Router
	)
	if localK8sConfig != nil {
		apiLocalK8sClient, err = kclient.NewWithWatch(localK8sConfig, kclient.Options{Scheme: k8sscheme.Scheme})
		if err != nil {
			return nil, fmt.Errorf("failed to build local k8s client for API server: %w", err)
		}

		// Create a scheme that includes the types we need to watch
		localRouter, err = nah.NewRouter("obot-local-k8s", &nah.Options{
			RESTConfig: localK8sConfig,
			Scheme:     k8sscheme.Scheme,
			Namespace:  config.MCPNamespace,
			// The router is scoped to the MCP namespace, but the managed provider token
			// secret lives in Obot's runtime namespace.
			ByObject:       localK8sCacheByObject(config.MCPNamespace, config.ServiceNamespace),
			ElectionConfig: nil, // No leader election for local router
			HealthzPort:    -1,  // Disable healthz port
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create local Kubernetes router: %w", err)
		}

		localCacheClient = localRouter.Backend()
	}

	webhookHelper := mcp.NewWebhookHelper(mcpWebhookValidationInformer.GetIndexer(), config.Hostname)

	mcpSessionManager, err := mcp.NewSessionManager(ctx, config.EnableAuthentication, persistentTokenServer, config.Hostname, config.HTTPListenPort, mcp.Options(config.MCPConfig), webhookHelper, localK8sConfig, apiLocalK8sClient, localCacheClient, storageClient)
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

	acrHelper := accesscontrolrule.NewAccessControlRuleHelper(acrInformer.GetIndexer(), r.Backend())

	skillAccessRuleGVK, err := r.Backend().GroupVersionKindFor(&v1.SkillAccessRule{})
	if err != nil {
		return nil, err
	}

	skillAccessRuleInformer, err := r.Backend().GetInformerForKind(ctx, skillAccessRuleGVK)
	if err != nil {
		return nil, err
	}

	if err = skillAccessRuleInformer.AddIndexers(map[string]gocache.IndexFunc{
		skillaccessrule.SkillIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.SkillResourceTypeSkill {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		skillaccessrule.RepositoryIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.SkillResourceTypeSkillRepository {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		skillaccessrule.ResourceSelectorIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.SkillResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		skillaccessrule.UserIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == apiclienttypes.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		skillaccessrule.GroupIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == apiclienttypes.SubjectTypeGroup {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		skillaccessrule.SubjectSelectorIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == apiclienttypes.SubjectTypeSelector {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
	}); err != nil {
		return nil, err
	}

	skillAccessRuleHelper := skillaccessrule.NewHelper(skillAccessRuleInformer.GetIndexer())

	mapHelper, err := modelaccesspolicy.NewHelper(ctx, r.Backend())
	if err != nil {
		return nil, err
	}

	licenseProvider, err := license.NewProvider(ctx, gatewayClient, license.Config(config.LicenseConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create license provider: %w", err)
	}

	providerDispatcher := dispatcher.New(mcpSessionManager, storageClient, gatewayClient, licenseProvider, config.Hostname, fmt.Sprintf("http://localhost:%d", config.HTTPListenPort), postgresDSN)

	var msgPolicyHelper *messagepolicy.Helper
	if config.EnableMessagePolicies {
		msgPolicyHelper, err = messagepolicy.NewHelper(ctx, r.Backend(), storageClient, providerDispatcher, gatewayClient)
		if err != nil {
			return nil, err
		}
	}

	apply.AddValidOwnerChange("otto-controller", "obot-controller")
	apply.AddValidOwnerChange("mcpcatalogentries", "catalog-default")

	var proxyManager *proxy.Manager
	bootstrapper, err := bootstrap.New(ctx, config.Hostname, gatewayClient, providerDispatcher, config.EnableAuthentication, config.ForceEnableBootstrap)
	if err != nil {
		return nil, err
	}

	gatewayOpts := gserver.Options(config.GatewayConfig)
	gatewayServer, err := gserver.New(ctx, gatewayDB, persistentTokenServer, providerDispatcher, acrHelper, mapHelper, msgPolicyHelper, gatewayOpts)
	if err != nil {
		return nil, err
	}

	authenticators := gserver.NewGatewayTokenReviewer(gatewayClient, providerDispatcher)
	if config.EnableAuthentication {
		proxyManager = proxy.NewProxyManager(providerDispatcher)

		// Token Auth + OAuth auth
		authenticators = union.NewFailOnError(authenticators, proxyManager)
		// Add gateway user info
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)
		// API Key authentication (for MCP server access) - restricted to GroupAPIKey only
		// Must come after UserDecorator since it handles its own user lookup
		authenticators = union.New(authenticators, gserver.NewAPIKeyAuthenticator(gatewayClient))
		// Persistent Token Auth
		authenticators = union.New(authenticators, persistentTokenServer)
		// Add bootstrap auth
		authenticators = union.NewFailOnError(authenticators, bootstrapper)
		if config.MetricsBearerToken != "" {
			// Add metrics auth
			authenticators = union.New(authenticators, authn.NewToken(config.MetricsBearerToken, "metrics", authz.MetricsGroup))
		}
		// Add anonymous user authenticator
		authenticators = union.NewFailOnError(authenticators, authn.Anonymous{})

		// Clean up "nobody" user from previous "Authentication Disabled" runs.
		// This reduces the chance that someone could authenticate as "nobody" and get admin access once authentication
		// is enabled.
		if id, err := gatewayClient.RemoveIdentityAndUser(ctx, &types.Identity{
			ProviderUsername:     "nobody",
			ProviderUserID:       "nobody",
			HashedProviderUserID: hash.String("nobody"),
		}); err != nil {
			return nil, fmt.Errorf(`failed to remove "nobody" user and identity from database: %w`, err)
		} else if id != 0 {
			// Create this UserDelete object so that their stuff gets deleted.
			if err = storageClient.Create(ctx, &v1.UserDelete{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    system.DefaultNamespace,
					GenerateName: system.UserDeletePrefix,
				},
				Spec: v1.UserDeleteSpec{
					UserID: id,
				},
			}); err != nil {
				return nil, fmt.Errorf(`failed to create "nobody" user delete object: %w`, err)
			}
		}
	} else {
		// "Authentication Disabled" flow

		// Add gateway user info if token auth worked
		authenticators = client.NewUserDecorator(authenticators, gatewayClient)

		// Persistent Token Auth
		authenticators = union.New(authenticators, persistentTokenServer)

		// Add no auth authenticator
		authenticators = union.New(authenticators, authn.NewNoAuth(gatewayClient))
	}

	auditLogger, err := audit.New(ctx, audit.Options(config.AuditConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}

	rateLimiter, err := ratelimiter.New(ratelimiter.Options(config.RateLimiterConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limiter: %w", err)
	}

	// Derive registryNoAuth flag from config
	// When EnableRegistryAuth is false (default), registry is in no-auth mode
	registryNoAuth := !config.EnableRegistryAuth
	secretBindingAllowedLabel := strings.TrimSpace(config.MCPSecretBindingAllowedLabel)
	if errs := kvalidation.IsQualifiedName(secretBindingAllowedLabel); len(errs) > 0 {
		return nil, fmt.Errorf("invalid MCP secret binding allowed label %q: %s", secretBindingAllowedLabel, strings.Join(errs, "; "))
	}

	oauthServerConfig := handlers.OAuthAuthorizationServerConfig{
		Issuer:                            config.Hostname,
		AuthorizationEndpoint:             fmt.Sprintf("%s/oauth/authorize", config.Hostname),
		TokenEndpoint:                     fmt.Sprintf("%s/oauth/token", config.Hostname),
		RegistrationEndpoint:              fmt.Sprintf("%s/oauth/register", config.Hostname),
		JWKSURI:                           config.Hostname + "/oauth/jwks.json",
		ScopesSupported:                   []string{"profile"},
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:token-exchange"},
		CodeChallengeMethodsSupported:     []string{"S256", "plain"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "private_key_jwt", "none"},
		TokenEndpointAuthSigningAlgValuesSupported: []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"},
		UserInfoEndpoint:                  fmt.Sprintf("%s/oauth/userinfo", config.Hostname),
		ClientIDMetadataDocumentSupported: true,
	}

	// For now, always auto-migrate the gateway database
	svcs := &Services{
		EncryptionConfig:      encryptionConfig,
		ServerURL:             config.Hostname,
		InternalServerURL:     fmt.Sprintf("http://localhost:%d", config.HTTPListenPort),
		DevUIPort:             devPort,
		UserUIPort:            config.UserUIPort,
		HTTPListenPort:        config.HTTPListenPort,
		ProviderRegistryPaths: config.ProviderRegistries,
		StorageClient:         storageClient,
		Router:                r,
		ObotNamespace:         config.ServiceNamespace,
		APIServer: server.NewServer(
			storageClient,
			gatewayClient,
			apiLocalK8sClient,
			config.ServiceNamespace,
			authn.NewAuthenticator(authenticators),
			authz.NewAuthorizer(gatewayClient, r.Backend(), storageClient, config.DevMode, acrHelper, skillAccessRuleHelper, registryNoAuth),
			proxyManager,
			auditLogger,
			rateLimiter,
			config.Hostname,
			oauthServerConfig.ScopesSupported,
			registryNoAuth,
			licenseProvider,
		),
		PersistentTokenServer: persistentTokenServer,
		GatewayServer:         gatewayServer,
		GatewayClient:         gatewayClient,
		AuthEnabled:           config.EnableAuthentication,
		ProxyManager:          proxyManager,
		ProviderDispatcher:    providerDispatcher,
		Bootstrapper:          bootstrapper,

		Otel:        otel,
		AuditLogger: auditLogger,
		PostgresDSN: postgresDSN,

		DefaultMCPCatalogPath:          config.DefaultMCPCatalogPath,
		DefaultSystemMCPCatalogPath:    config.DefaultSystemMCPCatalogPath,
		DefaultSkillRepoURL:            config.DefaultSkillRepoURL,
		DefaultSkillRepoRef:            config.DefaultSkillRepoRef,
		ModelInfoSourceURL:             config.ModelInfoSourceURL,
		MCPSessionManager:              mcpSessionManager,
		MCPOAuthTokenStorage:           mcpOAuthTokenStorage,
		MCPOAuthClientSecretExpiration: oauthClientExpiration,
		OAuthServerConfig:              oauthServerConfig,
		AccessControlRuleHelper:        acrHelper,
		ModelAccessPolicyHelper:        mapHelper,

		SkillAccessRuleHelper:                skillAccessRuleHelper,
		WebhookHelper:                        webhookHelper,
		LocalK8sConfig:                       localK8sConfig,
		LocalK8sClient:                       apiLocalK8sClient,
		LocalRouter:                          localRouter,
		MCPServerNamespace:                   config.MCPNamespace,
		ServiceAccountIssuerURL:              serviceAccountIssuerURL,
		ServiceAccountIssuerError:            serviceAccountIssuerError,
		MCPClusterDomain:                     config.MCPClusterDomain,
		ServiceName:                          config.ServiceName,
		ServiceNamespace:                     config.ServiceNamespace,
		ServiceAccountName:                   config.ServiceAccountName,
		StorageListenPort:                    config.StorageListenPort,
		PodSchedulingSettingsFromHelm:        podSchedulingSettings,
		PSASettingsFromHelm:                  psaSettings,
		DisableUpdateCheck:                   config.DisableUpdateCheck,
		HideK8sDetails:                       config.HideK8sDetails,
		MCPRuntimeBackend:                    config.MCPRuntimeBackend,
		MCPSecretBindingAllowedLabel:         secretBindingAllowedLabel,
		MCPImagePullSecrets:                  config.MCPImagePullSecrets,
		MCPRemoteShimBaseImage:               config.MCPRemoteShimBaseImage,
		MCPHTTPWebhookBaseImage:              config.MCPHTTPWebhookBaseImage,
		SingleUserIdleServerShutdownInterval: time.Duration(config.SingleUserIdleServerShutdownHours) * time.Hour,
		MultiUserIdleServerShutdownInterval:  time.Duration(config.MultiUserIdleServerShutdownHours) * time.Hour,
		AgentIdleServerShutdownInterval:      time.Duration(config.IdleAgentShutdownHours) * time.Hour,
		RegistryNoAuth:                       registryNoAuth,
		MessagePoliciesEnabled:               config.EnableMessagePolicies,
		EnableAgents:                         config.EnableAgents,
		MCPNetworkPolicyEnabled:              mcpNetworkPolicyEnabled,
		MCPDefaultDenyAllEgress:              config.MCPDefaultDenyAllEgress,
		MCPServerSearchImage:                 config.MCPServerSearchImage,
		NanobotAgentImage:                    config.NanobotAgentImage,
		MCPNetworkPolicyProviderChartRepo:    config.MCPNetworkPolicyProviderChartRepo,
		MCPNetworkPolicyProviderChartName:    config.MCPNetworkPolicyProviderChartName,
		MCPNetworkPolicyProviderChartVersion: config.MCPNetworkPolicyProviderChartVersion,
		MCPNetworkPolicyProviderChartPath:    config.MCPNetworkPolicyProviderChartPath,
		MCPNetworkPolicyProviderValues:       config.MCPNetworkPolicyProviderValues,
		ArtifactBlobBucket:                   config.ArtifactStorageBucket,
		LicenseProvider:                      licenseProvider,
	}

	if (config.ArtifactStorageProvider == "") != (config.ArtifactStorageBucket == "") {
		return nil, fmt.Errorf("both OBOT_ARTIFACT_STORAGE_PROVIDER and OBOT_ARTIFACT_STORAGE_BUCKET must be set together")
	}

	if config.ArtifactStorageProvider != "" && config.ArtifactStorageBucket != "" {
		artifactStorageConfig := buildArtifactStorageConfig(config)
		artifactBlobStore, err := blob.New(apiclienttypes.StorageProviderType(config.ArtifactStorageProvider), artifactStorageConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create artifact blob store: %w", err)
		}
		if err := artifactBlobStore.Test(ctx); err != nil {
			return nil, fmt.Errorf("failed to validate artifact blob store: %w", err)
		}
		svcs.ArtifactBlobStore = artifactBlobStore
	} else {
		// Fallback: local directory storage when no cloud provider is configured.
		defaultDir := filepath.Join(xdg.DataHome, "obot", "published-artifacts")
		artifactBlobStore, err := blob.NewDirectoryStore(defaultDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create local artifact blob store: %w", err)
		}
		svcs.ArtifactBlobStore = artifactBlobStore
		svcs.ArtifactBlobBucket = "default"
	}

	return svcs, nil
}

func migrateGPTScriptCredentials(ctx context.Context, gatewayClient *client.Client, gatewayDB *db.DB, dsn string) error {
	if strings.HasPrefix(dsn, "postgres://") {
		return gatewayClient.MigrateGPTScriptCredentials(ctx, gatewayDB.WithContext(ctx))
	}

	if !strings.HasPrefix(dsn, "sqlite://") {
		return nil
	}

	dbFile, ok := strings.CutPrefix(dsn, "sqlite://file:")
	if !ok {
		return fmt.Errorf("invalid sqlite dsn, must start with sqlite://file: %s", dsn)
	}
	dbFile, _, _ = strings.Cut(dbFile, "?")

	if !strings.HasSuffix(dbFile, ".db") {
		return fmt.Errorf("invalid sqlite dsn, file must end in .db: %s", dsn)
	}

	credentialDBFile := strings.TrimSuffix(dbFile, ".db") + "-credentials.db"

	if _, err := os.Stat(credentialDBFile); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to stat GPTScript credential database %q: %w", credentialDBFile, err)
	}

	oldDB, err := gorm.Open(sqlite.Open(credentialDBFile), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open GPTScript credential database %q: %w", credentialDBFile, err)
	}
	sqlDB, err := oldDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get GPTScript credential database handle: %w", err)
	}
	defer sqlDB.Close()

	return gatewayClient.MigrateGPTScriptCredentials(ctx, oldDB)
}

func buildArtifactStorageConfig(config Config) apiclienttypes.StorageConfig {
	switch apiclienttypes.StorageProviderType(config.ArtifactStorageProvider) {
	case apiclienttypes.StorageProviderS3:
		return apiclienttypes.StorageConfig{
			S3Config: &apiclienttypes.S3Config{
				Region:          config.ArtifactS3Region,
				AccessKeyID:     config.ArtifactS3AccessKeyID,
				SecretAccessKey: config.ArtifactS3SecretAccessKey,
			},
		}
	case apiclienttypes.StorageProviderCustomS3:
		return apiclienttypes.StorageConfig{
			CustomS3Config: &apiclienttypes.CustomS3Config{
				Endpoint:        config.ArtifactS3Endpoint,
				Region:          config.ArtifactS3Region,
				AccessKeyID:     config.ArtifactS3AccessKeyID,
				SecretAccessKey: config.ArtifactS3SecretAccessKey,
			},
		}
	case apiclienttypes.StorageProviderGCS:
		return apiclienttypes.StorageConfig{
			GCSConfig: &apiclienttypes.GCSConfig{
				ServiceAccountJSON: config.ArtifactGCSServiceAccountJSON,
			},
		}
	case apiclienttypes.StorageProviderAzureBlob:
		return apiclienttypes.StorageConfig{
			AzureConfig: &apiclienttypes.AzureConfig{
				StorageAccount: config.ArtifactAzureStorageAccount,
				TenantID:       config.ArtifactAzureTenantID,
				ClientID:       config.ArtifactAzureClientID,
				ClientSecret:   config.ArtifactAzureClientSecret,
			},
		}
	default:
		return apiclienttypes.StorageConfig{}
	}
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

func localK8sCacheByObject(mcpServerNamespace, runtimeNamespace string) map[kclient.Object]crcache.ByObject {
	secretNamespaces := map[string]crcache.Config{}
	if mcpServerNamespace != "" {
		secretNamespaces[mcpServerNamespace] = crcache.Config{}
	}
	if runtimeNamespace != "" {
		secretNamespaces[runtimeNamespace] = crcache.Config{}
	}
	if len(secretNamespaces) == 0 {
		return nil
	}

	return map[kclient.Object]crcache.ByObject{
		&corev1.Secret{}: {
			Namespaces: secretNamespaces,
		},
	}
}
