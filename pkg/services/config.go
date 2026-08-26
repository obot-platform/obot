package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/agentbackend"
	agentbackendfake "github.com/obot-platform/obot/pkg/agentbackend/fake"
	agentbackendkubernetes "github.com/obot-platform/obot/pkg/agentbackend/kubernetes"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/handlers/agentconnect"
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
	"github.com/obot-platform/obot/pkg/hostedagentaccessrule"
	"github.com/obot-platform/obot/pkg/imagepullsecrets"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/logutil"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/messagepolicy"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/otel"
	"github.com/obot-platform/obot/pkg/proxy"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageauthn "github.com/obot-platform/obot/pkg/storage/authn"
	"github.com/obot-platform/obot/pkg/storage/blob"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tunnel"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
)

const (
	leaderElectionRequestTimeout = 15 * time.Second
)

type (
	GatewayConfig     gserver.Options
	AuditConfig       audit.Options
	RateLimiterConfig ratelimiter.Options
	EncryptionConfig  encryption.Options
	MCPConfig         mcp.Options
	LicenseConfig     license.Config
)

type Config struct {
	HTTPListenPort       int      `usage:"HTTP port to listen on" default:"8080" name:"http-listen-port"`
	AllowedOrigin        string   `usage:"Allowed origin for CORS"`
	ProviderRegistries   []string `usage:"Local filesystem paths to provider registries (directories) to load providers from"`
	EnableAuthentication bool     `usage:"Enable authentication" default:"false"`
	AuthAdminEmails      []string `usage:"Emails of admin users"`
	AuthOwnerEmails      []string `usage:"Emails of owner users"`
	TunnelPeerID         string   `usage:"Unique Pod UID of this Obot replica for tunnel peering"`
	TunnelPeerToken      string   `usage:"Shared internal credential for tunnel peering"`

	MCPOAuthClientExpiration       string   `usage:"The expiration time in dynamically registered MCP OAuth clients, must be a valid duration string and may include days, hours, or minutes" default:"30d"`
	MCPOAuthClientNativeExceptions []string `usage:"Additional Client ID Metadata Document URLs that default to the native application type when application_type is omitted"`
	ForceDynamicClient             bool     `usage:"Force Dynamic Client Registration for MCP OAuth instead of Client ID Metadata Documents"`

	DevMode              bool   `usage:"Enable development mode" default:"false" name:"dev-mode" env:"OBOT_DEV_MODE"`
	DevUIPort            int    `usage:"The port on localhost running the dev instance of the UI" default:"5174"`
	UserUIPort           int    `usage:"The port on localhost running the user production instance of the UI" env:"OBOT_SERVER_USER_UI_PORT"`
	ElectionFile         string `usage:"Use this file for leader election instead of database leases"`
	ForceEnableBootstrap bool   `usage:"Enables the bootstrap user even if other admin users have been created" default:"false"`
	MetricsBearerToken   string `usage:"Bearer token for metrics endpoint authentication" name:"metrics-bearer-token"`

	DefaultMCPCatalogPath                string `usage:"The path to the default MCP catalog (accessible to all users)" default:""`
	DefaultSystemMCPCatalogPath          string `usage:"The path to the default System MCP catalog" default:""`
	MDMAssetSource                       string `usage:"The source for MDM assets (a local directory, a tar archive path, or an HTTP(S) tarball URL)" default:"https://github.com/obot-platform/obot-sentry/releases/download/v0.1.6/mdm-assets.tar.gz" env:"OBOT_SERVER_MDM_ASSET_SOURCE"`
	DefaultSkillRepoURL                  string `usage:"The default skill repository URL (must be HTTPS GitHub URL)" default:"https://github.com/obot-platform/skills" env:"OBOT_DEFAULT_SKILL_REPO_URL"`
	DefaultSkillRepoRef                  string `usage:"The ref (branch/tag) for the default skill repository" default:"" env:"OBOT_DEFAULT_SKILL_REPO_REF"`
	DefaultHostedAgentsCatalogURL        string `usage:"The default hosted agent catalog repository URL (must be HTTPS)" default:"https://github.com/obot-platform/hosted-agents-catalog" env:"OBOT_DEFAULT_HOSTED_AGENTS_CATALOG_URL"`
	DefaultHostedAgentsCatalogRef        string `usage:"The ref (branch/tag) for the default hosted agent catalog repository" default:"" env:"OBOT_DEFAULT_HOSTED_AGENTS_CATALOG_REF"`
	ModelInfoSourceURL                   string `usage:"Authoritative URL for the model info (pricing) source synced into model costs; changes take effect on restart, empty disables it" default:"https://models.dev/api.json"`
	DisableUpdateCheck                   bool   `usage:"Disable Obot server update checks"`
	HideK8sDetails                       bool   `usage:"Hide Kubernetes configuration details such as the Server Scheduling page from the UI" default:"false"`
	EnableRegistryAuth                   bool   `usage:"Enable authentication for the MCP registry API" default:"false" env:"OBOT_SERVER_ENABLE_REGISTRY_AUTH"`
	EnableMessagePolicies                bool   `usage:"Enable message policies for LLM proxy content enforcement" default:"false"`
	LLMAuditLogRetentionDays             int    `usage:"Number of days to retain LLM audit logs (0 to disable cleanup)." default:"90"`
	DisableLLMAuditLog                   bool   `usage:"Disable LLM gateway audit logging" default:"false"`
	DeviceScanRetentionDays              int    `usage:"Number of days to retain submitted device scans (0 to disable cleanup)." default:"90"`
	EnableAgents                         *bool  `usage:"Enable Obot Agent features. When unset, agents are disabled for new deployments but grandfathered in for deployments that already have agents. Explicitly set to true to force-enable, or false to force-disable, regardless of grandfathering." env:"OBOT_ENABLE_AGENTS"`
	HostedAgentsBackend                  string `usage:"Hosted agent runtime backend (disabled, fake, or kubernetes). Defaults to the MCP runtime backend: kubernetes when MCP servers run on Kubernetes, and otherwise fake, since there is no docker agent backend." name:"hosted-agents-backend" env:"OBOT_HOSTED_AGENTS_BACKEND"`
	HostedAgentsStorageClassName         string `usage:"StorageClass for hosted agent pool volumes. It should use volumeBindingMode WaitForFirstConsumer, which is what keeps a pool on one node." name:"hosted-agents-storage-class-name"`
	HostedAgentsPodSecurityLevel         string `usage:"Pod Security Admission level enforced on the namespace hosted agent sandboxes run in (privileged, baseline, or restricted). Must match the namespace's own label or sandboxes are refused at admission. Empty means restricted." name:"hosted-agents-pod-security-level" env:"OBOT_HOSTED_AGENTS_POD_SECURITY_LEVEL"`
	HostedAgentsImagePullPolicy          string `usage:"Pull policy for hosted agent sandbox images (Always, IfNotPresent, Never)" default:"" name:"hosted-agents-image-pull-policy" env:"OBOT_HOSTED_AGENTS_IMAGE_PULL_POLICY"`
	HostedAgentsCleanupImage             string `usage:"Image used to erase a deleted sandbox's directory from its pool volume. Needs a shell and coreutils." name:"hosted-agents-cleanup-image" default:"busybox:1.36"`
	HostedAgentsRuntimeClassName         string `usage:"RuntimeClass for hosted agent deployments" name:"hosted-agents-runtime-class-name"`
	HostedAgentsAffinity                 string `usage:"Affinity rules for hosted agent pods (JSON)" name:"hosted-agents-affinity"`
	HostedAgentsTolerations              string `usage:"Tolerations for hosted agent pods (JSON)" name:"hosted-agents-tolerations"`
	HostedAgentsNodeSelector             string `usage:"Node selector for hosted agent pods (JSON)" name:"hosted-agents-node-selector"`
	MCPServerSearchImage                 string `usage:"Container image for the obot MCP server" default:"ghcr.io/obot-platform/obot-mcp-server:v0.2.0"`
	NanobotAgentImage                    string `usage:"Container image for the Nanobot agent MCP server" default:"ghcr.io/obot-platform/nanobot-agent:v0.0.92"`
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
	AuditConfig
	RateLimiterConfig
	MCPConfig
	LicenseConfig
	storageservices.Config
}

type Services struct {
	EncryptionConfig      *encryptionconfig.EncryptionConfiguration
	StorageClient         storage.Client
	Router                *router.Router
	PersistentTokenServer *persistent.TokenService
	APIServer             *server.Server
	GatewayClient         *client.Client
	ProxyManager          *proxy.Manager
	ProviderDispatcher    *dispatcher.Dispatcher
	Otel                  *otel.Otel
	AuditLogger           audit.Logger
	DSN                   string
	ServerURL             string
	// AgentServerURL is Obot's address as a sandbox can reach it, which differs
	// from ServerURL when Obot runs outside the cluster its sandboxes run in.
	AgentServerURL    string
	MCPSessionManager *mcp.SessionManager
	TunnelManager     *tunnel.Manager
	OAuthServerConfig handlers.OAuthAuthorizationServerConfig

	// Global token storage client for MCP OAuth
	MCPOAuthTokenStorage         mcp.GlobalTokenStore
	MCPSecretBindingAllowedLabel string
	RegistryNoAuth               bool

	PostgresDSN                   string
	ProviderRegistryPaths         []string
	DevUIPort                     int
	DevMode                       bool
	UserUIPort                    int
	GatewayServer                 *gserver.Server
	Bootstrapper                  *bootstrap.Bootstrap
	LocalAuthProvider             *localauth.Provider
	AuthEnabled                   bool
	DefaultMCPCatalogPath         string
	DefaultSystemMCPCatalogPath   string
	MDMAssetSource                string
	DefaultSkillRepoURL           string
	DefaultSkillRepoRef           string
	DefaultHostedAgentsCatalogURL string
	DefaultHostedAgentsCatalogRef string
	ModelInfoSourceURL            string

	// Used for indexed lookups of access control rules.
	AccessControlRuleHelper *accesscontrolrule.Helper

	// Used for indexed lookups of model access policies.
	ModelAccessPolicyHelper *modelaccesspolicy.Helper

	// Used for indexed lookups of skill access rules.
	SkillAccessRuleHelper *skillaccessrule.Helper

	// Used for indexed lookups of hosted agent access rules.
	HostedAgentAccessRuleHelper *hostedagentaccessrule.Helper

	MCPOAuthClientSecretExpiration time.Duration
	MCPOAuthClientNativeExceptions []string
	ForceDynamicClient             bool

	// LocalK8sClient is a kclient for the local Kubernetes cluster — the
	// cluster the obot pod runs in, where source Secrets for
	// secretBindings live. Nil on the docker backend.
	LocalK8sClient            kclient.Client
	LocalRouter               *router.Router
	EveryReplicaRouter        *router.Router
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

	DisableUpdateCheck      bool
	HideK8sDetails          bool
	MCPRuntimeBackend       string
	MCPImagePullSecrets     []string
	MCPHTTPWebhookBaseImage string
	MessagePoliciesEnabled  bool
	EnableAgents            *bool
	AgentBackend            agentbackend.Backend
	AgentBackendKind        string
	// AgentDevRouter reaches sandboxes from outside the cluster. It is set only
	// in development; in production Obot runs in-cluster and the sandbox address
	// resolves directly.
	AgentDevRouter                       agentconnect.DevRouter
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

type hostedAgentPodSchedulingSettings struct {
	Affinity     *corev1.Affinity
	Tolerations  []corev1.Toleration
	NodeSelector map[string]string
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

func parseHostedAgentPodSchedulingSettings(config Config) (hostedAgentPodSchedulingSettings, error) {
	var settings hostedAgentPodSchedulingSettings

	if config.HostedAgentsAffinity != "" && config.HostedAgentsAffinity != "{}" {
		var affinity corev1.Affinity
		if err := unmarshalJSONStrict([]byte(config.HostedAgentsAffinity), &affinity); err != nil {
			return settings, fmt.Errorf("failed to parse hosted agent affinity: %w", err)
		}
		settings.Affinity = &affinity
	}

	if config.HostedAgentsTolerations != "" && config.HostedAgentsTolerations != "[]" {
		if err := unmarshalJSONStrict([]byte(config.HostedAgentsTolerations), &settings.Tolerations); err != nil {
			return settings, fmt.Errorf("failed to parse hosted agent tolerations: %w", err)
		}
	}

	if config.HostedAgentsNodeSelector != "" && config.HostedAgentsNodeSelector != "{}" {
		if err := unmarshalJSONStrict([]byte(config.HostedAgentsNodeSelector), &settings.NodeSelector); err != nil {
			return settings, fmt.Errorf("failed to parse hosted agent node selector: %w", err)
		}
	}

	return settings, nil
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

// parsePodSchedulingSettingsFromHelm parses MCP pod scheduling settings (affinity, tolerations,
// resources, runtimeClassName, etc.) from Helm-populated mcp.Options env vars.
// If this returns non-nil, SetViaHelm will be true and the UI cannot modify these settings.
func parsePodSchedulingSettingsFromHelm(opts mcp.Options) (*v1.K8sSettingsSpec, error) {
	hasPodSettings := (opts.MCPK8sSettingsAffinity != "" && opts.MCPK8sSettingsAffinity != "{}") ||
		(opts.MCPK8sSettingsTolerations != "" && opts.MCPK8sSettingsTolerations != "[]") ||
		(opts.MCPK8sSettingsResources != "" && opts.MCPK8sSettingsResources != "{}") ||
		(opts.MCPK8sSettingsNanobotAgentResources != "" && opts.MCPK8sSettingsNanobotAgentResources != "{}") ||
		opts.MCPK8sSettingsRuntimeClassName != "" ||
		opts.MCPK8sSettingsStorageClassName != "" ||
		opts.MCPK8sSettingsNanobotWorkspaceSize != ""
	hasMaximums := opts.MCPK8sMaxCPURequest != "" ||
		opts.MCPK8sMaxCPULimit != "" ||
		opts.MCPK8sMaxMemoryRequest != "" ||
		opts.MCPK8sMaxMemoryLimit != ""

	if !hasPodSettings && !hasMaximums {
		return nil, nil
	}

	affinity, tolerations, resources, runtimeClassName, err := parsePodSchedulingJSONFields(
		opts.MCPK8sSettingsAffinity,
		opts.MCPK8sSettingsTolerations,
		opts.MCPK8sSettingsResources,
		opts.MCPK8sSettingsRuntimeClassName,
	)
	if err != nil {
		return nil, err
	}

	spec := &v1.K8sSettingsSpec{
		Affinity:           affinity,
		Tolerations:        tolerations,
		Resources:          resources,
		RuntimeClassName:   runtimeClassName,
		SetViaHelm:         hasPodSettings,
		MaximumsSetViaHelm: hasMaximums,
	}
	maximums, err := mcp.ParseResourceMaximums(opts)
	if err != nil {
		return nil, err
	}
	spec.MaxCPURequest = maximums.CPURequest
	spec.MaxCPULimit = maximums.CPULimit
	spec.MaxMemoryRequest = maximums.MemoryRequest
	spec.MaxMemoryLimit = maximums.MemoryLimit

	if opts.MCPK8sSettingsNanobotAgentResources != "" && opts.MCPK8sSettingsNanobotAgentResources != "{}" {
		var nanobotAgentResources corev1.ResourceRequirements
		if err := unmarshalJSONStrict([]byte(opts.MCPK8sSettingsNanobotAgentResources), &nanobotAgentResources); err != nil {
			return nil, fmt.Errorf("failed to parse nanobot agent resources from Helm: %w", err)
		}
		spec.NanobotAgentResources = &nanobotAgentResources
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

func parsePodSchedulingJSONFields(affinityJSON, tolerationsJSON, resourcesJSON, runtimeClassName string) (
	*corev1.Affinity,
	[]corev1.Toleration,
	*corev1.ResourceRequirements,
	*string,
	error,
) {
	var (
		affinity              *corev1.Affinity
		tolerations           []corev1.Toleration
		resources             *corev1.ResourceRequirements
		runtimeClassNameValue *string
	)

	if affinityJSON != "" && affinityJSON != "{}" {
		affinity = new(corev1.Affinity)
		if err := unmarshalJSONStrict([]byte(affinityJSON), affinity); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to parse affinity from Helm: %w", err)
		}
	}

	if tolerationsJSON != "" && tolerationsJSON != "[]" {
		if err := unmarshalJSONStrict([]byte(tolerationsJSON), &tolerations); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to parse tolerations from Helm: %w", err)
		}
	}

	if resourcesJSON != "" && resourcesJSON != "{}" {
		resources = new(corev1.ResourceRequirements)
		if err := unmarshalJSONStrict([]byte(resourcesJSON), resources); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to parse resources from Helm: %w", err)
		}
	}

	if runtimeClassName != "" {
		runtimeClassNameValue = &runtimeClassName
	}

	return affinity, tolerations, resources, runtimeClassNameValue, nil
}

func New(ctx context.Context, config Config) (*Services, error) {
	// Setup Otel first so other services can use it.
	otel, err := otel.New(ctx)
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
	slog.Info("Connecting to database", "dsn", sanitizedDSN)
	storageClient, restConfig, dbAccess, storageServices, err := storage.Start(ctx, storageservices.Config{
		StorageListenPort: config.StorageListenPort,
		StorageToken:      config.StorageToken,
		DSN:               config.DSN,
	})
	if err != nil {
		slog.Error("Failed to connect to database", "dsn", sanitizedDSN, "error", err)
		return nil, err
	}
	slog.Info("Successfully connected to database", "dsn", sanitizedDSN)

	// For now, always auto-migrate.
	slog.Info("Initializing gateway database connection")
	gatewayDB, err := db.New(dbAccess.DB, dbAccess.SQLDB, true)
	if err != nil {
		slog.Error("Failed to initialize gateway database", "error", err)
		return nil, err
	}
	slog.Info("Running database migrations")
	if err := gatewayDB.AutoMigrate(); err != nil {
		slog.Error("Failed to run database migrations", "error", err)
		return nil, err
	}
	slog.Info("Database migrations completed successfully")

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

	var electionConfig *leader.ElectionConfig
	if config.ElectionFile != "" {
		electionConfig = leader.NewFileElectionConfig(config.ElectionFile)
	} else {
		electionConfig = leader.NewDefaultElectionConfig("", "obot-controller", leaderElectionRESTConfig(restConfig))
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

	gatewayClient := client.New(
		ctx,
		gatewayDB,
		storageClient,
		encryptionConfig,
		func(ctx context.Context, mcpID string) error {
			return r.Backend().Trigger(ctx, v1.SchemeGroupVersion.WithKind("MCPServer"), mcpID, 0)
		},
		config.AuthOwnerEmails,
		config.AuthAdminEmails,
		time.Duration(config.MCPAuditLogPersistIntervalSeconds)*time.Second,
		config.MCPAuditLogsPersistBatchSize,
		config.MCPAuditLogRetentionDays,
		config.LLMAuditLogRetentionDays,
		config.DeviceScanRetentionDays,
		!config.DisableLLMAuditLog,
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

	// Build local Kubernetes config for deployment monitoring and, when Obot
	// has multiple replicas, EndpointSlice-based tunnel peer discovery.
	var (
		localK8sConfig            *rest.Config
		tunnelPeerConfig          tunnel.PeerConfig
		serviceAccountIssuerURL   string
		serviceAccountIssuerError string
	)
	// Hosted agents can require a cluster independently of how MCP servers run,
	// so either feature is reason enough to build a local config.
	if mcp.IsKubernetesBackend(config.MCPRuntimeBackend) || hostedAgentsNeedK8s(config) {
		localK8sConfig, err = BuildLocalK8sConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build local Kubernetes config: %w", err)
		}
	}
	if localK8sConfig != nil && mcp.IsKubernetesBackend(config.MCPRuntimeBackend) {
		// Image pull secret issuance is an MCP concern and has no bearing on
		// hosted agents.
		serviceAccountIssuerURL, err = imagepullsecrets.DiscoverServiceAccountIssuer(ctx, localK8sConfig)
		if err != nil {
			serviceAccountIssuerError = err.Error()
			slog.Warn("Failed to discover Kubernetes service account issuer URL", "error", err)
		}

		tunnelPeerConfig = tunnel.PeerConfig{
			ID:               strings.TrimSpace(config.TunnelPeerID),
			Token:            strings.TrimSpace(config.TunnelPeerToken),
			ServiceName:      strings.TrimSpace(config.ServiceName),
			ServiceNamespace: strings.TrimSpace(config.ServiceNamespace),
		}
		if err := tunnelPeerConfig.Validate(); err != nil {
			return nil, fmt.Errorf("invalid tunnel peer configuration: %w", err)
		}
	}

	tunnelManager, err := tunnel.NewManager(ctx, fmt.Sprintf("http://127.0.0.1:%d", config.HTTPListenPort), storageClient, tunnelPeerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tunnel manager: %w", err)
	}

	// Parse Helm K8s settings - PSA settings and pod scheduling settings are handled separately
	// PSA settings are always sourced from Helm/environment and cannot be modified via UI.
	var (
		psaSettings           *v1.PodSecurityAdmissionSettings
		podSchedulingSettings *v1.K8sSettingsSpec
	)
	if mcp.IsKubernetesBackend(config.MCPRuntimeBackend) {
		psaSettings, err = parsePSASettingsFromHelm(mcp.Options(config.MCPConfig))
		if err != nil {
			return nil, err
		}
		podSchedulingSettings, err = parsePodSchedulingSettingsFromHelm(mcp.Options(config.MCPConfig))
		if err != nil {
			return nil, err
		}
	}

	var postgresDSN string
	if strings.HasPrefix(config.DSN, "postgres://") {
		postgresDSN = config.DSN
	}

	persistentTokenServer, err := persistent.NewTokenService(config.Hostname, gatewayClient)
	if err != nil {
		return nil, fmt.Errorf("failed to setup persistent token service: %w", err)
	}

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
		tunnelPeerRouter  *router.Router
	)
	if mcp.IsKubernetesBackend(config.MCPRuntimeBackend) {
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
			ElectionConfig: leader.NewDefaultElectionConfig(config.MCPNamespace, "obot-local-controller", leaderElectionRESTConfig(localK8sConfig)),
			HealthzPort:    -1, // Disable healthz port
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create local Kubernetes router: %w", err)
		}

		localCacheClient = localRouter.Backend()
	}
	if tunnelManager.Enabled() {
		tunnelPeerRouter, err = nah.NewRouter("obot-tunnel-peers", &nah.Options{
			RESTConfig:     localK8sConfig,
			Scheme:         k8sscheme.Scheme,
			Namespace:      tunnelManager.ServiceNamespace,
			ElectionConfig: nil,
			HealthzPort:    -1,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create tunnel peer Kubernetes router: %w", err)
		}
	}

	webhookHelper := mcp.NewWebhookHelper(mcpWebhookValidationInformer.GetIndexer(), config.Hostname)

	mcpOAuthTokenStorage := mcp.NewGlobalTokenStore(gatewayClient)
	mcpSessionManager, err := mcp.NewSessionManager(ctx, config.EnableAuthentication, mcpOAuthTokenStorage, persistentTokenServer, config.Hostname, config.HTTPListenPort, mcp.Options(config.MCPConfig), webhookHelper, localK8sConfig, apiLocalK8sClient, localCacheClient, storageClient, gatewayClient, config.ServiceNamespace, tunnelManager)
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

	hostedAgentAccessRuleGVK, err := r.Backend().GroupVersionKindFor(&v1.HostedAgentAccessRule{})
	if err != nil {
		return nil, err
	}

	hostedAgentAccessRuleInformer, err := r.Backend().GetInformerForKind(ctx, hostedAgentAccessRuleGVK)
	if err != nil {
		return nil, err
	}

	if err = hostedAgentAccessRuleInformer.AddIndexers(map[string]gocache.IndexFunc{
		hostedagentaccessrule.HostedAgentIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.HostedAgentAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.HostedAgentResourceTypeHostedAgent {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		hostedagentaccessrule.ResourceSelectorIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.HostedAgentAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == apiclienttypes.HostedAgentResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		hostedagentaccessrule.UserIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.HostedAgentAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == apiclienttypes.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		hostedagentaccessrule.GroupIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.HostedAgentAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == apiclienttypes.SubjectTypeGroup {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		hostedagentaccessrule.SubjectSelectorIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.HostedAgentAccessRule)
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

	hostedAgentAccessRuleHelper := hostedagentaccessrule.NewHelper(hostedAgentAccessRuleInformer.GetIndexer())

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
	gatewayServer, err := gserver.New(gatewayDB, persistentTokenServer, providerDispatcher, acrHelper, mapHelper, msgPolicyHelper, gatewayOpts)
	if err != nil {
		return nil, err
	}

	authenticators := gserver.NewGatewayTokenReviewer(gatewayClient, providerDispatcher)
	var localAuthProvider *localauth.Provider
	if config.EnableAuthentication {
		proxyManager = proxy.NewProxyManager(providerDispatcher)

		// The local auth provider runs in-process, rather than as a daemon launched from the
		// provider registry, so that it can use Obot's own database for users and sessions.
		localAuthProvider, err = localauth.New(gatewayClient, config.Hostname)
		if err != nil {
			return nil, err
		}

		localAuthProviderURL, err := localAuthProvider.Start(ctx)
		if err != nil {
			return nil, err
		}
		providerDispatcher.RegisterBuiltinAuthProvider(system.DefaultNamespace, localauth.ProviderName, localAuthProviderURL)

		// Token Auth + OAuth auth
		authenticators = union.NewFailOnError(authenticators, proxyManager)
		// Add gateway user info
		authenticators = client.NewUserDecorator(authenticators, gatewayClient, licenseProvider)
		// Tunnel credentials are non-user principals and must be handled after
		// the user decorator. Authorization restricts them to tunnel setup only.
		authenticators = union.New(authenticators, tunnel.NewTunnelAuthenticator(storageClient))
		// API Key authentication (for MCP server access) - restricted to GroupAPIKey only
		// Must come after UserDecorator since it handles its own user lookup
		authenticators = union.New(authenticators, gserver.NewAPIKeyAuthenticator(gatewayClient, storageClient))
		// Device enrollment tokens (ode1-) and device access JWTs. Both yield
		// non-user principals, so they must come after the UserDecorator.
		authenticators = union.New(authenticators, gserver.NewDeviceEnrollmentAuthenticator(gatewayClient))
		authenticators = union.New(authenticators, gserver.NewDeviceAuthenticator(gatewayClient))
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
				Namespace:    system.DefaultNamespace,
				GenerateName: system.UserDeletePrefix,
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
		authenticators = client.NewUserDecorator(authenticators, gatewayClient, licenseProvider)

		// Tunnel authenticator
		authenticators = union.New(authenticators, tunnel.NewTunnelAuthenticator(storageClient))

		// Persistent Token Auth
		authenticators = union.New(authenticators, persistentTokenServer)

		// Add no auth authenticator
		authenticators = union.New(authenticators, authn.NewNoAuth(gatewayClient))
	}

	// Bridge requests use a non-user principal and can also carry the remote
	// server's Authorization header, so recognize them before all user-facing
	// authenticators and outside the user decorator.
	authenticators = union.New(tunnelManager, authenticators)

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
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256", "plain"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "private_key_jwt", "none"},
		TokenEndpointAuthSigningAlgValuesSupported: []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"},
		UserInfoEndpoint:                  fmt.Sprintf("%s/oauth/userinfo", config.Hostname),
		ClientIDMetadataDocumentSupported: true,
	}

	authorizer := authz.NewAuthorizer(gatewayClient, r.Backend(), storageClient, config.DevMode, acrHelper, skillAccessRuleHelper, hostedAgentAccessRuleHelper, registryNoAuth)

	// The kubernetes agent backend needs a client to the local cluster even when
	// MCP servers do not run there -- the "hosted agents get the client
	// directly" case the comment further down anticipates. It reuses the MCP
	// client when one exists, and otherwise builds its own from the same
	// config, so hosted agents work with a docker MCP backend without switching
	// on the MCP-scoped router and its handlers.
	agentLocalK8sClient := apiLocalK8sClient
	if agentLocalK8sClient == nil && hostedAgentsNeedK8s(config) {
		agentLocalK8sClient, err = kclient.NewWithWatch(localK8sConfig, kclient.Options{Scheme: k8sscheme.Scheme})
		if err != nil {
			return nil, fmt.Errorf("failed to build local k8s client for the agent backend: %w", err)
		}
	}
	agentBackendKind, agentBackend, err := newHostedAgentsBackend(config, localK8sConfig, agentLocalK8sClient, localCacheClient)
	if err != nil {
		return nil, err
	}

	// Where a sandbox finds Obot depends on where Obot itself is running, and
	// the two cases need different answers.
	//
	// In the cluster, the configured hostname resolving is not enough: it has to
	// resolve somewhere a sandbox is permitted to reach, and the egress policy
	// shipped with the chart excludes private ranges, so a hostname pointing at
	// an internal load balancer or a node is refused even though it resolves.
	// MCP servers have always answered this by addressing Obot's Service, and a
	// sandbox reaches Obot for the same things, so TransformObotHostname gives
	// it the same address rather than inventing a second mechanism.
	//
	// Outside the cluster -- a developer running Obot on their own machine --
	// that method cannot answer at all: it substitutes Obot's Service, and there
	// is no Service, so ServiceName is unset and it returns the hostname
	// untouched. The hostname is a loopback address, which inside a sandbox
	// means the sandbox. A node stands in for the developer's machine, since
	// Obot listens on all of its interfaces, which is what ReachableServerURL
	// resolves. It leaves any non-loopback hostname alone, so the in-cluster
	// case falls through to the method above.
	agentServerURL := config.Hostname
	if agentBackendKind == "kubernetes" && agentLocalK8sClient != nil {
		reachable, err := agentbackendkubernetes.ReachableServerURL(ctx, agentLocalK8sClient, config.Hostname)
		if err != nil {
			return nil, err
		}
		if reachable != config.Hostname {
			agentServerURL = reachable
		} else {
			agentServerURL = mcpSessionManager.TransformObotHostname(config.Hostname)
		}
		if agentServerURL != config.Hostname {
			slog.Info("hosted agent sandboxes will reach Obot at a rewritten URL", "agentServerURL", agentServerURL)
		}
	}

	// Running outside the cluster is what makes a sandbox unreachable, and it is
	// exactly what falling back to a kubeconfig means. Reaching one through the
	// API server needs services/proxy, which grants access to every Service in
	// the cluster, so this is deliberately confined to development rather than
	// something a production deployment is granted.
	var agentDevRouter agentconnect.DevRouter
	if router, ok := agentBackend.(agentconnect.DevRouter); ok && config.DevMode {
		if _, inClusterErr := rest.InClusterConfig(); inClusterErr != nil {
			agentDevRouter = router
		}
	}

	// LocalK8sClient drives MCP-side cluster features: service account key
	// rotation, image pull secret issuance, network policies. Hosted agents get
	// the client directly, so this stays nil when MCP does not run on
	// Kubernetes. Otherwise enabling hosted agents alone would switch on MCP
	// machinery that needs configuration the deployment has no reason to set.
	var mcpLocalK8sClient kclient.WithWatch
	if mcp.IsKubernetesBackend(config.MCPRuntimeBackend) {
		mcpLocalK8sClient = apiLocalK8sClient
	}
	// For now, always auto-migrate the gateway database
	svcs := &Services{
		EncryptionConfig:      encryptionConfig,
		ServerURL:             config.Hostname,
		StorageClient:         storageClient,
		Router:                r,
		PersistentTokenServer: persistentTokenServer,
		APIServer: server.NewServer(
			storageClient,
			gatewayClient,
			apiLocalK8sClient,
			config.ServiceNamespace,
			authn.NewAuthenticator(authenticators),
			authorizer,
			proxyManager,
			auditLogger,
			rateLimiter,
			config.Hostname,
			oauthServerConfig.ScopesSupported,
			registryNoAuth,
			licenseProvider,
		),
		GatewayClient:                gatewayClient,
		ProxyManager:                 proxyManager,
		ProviderDispatcher:           providerDispatcher,
		Otel:                         otel,
		AuditLogger:                  auditLogger,
		MCPSessionManager:            mcpSessionManager,
		TunnelManager:                tunnelManager,
		OAuthServerConfig:            oauthServerConfig,
		MCPOAuthTokenStorage:         mcpOAuthTokenStorage,
		MCPSecretBindingAllowedLabel: secretBindingAllowedLabel,
		RegistryNoAuth:               registryNoAuth,
		DSN:                          config.DSN,
		PostgresDSN:                  postgresDSN,
		ObotNamespace:                config.ServiceNamespace,
		DevUIPort:                    devPort,
		DevMode:                      config.DevMode,
		UserUIPort:                   config.UserUIPort,
		ProviderRegistryPaths:        config.ProviderRegistries,
		GatewayServer:                gatewayServer,
		AuthEnabled:                  config.EnableAuthentication,
		Bootstrapper:                 bootstrapper,
		LocalAuthProvider:            localAuthProvider,

		DefaultMCPCatalogPath:          config.DefaultMCPCatalogPath,
		MDMAssetSource:                 config.MDMAssetSource,
		DefaultSystemMCPCatalogPath:    config.DefaultSystemMCPCatalogPath,
		DefaultSkillRepoURL:            config.DefaultSkillRepoURL,
		DefaultSkillRepoRef:            config.DefaultSkillRepoRef,
		DefaultHostedAgentsCatalogURL:  config.DefaultHostedAgentsCatalogURL,
		DefaultHostedAgentsCatalogRef:  config.DefaultHostedAgentsCatalogRef,
		ModelInfoSourceURL:             config.ModelInfoSourceURL,
		MCPOAuthClientSecretExpiration: oauthClientExpiration,
		MCPOAuthClientNativeExceptions: config.MCPOAuthClientNativeExceptions,
		ForceDynamicClient:             config.ForceDynamicClient,
		AccessControlRuleHelper:        acrHelper,
		ModelAccessPolicyHelper:        mapHelper,

		SkillAccessRuleHelper:                skillAccessRuleHelper,
		HostedAgentAccessRuleHelper:          hostedAgentAccessRuleHelper,
		LocalK8sClient:                       mcpLocalK8sClient,
		LocalRouter:                          localRouter,
		EveryReplicaRouter:                   tunnelPeerRouter,
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
		MCPImagePullSecrets:                  config.MCPImagePullSecrets,
		MCPHTTPWebhookBaseImage:              config.MCPHTTPWebhookBaseImage,
		SingleUserIdleServerShutdownInterval: time.Duration(config.SingleUserIdleServerShutdownHours) * time.Hour,
		MultiUserIdleServerShutdownInterval:  time.Duration(config.MultiUserIdleServerShutdownHours) * time.Hour,
		AgentIdleServerShutdownInterval:      time.Duration(config.IdleAgentShutdownHours) * time.Hour,
		MessagePoliciesEnabled:               config.EnableMessagePolicies,
		EnableAgents:                         config.EnableAgents,
		AgentBackend:                         agentBackend,
		AgentServerURL:                       agentServerURL,
		AgentBackendKind:                     agentBackendKind,
		AgentDevRouter:                       agentDevRouter,
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

func leaderElectionRESTConfig(config *rest.Config) *rest.Config {
	config = rest.CopyConfig(config)
	if config.Timeout == 0 || config.Timeout > leaderElectionRequestTimeout {
		config.Timeout = leaderElectionRequestTimeout
	}
	return config
}

// resolveHostedAgentsBackendKind applies the default so that callers which need to know
// the backend before it is constructed agree with newHostedAgentsBackend.
//
// Unset follows the MCP runtime, because a deployment that already runs MCP
// servers on a cluster has everything hosted agents need and would otherwise
// have to name the same backend twice.
//
// A docker MCP runtime resolves to the fake backend for now, because there is
// no docker agent backend yet. When one lands, this is where it goes: the
// docker case becomes "docker" and the fallback stops being a placeholder.
func resolveHostedAgentsBackendKind(config Config) string {
	kind := strings.ToLower(strings.TrimSpace(config.HostedAgentsBackend))
	if kind != "" {
		return kind
	}
	if mcp.IsKubernetesBackend(config.MCPRuntimeBackend) {
		return "kubernetes"
	}
	return "fake"
}

func hostedAgentsNeedK8s(config Config) bool {
	switch resolveHostedAgentsBackendKind(config) {
	case "kubernetes", "k8s":
		return true
	default:
		return false
	}
}

func newHostedAgentsBackend(config Config, restConfig *rest.Config, client, cachedClient kclient.Client) (string, agentbackend.Backend, error) {
	kind := resolveHostedAgentsBackendKind(config)

	switch kind {
	case "disabled":
		return kind, agentbackend.Disabled{}, nil
	case "fake":
		return kind, agentbackendfake.New(agentbackendfake.Config{
			TransitionDelay: time.Second,
		}), nil
	case "kubernetes", "k8s":
		if client == nil {
			return "", nil, fmt.Errorf("agent backend %q requires a local Kubernetes cluster, but no local K8s config is available", kind)
		}
		scheduling, err := parseHostedAgentPodSchedulingSettings(config)
		if err != nil {
			return "", nil, err
		}
		backend, err := agentbackendkubernetes.New(client, cachedClient, agentbackendkubernetes.Options{
			// Sandboxes share the MCP namespace so that the existing local
			// cluster router, which is scoped to it, sees them. Pools are
			// separated by PriorityClass rather than by namespace.
			Namespace:        config.MCPNamespace,
			ClusterDomain:    config.MCPClusterDomain,
			StorageClassName: config.HostedAgentsStorageClassName,
			RuntimeClassName: config.HostedAgentsRuntimeClassName,
			Affinity:         scheduling.Affinity,
			Tolerations:      scheduling.Tolerations,
			NodeSelector:     scheduling.NodeSelector,
			// Sandboxes share the MCP namespace, so they are admitted against
			// whatever Pod Security level that namespace carries.
			PodSecurityLevel: agentbackendkubernetes.ParsePodSecurityLevel(config.HostedAgentsPodSecurityLevel),
			ImagePullSecrets: config.MCPImagePullSecrets,
			CleanupImage:     config.HostedAgentsCleanupImage,
			ImagePullPolicy:  config.HostedAgentsImagePullPolicy,
			RESTConfig:       restConfig,
		})
		if err != nil {
			return "", nil, err
		}
		return "kubernetes", backend, nil
	default:
		return "", nil, fmt.Errorf("unsupported agent backend %q (expected disabled, fake, or kubernetes)", kind)
	}
}

func startDevMode(ctx context.Context, storageClient storage.Client) {
	_ = storageClient.Delete(ctx, &coordinationv1.Lease{
		Name:      "obot-controller",
		Namespace: "kube-system",
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
