package agents

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/create"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func createWorkspace(ctx context.Context, c kclient.Client, agent *v1.Agent) error {
	if agent.Status.WorkspaceName != "" {
		return nil
	}

	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  agent.Namespace,
			Name:       name.SafeConcatName(system.WorkspacePrefix, agent.Name),
			Finalizers: []string{v1.WorkspaceFinalizer},
		},
		Spec: v1.WorkspaceSpec{
			AgentName: agent.Name,
		},
	}
	if err := create.OrGet(ctx, c, ws); err != nil {
		return err
	}

	agent.Status.WorkspaceName = ws.Name
	return c.Status().Update(ctx, agent)
}

func createKnowledgeSet(ctx context.Context, c kclient.Client, agent *v1.Agent) error {
	if len(agent.Status.KnowledgeSetNames) > 0 {
		return nil
	}

	ks := &v1.KnowledgeSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  agent.Namespace,
			Name:       name.SafeConcatName(system.KnowledgeSetPrefix, agent.Name),
			Finalizers: []string{v1.KnowledgeSetFinalizer},
		},
		Spec: v1.KnowledgeSetSpec{
			AgentName: agent.Name,
		},
	}
	if err := create.OrGet(ctx, c, ks); err != nil {
		return err
	}

	agent.Status.KnowledgeSetNames = append(agent.Status.KnowledgeSetNames, ks.Name)
	return c.Status().Update(ctx, agent)
}

func CreateWorkspaceAndKnowledgeSet(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.Agent)

	if err := createWorkspace(req.Ctx, req.Client, agent); err != nil {
		return err
	}

	return createKnowledgeSet(req.Ctx, req.Client, agent)
}

func BackPopulateAuthStatus(req router.Request, _ router.Response) error {
	agent := req.Object.(*v1.Agent)

	var logins v1.OAuthAppLoginList
	if err := req.List(&logins, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.credentialContext": agent.Name}),
		Namespace:     agent.Namespace,
	}); err != nil {
		return err
	}

	existingLoginInfo := agent.Status.AuthStatus
	updateRequired := len(existingLoginInfo) != len(logins.Items)
	agent.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
	for _, login := range logins.Items {
		var required *bool
		credentialTools, err := v1.CredentialTools(req.Ctx, req.Client, agent.Namespace, login.Spec.ToolReference)
		if err != nil {
			login.Status.External.Error = fmt.Sprintf("failed to get credential tool for knowledge source [%s]: %v", agent.Name, err)
		} else {
			required = &[]bool{len(credentialTools) > 0}[0]
			updateRequired = updateRequired || login.Status.External.Required == nil || *login.Status.External.Required != *required
			login.Status.External.Required = required
		}

		if required != nil && *required {
			updateRequired = updateRequired || equality.Semantic.DeepEqual(login.Status.External, existingLoginInfo[login.Spec.ToolReference])
		}

		agent.Status.AuthStatus[login.Spec.ToolReference] = login.Status.External
	}

	if updateRequired {
		return req.Client.Status().Update(req.Ctx, agent)
	}

	return nil
}
