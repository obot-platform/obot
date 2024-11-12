package v1

import (
	"github.com/otto8-ai/nah/pkg/fields"
	OTTO8_gptscript_ai "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const Version = "v1"

var SchemeGroupVersion = schema.GroupVersion{
	Group:   OTTO8_gptscript_ai.Group,
	Version: Version,
}

func AddToScheme(scheme *runtime.Scheme) error {
	return AddToSchemeWithGV(scheme, SchemeGroupVersion)
}

func AddToSchemeWithGV(scheme *runtime.Scheme, schemeGroupVersion schema.GroupVersion) error {
	if err := fields.AddKnownTypesWithFieldConversion(scheme, schemeGroupVersion,
		&Agent{},
		&AgentList{},
		&Run{},
		&RunList{},
		&RunState{},
		&RunStateList{},
		&Reference{},
		&ReferenceList{},
		&Thread{},
		&ThreadList{},
		&Workflow{},
		&WorkflowList{},
		&WorkflowExecution{},
		&WorkflowExecutionList{},
		&WorkflowStep{},
		&WorkflowStepList{},
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
		&WebhookReference{},
		&WebhookReferenceList{},
		&CronJob{},
		&CronJobList{},
		&OAuthApp{},
		&OAuthAppList{},
		&OAuthAppReference{},
		&OAuthAppReferenceList{},
		&OAuthAppLogin{},
		&OAuthAppLoginList{},
		&Model{},
		&ModelList{},
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
