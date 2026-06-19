package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	obot_platform_ai "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const Version = "v1"

var SchemeGroupVersion = schema.GroupVersion{
	Group:   obot_platform_ai.Group,
	Version: Version,
}

func AddToScheme(scheme *runtime.Scheme) error {
	return AddToSchemeWithGV(scheme, SchemeGroupVersion)
}

func AddToSchemeWithGV(scheme *runtime.Scheme, schemeGroupVersion schema.GroupVersion) error {
	if err := fields.AddKnownTypesWithFieldConversion(scheme, schemeGroupVersion,
		&Alias{},
		&AliasList{},
		&MCPServer{},
		&MCPServerList{},
		&MCPNetworkPolicy{},
		&MCPNetworkPolicyList{},
		&MCPServerInstance{},
		&MCPServerInstanceList{},
		&MCPServerCatalogEntry{},
		&MCPServerCatalogEntryList{},
		&Model{},
		&ModelList{},
		&DefaultModelAlias{},
		&DefaultModelAliasList{},
		&UserDelete{},
		&UserDeleteList{},
		&UserRoleChange{},
		&UserRoleChangeList{},
		&UserGroupChange{},
		&UserGroupChangeList{},
		&GroupRoleChange{},
		&GroupRoleChangeList{},
		&MCPCatalog{},
		&MCPCatalogList{},
		&SystemMCPCatalog{},
		&SystemMCPCatalogList{},
		&SystemMCPServerCatalogEntry{},
		&SystemMCPServerCatalogEntryList{},
		&SkillRepository{},
		&SkillRepositoryList{},
		&Skill{},
		&SkillList{},
		&SkillAccessRule{},
		&SkillAccessRuleList{},
		&OAuthClient{},
		&OAuthClientList{},
		&OAuthAuthRequest{},
		&OAuthAuthRequestList{},
		&OAuthToken{},
		&OAuthTokenList{},
		&AccessControlRule{},
		&AccessControlRuleList{},
		&MCPWebhookValidation{},
		&MCPWebhookValidationList{},
		&PowerUserWorkspace{},
		&PowerUserWorkspaceList{},
		&UserDefaultRoleSetting{},
		&UserDefaultRoleSettingList{},
		&K8sSettings{},
		&K8sSettingsList{},
		&ImagePullSecret{},
		&ImagePullSecretList{},
		&AppPreferences{},
		&AppPreferencesList{},
		&AppNotification{},
		&AppNotificationList{},
		&AuditLogExport{},
		&AuditLogExportList{},
		&ScheduledAuditLogExport{},
		&ScheduledAuditLogExportList{},
		&SystemMCPServer{},
		&SystemMCPServerList{},
		&ModelAccessPolicy{},
		&ModelAccessPolicyList{},
		&MessagePolicy{},
		&MessagePolicyList{},
		&NanobotAgent{},
		&NanobotAgentList{},
		&Project{},
		&ProjectList{},
		&ProjectV2{},
		&ProjectV2List{},
		&PublishedArtifact{},
		&PublishedArtifactList{},
		&OktaGroupMigration{},
		&OktaGroupMigrationList{},
		&AuthProvider{},
		&AuthProviderList{},
		&ModelProvider{},
		&ModelProviderList{},
	); err != nil {
		return err
	}

	// Add common types
	scheme.AddKnownTypes(schemeGroupVersion, &metav1.Status{})

	if schemeGroupVersion == SchemeGroupVersion {
		// Add the watch version that applies
		metav1.AddToGroupVersion(scheme, schemeGroupVersion)
	}
	return nil
}
