package handlers

import (
	"errors"
	"fmt"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hostedagentaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type HostedAgentHandler struct {
	accessRuleHelper *hostedagentaccessrule.Helper
}

func NewHostedAgentHandler(accessRuleHelper *hostedagentaccessrule.Helper) *HostedAgentHandler {
	return &HostedAgentHandler{accessRuleHelper: accessRuleHelper}
}

func (h *HostedAgentHandler) List(req api.Context) error {
	var list v1.HostedAgentList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list hosted agents: %w", err)
	}

	// Admins and auditors can see every agent with ?all=true. Everyone else,
	// including admins without the flag, sees only what the access rules allow.
	if (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true" {
		items := make([]types.HostedAgent, 0, len(list.Items))
		for _, item := range list.Items {
			items = append(items, convertHostedAgent(item))
		}
		return req.Write(types.HostedAgentList{Items: items})
	}

	items := make([]types.HostedAgent, 0, len(list.Items))
	for _, item := range list.Items {
		hasAccess, err := h.accessRuleHelper.UserHasAccessToHostedAgent(req.User, &item)
		if err != nil {
			return fmt.Errorf("failed to check access to hosted agent %s: %w", item.Name, err)
		}
		if hasAccess {
			items = append(items, convertHostedAgent(item))
		}
	}

	return req.Write(types.HostedAgentList{Items: items})
}

func (h *HostedAgentHandler) Get(req api.Context) error {
	var agent v1.HostedAgent
	if err := req.Get(&agent, req.PathValue("hosted_agent_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent: %w", err)
	}

	return req.Write(convertHostedAgent(agent))
}

func (h *HostedAgentHandler) Create(req api.Context) error {
	var manifest types.HostedAgentManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read hosted agent manifest: %v", err)
	}

	if err := validateHostedAgentManifest(req, manifest); err != nil {
		return err
	}

	secrets := extractHostedAgentSecrets(&manifest)

	agent := v1.HostedAgent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.HostedAgentPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.HostedAgentSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&agent); err != nil {
		return fmt.Errorf("failed to create hosted agent: %w", err)
	}

	if len(secrets) > 0 {
		if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
			Context: HostedAgentCredentialContext(agent),
			Name:    agent.Name,
			Secrets: secrets,
		}); err != nil {
			return fmt.Errorf("failed to store hosted agent credentials: %w", err)
		}
	}

	return req.WriteCreated(convertHostedAgent(agent))
}

func (h *HostedAgentHandler) Update(req api.Context) error {
	var manifest types.HostedAgentManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read hosted agent manifest: %v", err)
	}

	if err := validateHostedAgentManifest(req, manifest); err != nil {
		return err
	}

	var agent v1.HostedAgent
	if err := req.Get(&agent, req.PathValue("hosted_agent_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent: %w", err)
	}

	newSecrets := extractHostedAgentSecrets(&manifest)

	existing, err := hostedAgentSecrets(req, agent)
	if err != nil {
		return err
	}

	// A blank value on a sensitive env var means "leave it as it was", so that
	// clients can round-trip a manifest without ever seeing the secret.
	secrets := make(map[string]string, len(newSecrets))
	for _, env := range manifest.Env {
		if !env.Sensitive {
			continue
		}
		if value, ok := newSecrets[env.Key]; ok {
			secrets[env.Key] = value
		} else if value, ok := existing[env.Key]; ok {
			secrets[env.Key] = value
		}
	}

	agent.Spec.Manifest = manifest
	if err := req.Update(&agent); err != nil {
		return fmt.Errorf("failed to update hosted agent: %w", err)
	}

	if len(secrets) > 0 {
		if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
			Context: HostedAgentCredentialContext(agent),
			Name:    agent.Name,
			Secrets: secrets,
		}); err != nil {
			return fmt.Errorf("failed to store hosted agent credentials: %w", err)
		}
	} else if len(existing) > 0 {
		if _, err := req.GatewayClient.DeleteCredential(req.Context(), HostedAgentCredentialContext(agent), agent.Name); err != nil {
			return fmt.Errorf("failed to delete hosted agent credentials: %w", err)
		}
	}

	return req.Write(convertHostedAgent(agent))
}

func (h *HostedAgentHandler) Delete(req api.Context) error {
	var agent v1.HostedAgent
	if err := req.Get(&agent, req.PathValue("hosted_agent_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent: %w", err)
	}

	if _, err := req.GatewayClient.DeleteCredential(req.Context(), HostedAgentCredentialContext(agent), agent.Name); err != nil {
		return fmt.Errorf("failed to delete hosted agent credentials: %w", err)
	}

	return req.Delete(&v1.HostedAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: req.Namespace(),
		},
	})
}

func (h *HostedAgentHandler) Reveal(req api.Context) error {
	var agent v1.HostedAgent
	if err := req.Get(&agent, req.PathValue("hosted_agent_id")); err != nil {
		return fmt.Errorf("failed to get hosted agent: %w", err)
	}

	secrets, err := hostedAgentSecrets(req, agent)
	if err != nil {
		return err
	}

	if len(secrets) == 0 {
		return req.Write(map[string]string{})
	}

	return req.Write(secrets)
}

// validateHostedAgentManifest runs the shared manifest validation, plus the
// checks that apiclient/types deliberately leaves out: cron parsing (a
// question's default is an answer like any other, so it gets the same
// treatment) and that the referenced harness actually exists.
func validateHostedAgentManifest(req api.Context, manifest types.HostedAgentManifest) error {
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid hosted agent manifest: %v", err)
	}

	var harness v1.Harness
	if err := req.Get(&harness, manifest.HarnessID); apierrors.IsNotFound(err) {
		return types.NewErrBadRequest("harness %s not found", manifest.HarnessID)
	} else if err != nil {
		return fmt.Errorf("failed to get harness %s: %w", manifest.HarnessID, err)
	}

	for _, question := range manifest.Questions {
		if question.Type != types.HostedAgentQuestionTypeSchedule || question.Default == "" {
			continue
		}
		if !gronx.IsValid(question.Default) {
			return types.NewErrBadRequest("invalid hosted agent manifest: question %s: default must be a valid cron expression", question.Key)
		}
	}

	return nil
}

// extractHostedAgentSecrets pulls the values of sensitive env vars out of the
// manifest and blanks them, so they are only ever persisted in the credential
// store and never on the resource itself.
func extractHostedAgentSecrets(manifest *types.HostedAgentManifest) map[string]string {
	secrets := make(map[string]string)
	for i, env := range manifest.Env {
		if env.Sensitive && env.Value != "" {
			secrets[env.Key] = env.Value
			manifest.Env[i].Value = ""
		}
	}
	return secrets
}

func hostedAgentSecrets(req api.Context, agent v1.HostedAgent) (map[string]string, error) {
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{HostedAgentCredentialContext(agent)}, agent.Name)
	if err != nil {
		if errors.As(err, &gateway.CredentialNotFoundError{}) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find hosted agent credential: %w", err)
	}

	return cred.Secrets, nil
}

func HostedAgentCredentialContext(agent v1.HostedAgent) string {
	return fmt.Sprintf("hosted-agent-%s", agent.Name)
}

func convertHostedAgent(agent v1.HostedAgent) types.HostedAgent {
	return types.HostedAgent{
		Metadata:            MetadataFrom(&agent),
		HostedAgentManifest: agent.Spec.Manifest,
	}
}
