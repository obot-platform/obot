package vmcpinstance

import (
	"errors"
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// SyncAgentProfileConfiguration gives an agent's instance the configuration its profiles resolve it to
// (vmcpconfig.ResolveAgentConfiguration), by writing it to the instance's configuration credential: the credential a
// user's own entries are kept in for a user's instance, from which the controllers configure the instance's dedicated
// component servers and its connections to shared ones. An agent never enters configuration, so its instance holds
// exactly what its profiles give it, and nothing an administrator entered on the instance survives. Values the vMCP
// fixes are not copied: component servers take them from the vMCP's own credential, for agents as for users.
//
// It runs only for agents' instances. It watches the vMCP, whose profiles and whose status, which publishes each
// profile's configuration revision, decide the resolution, and the agent's lifecycle. Once the credential is written,
// it records the resolution on the instance (AgentConfigurationRevision), which a connect waits for, and the
// revisions it was copied from, which the vMCP controller keeps until no instance records them.
func (h *Handler) SyncAgentProfileConfiguration(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.VMCPInstance)
	if _, ok := principal.ParseAgent(instance.Spec.UserID); !ok || !instance.DeletionTimestamp.IsZero() {
		// A user's instance keeps the configuration the user enters.
		return nil
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Namespace, instance.Spec.Manifest.VMCPID); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("get VMCP %q: %w", instance.Spec.Manifest.VMCPID, err)
	}
	if vmcp.Spec.UserID != "" {
		// Agents never use a personal vMCP, and personal vMCPs have no profiles.
		return nil
	}

	agent, err := h.instanceUser(req, vmcp, *instance)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// DeleteUnauthorized deletes the instance of a deleted agent.
		return nil
	} else if err != nil {
		return err
	}

	configuration := vmcpconfig.ResolveAgentConfiguration(vmcp, *instance, agent)
	if instance.Status.AgentConfigurationRevision == configuration.Revision {
		return nil
	}
	values, err := h.profileConfiguration.AgentConfigurationValues(req.Ctx, vmcp.Name, configuration)
	if err != nil {
		return fmt.Errorf("resolve profile configuration for VMCP instance %q: %w", instance.Name, err)
	}

	credentialContext := vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)
	if len(values) == 0 {
		if _, err := h.deleteCredential(req.Ctx, credentialContext, vmcpconfig.ConfigurationCredentialName()); err != nil {
			return fmt.Errorf("clear configuration of VMCP instance %q: %w", instance.Name, err)
		}
	} else if err := h.upsertCredential(req.Ctx, gatewaytypes.Credential{
		Context: credentialContext,
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: values,
	}); err != nil {
		return fmt.Errorf("store configuration of VMCP instance %q: %w", instance.Name, err)
	}

	// The sync annotation is how SyncUserConfigurationHash, and a connect waiting on it, learn that the credential
	// changed, as after a user saves their configuration. It carries the resolution's revision, which reveals nothing
	// about the values.
	if instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation] != configuration.Revision {
		if instance.Annotations == nil {
			instance.Annotations = map[string]string{}
		}
		instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation] = configuration.Revision
		// Update the metadata before the status: the client writes the new resource version back onto the object, so
		// the status update below still applies cleanly.
		if err := req.Client.Update(req.Ctx, instance); err != nil {
			return err
		}
	}
	instance.Status.AgentConfigurationRevision = configuration.Revision
	instance.Status.AgentConfigurationRevisions = configuration.Revisions
	return req.Client.Status().Update(req.Ctx, instance)
}
