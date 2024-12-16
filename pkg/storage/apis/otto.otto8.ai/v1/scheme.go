package v1

import (
	otto8_gptscript_ai "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai"
	"github.com/acorn-io/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const Version = "v1"

var SchemeGroupVersion = schema.GroupVersion{
	Group:   otto8_gptscript_ai.Group,
	Version: Version,
}

func AddToScheme(scheme *runtime.Scheme) error {
	return AddToSchemeWithGV(scheme, SchemeGroupVersion)
}

func AddToSchemeWithGV(scheme *runtime.Scheme, schemeGroupVersion schema.GroupVersion) error {
	if err := fields.AddKnownTypesWithFieldConversion(scheme, schemeGroupVersion,
		&Alias{},
		&AliasList{},
		&Agent{},
		&AgentList{},
		&EmailReceiver{},
		&EmailReceiverList{},
		&Run{},
		&RunList{},
		&RunState{},
		&RunStateList{},
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
