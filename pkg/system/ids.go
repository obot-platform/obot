package system

import (
	"strings"

	"github.com/obot-platform/nah/pkg/name"
)

const (
	SystemThreadPrefix               = "st1"
	ThreadPrefix                     = "t1"
	ThreadAuthorizationPrefix        = "ta1"
	RunPrefix                        = "r1"
	ModelPrefix                      = "m1"
	AliasPrefix                      = "al1"
	DefaultModelAliasPrefix          = "dma1"
	ProjectPrefix                    = "p1"
	UserDeletePrefix                 = "ud1"
	UserRoleChangePrefix             = "urc1"
	UserGroupChangePrefix            = "ugc1"
	GroupRoleChangePrefix            = "grc1"
	MCPServerPrefix                  = "ms1"
	MCPNetworkPolicyPrefix           = "mnp1"
	MCPServerInstancePrefix          = "msi1"
	ImagePullSecretPrefix            = "ips1"
	CatalogPrefix                    = "mcat1"
	SystemCatalogPrefix              = "smcat1"
	SkillRepositoryPrefix            = "skr1"
	SkillPrefix                      = "sk1"
	SkillAccessRulePrefix            = "sar1"
	OAuthClientPrefix                = "oc1"
	OAuthAuthRequestPrefix           = "oar1"
	AccessControlRulePrefix          = "acr1"
	MCPWebhookValidationPrefix       = "mwv1"
	PowerUserWorkspacePrefix         = "puw1"
	AuditLogExportPrefix             = "ael1"
	ScheduledAuditLogExportPrefix    = "sael1"
	LLMAuditLogExportPrefix          = "lael1"
	ScheduledLLMAuditLogExportPrefix = "slael1"
	SystemMCPServerPrefix            = "sms1"
	ModelAccessPolicyPrefix          = "map1"
	MessagePolicyPrefix              = "mp1"
	NanobotAgentPrefix               = "nba1"
	PublishedArtifactPrefix          = "pa1"
	OktaGroupMigrationPrefix         = "ogm1"

	ObotMCPServerName = SystemMCPServerPrefix + "obot-mcp-server"
)

func IsMCPServerID(id string) bool {
	return strings.HasPrefix(id, MCPServerPrefix)
}

func IsMCPServerInstanceID(id string) bool {
	return strings.HasPrefix(id, MCPServerInstancePrefix)
}

func IsPowerUserWorkspaceID(id string) bool {
	return strings.HasPrefix(id, PowerUserWorkspacePrefix)
}

func IsSystemMCPServerID(id string) bool {
	return strings.HasPrefix(id, SystemMCPServerPrefix)
}

func IsWebhookSystemMCPServerID(id string) bool {
	return strings.HasPrefix(id, SystemMCPServerPrefix+MCPWebhookValidationPrefix)
}

func IsModelID(id string) bool {
	return strings.HasPrefix(id, ModelPrefix)
}

func GetPowerUserWorkspaceID(userID string) string {
	return name.SafeConcatName(PowerUserWorkspacePrefix, userID)
}
