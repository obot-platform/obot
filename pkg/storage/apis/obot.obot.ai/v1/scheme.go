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
		&AgentAuthorization{},
		&AgentAuthorizationList{},
		&Alias{},
		&AliasList{},
		&Agent{},
		&AgentList{},
		&EmailReceiver{},
		&EmailReceiverList{},
		&MCPServer{},
		&MCPServerList{},
		&MCPServerCatalogEntry{},
		&MCPServerCatalogEntryList{},
		&Run{},
		&RunList{},
		&RunState{},
		&RunStateList{},
		&Thread{},
		&ThreadList{},
		&ThreadAuthorization{},
		&ThreadAuthorizationList{},
		&Workflow{},
		&WorkflowList{},
		&WorkflowExecution{},
		&WorkflowExecutionList{},
		&WorkflowStep{},
		&WorkflowStepList{},
		&KnowledgeSummary{},
		&KnowledgeSummaryList{},
		&KnowledgeSource{},
		&KnowledgeSourceList{},
		&KnowledgeFile{},
		&KnowledgeFileList{},
		&KnowledgeSet{},
		&KnowledgeSetList{},
		&ToolReference{},
		&ToolReferenceList{},
		&Workspace{},
		&WorkspaceList{},
		&Webhook{},
		&WebhookList{},
		&CronJob{},
		&CronJobList{},
		&OAuthApp{},
		&OAuthAppList{},
		&OAuthAppLogin{},
		&OAuthAppLoginList{},
		&Model{},
		&ModelList{},
		&DefaultModelAlias{},
		&DefaultModelAliasList{},
		&MemorySet{},
		&MemorySetList{},
		&Tool{},
		&ToolList{},
		&ThreadShare{},
		&ThreadShareList{},
		&ProjectInvitation{},
		&ProjectInvitationList{},
		&SlackTrigger{},
		&SlackTriggerList{},
		&SlackReceiver{},
		&SlackReceiverList{},
		&UserDelete{},
		&UserDeleteList{},
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
