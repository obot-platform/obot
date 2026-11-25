package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type VMCPInstanceHandler struct{}

func NewVMCPInstanceHandler() *VMCPInstanceHandler {
	return nil
}

func (*VMCPInstanceHandler) List(req api.Context) error {
	var (
		list       v1.VMCPInstanceList
		fields     = kclient.MatchingFields{}
		privileged = req.UserCanViewAdmin()
	)
	if !privileged {
		fields["spec.userID"] = req.User.GetUID()
	}
	if err := req.List(&list, fields); err != nil {
		return fmt.Errorf("failed to list VMCP instances: %w", err)
	}

	items := make([]types.VMCPInstance, 0, len(list.Items))
	for _, item := range list.Items {
		if principal.IsDelegatedOwner(item.Spec.UserID) {
			// What a user set up through an agent is listed to no one here, an administrator included
			// (authz.UserCanReadVMCPInstance).
			continue
		}
		if !privileged {
			var vmcp v1.VMCP
			if err := req.Get(&vmcp, item.Spec.Manifest.VMCPID); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("failed to get VMCP for instance %q: %w", item.Name, err)
			}
			if !authz.UserCanReadVMCPInstance(req.User, &item, &vmcp) {
				continue
			}
		}
		items = append(items, convertVMCPInstance(item))
	}
	return req.Write(types.VMCPInstanceList{Items: items})
}

func (*VMCPInstanceHandler) Get(req api.Context) error {
	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}
	return req.Write(convertVMCPInstance(instance))
}

func (*VMCPInstanceHandler) Create(req api.Context) error {
	var manifest types.VMCPInstanceManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP instance manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP instance manifest: %v", err)
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, manifest.VMCPID); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}

	if !authz.UserCanReadVMCP(req.User, &vmcp) {
		return types.NewErrForbidden("access denied to VMCP %q", manifest.VMCPID)
	}

	if err := authz.ValidateVMCPToolSelection(req.User, &vmcp, manifest.ComponentSet); err != nil {
		return err
	}

	existing, err := vmcpconfig.FindInstance(req.Context(), req.Storage, req.Namespace(), manifest.VMCPID, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to find existing VMCP instance: %w", err)
	}
	if existing != nil {
		return req.WriteCreated(convertVMCPInstance(*existing))
	}

	instance := v1.VMCPInstance{
		Finalizers:   []string{v1.VMCPInstanceFinalizer},
		GenerateName: system.VMCPInstancePrefix,
		Namespace:    req.Namespace(),
		Spec: v1.VMCPInstanceSpec{
			Manifest: manifest,
			UserID:   req.User.GetUID(),
		},
	}
	if err := req.Create(&instance); err != nil {
		return fmt.Errorf("failed to create VMCP instance: %w", err)
	}
	return req.WriteCreated(convertVMCPInstance(instance))
}

func (*VMCPInstanceHandler) Update(req api.Context) error {
	var manifest types.VMCPInstanceManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read VMCP instance manifest: %v", err)
	}
	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid VMCP instance manifest: %v", err)
	}

	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}
	if manifest.VMCPID != instance.Spec.Manifest.VMCPID {
		return types.NewErrBadRequest("vmcpID cannot be changed")
	}
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, manifest.VMCPID); err != nil {
		return err
	}
	selectionUser := req.User
	if instance.Spec.UserID != req.User.GetUID() && len(manifest.ComponentSet) > 0 {
		ownerID, err := principal.Parse(instance.Spec.UserID)
		if err != nil {
			return fmt.Errorf("invalid VMCP instance user ID: %w", err)
		}
		owner, err := req.GatewayClient.IdentityInfo(req.Context(), ownerID)
		if err != nil {
			return err
		}
		selectionUser = owner
	}
	if err := authz.ValidateVMCPToolSelection(selectionUser, &vmcp, manifest.ComponentSet); err != nil {
		return err
	}
	instance.Spec.Manifest = manifest
	if err := req.Update(&instance); err != nil {
		return fmt.Errorf("failed to update VMCP instance: %w", err)
	}
	return req.Write(convertVMCPInstance(instance))
}

func (*VMCPInstanceHandler) Delete(req api.Context) error {
	instanceID := req.PathValue("vmcp_instance_id")
	return req.Delete(&v1.VMCPInstance{
		Name:      instanceID,
		Namespace: req.Namespace(),
	})
}

func (*VMCPInstanceHandler) Configure(req api.Context) error {
	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}
	if err := refuseAgentInstanceConfiguration(instance); err != nil {
		return err
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Spec.Manifest.VMCPID); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}

	var configuration types.VMCPConfiguration
	if err := req.Read(&configuration); err != nil {
		return types.NewErrBadRequest("failed to read VMCP instance configuration: %v", err)
	}

	effective := vmcp.Spec.Manifest
	effective.Components = vmcpconfig.ComponentsForInstance(vmcp, instance)
	secrets, err := vmcpconfig.ValidateAndEncodeUserConfiguration(effective, configuration)
	if err != nil {
		return types.NewErrBadRequest("invalid VMCP instance configuration: %v", err)
	}
	// Reject a URL template value that changes the template's host now, rather than storing it
	// and letting the connection fail later with an error only admins see in the logs. This runs
	// here rather than in ValidateAndEncodeUserConfiguration because pkg/mcp imports pkg/vmcp.
	//
	// Admin-fixed and catalog-static values live in credentials this request does not read, so
	// a template that references one resolves as missing and is skipped; the runtime and the vMCP
	// controller still run the same check with every value before anything connects.
	for _, component := range effective.Components {
		remote := component.CatalogEntry.Manifest.RemoteConfig
		if remote == nil || remote.URLTemplate == "" {
			continue
		}
		// Trim the same way ValidateAndEncodeUserConfiguration does, so the check sees the
		// values that are stored.
		values := make(map[string]string, len(configuration.Components[component.ID]))
		for key, value := range configuration.Components[component.ID] {
			values[key] = strings.TrimSpace(value)
		}
		// No admin keys: the vMCP's fixed values aren't in values (see above), so a template
		// that uses one resolves as missing before the host check runs.
		if _, _, err := mcp.ResolveURLTemplate(remote.URLTemplate, vmcpconfig.ComponentConfig(component), values, nil); err != nil {
			return types.NewErrBadRequest("invalid VMCP instance configuration for component %q: %v", component.ID, err)
		}
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: secrets,
	}); err != nil {
		return fmt.Errorf("failed to store VMCP instance configuration: %w", err)
	}

	if err := syncInstanceConfigurationHash(req, &instance, secrets); err != nil {
		return err
	}
	return req.Write(convertVMCPInstance(instance))
}

// syncInstanceConfigurationHash annotates the instance with the digest of written,
// the configuration credential this request left behind (empty after Deconfigure
// deleted it), so controllers reconcile and the OAuth consent flow can wait for the
// resulting status.
func syncInstanceConfigurationHash(req api.Context, instance *v1.VMCPInstance, written map[string]string) error {
	existingAnnotationHash := instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation]
	toWriteHash := utils.Digest(written)

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Get(instance, instance.Name); err != nil {
			return err
		}

		currentAnnotationHash := instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation]
		if existingAnnotationHash != currentAnnotationHash {
			// If the currentHash is the same as what we want to write, then we're done.
			if currentAnnotationHash == toWriteHash {
				return nil
			}

			// Another request stored a different credential in the meantime.
			// Record what it stored rather than what this request wrote.
			credential, err := req.GatewayClient.RevealCredential(req.Context(),
				[]string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)},
				vmcpconfig.ConfigurationCredentialName(),
			)
			if err != nil {
				if _, ok := errors.AsType[gateway.CredentialNotFoundError](err); !ok {
					return fmt.Errorf("failed to reveal VMCP instance configuration: %w", err)
				}
				// A concurrent deconfigure removed the credential.
				credential.Secrets = map[string]string{}
			}
			if credential.Secrets == nil {
				credential.Secrets = map[string]string{}
			}

			toWriteHash = utils.Digest(credential.Secrets)
			if currentAnnotationHash == toWriteHash {
				// The annotation already describes the stored credential.
				return nil
			}

			existingAnnotationHash = currentAnnotationHash
		}

		if instance.Annotations == nil {
			instance.Annotations = make(map[string]string, 1)
		}
		instance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation] = toWriteHash

		return req.Update(instance)
	}); err != nil {
		return fmt.Errorf("failed to trigger VMCP instance configuration reconciliation: %w", err)
	}
	return nil
}

func (*VMCPInstanceHandler) Reveal(req api.Context) error {
	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}
	if err := refuseAgentInstanceConfiguration(instance); err != nil {
		return err
	}

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, instance.Spec.Manifest.VMCPID); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}

	credential, err := req.GatewayClient.RevealCredential(req.Context(),
		[]string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err != nil {
		if _, ok := errors.AsType[gateway.CredentialNotFoundError](err); !ok {
			return fmt.Errorf("failed to reveal VMCP instance configuration: %w", err)
		}
	}

	return req.Write(vmcpConfiguration(vmcpconfig.ComponentsForInstance(vmcp, instance), credential.Secrets, types.VMCPConfigurationPolicyUserAllowed))
}

func (*VMCPInstanceHandler) Deconfigure(req api.Context) error {
	var instance v1.VMCPInstance
	if err := req.Get(&instance, req.PathValue("vmcp_instance_id")); err != nil {
		return fmt.Errorf("failed to get VMCP instance: %w", err)
	}
	if err := refuseAgentInstanceConfiguration(instance); err != nil {
		return err
	}

	if _, err := req.GatewayClient.DeleteCredential(req.Context(),
		vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		vmcpconfig.ConfigurationCredentialName(),
	); err != nil {
		return fmt.Errorf("failed to delete VMCP instance configuration: %w", err)
	}
	if err := syncInstanceConfigurationHash(req, &instance, map[string]string{}); err != nil {
		return err
	}
	return req.Write(convertVMCPInstance(instance))
}

// refuseAgentInstanceConfiguration refuses to configure, deconfigure or reveal the configuration of an agent's
// instance. Agents are given only the values fixed for everyone, by the vMCP or the catalog entry, so that no one can
// give an agent a value a user would enter, and an agent's instance configuration is never read
// (vmcpconfig.InstanceTakesUserConfiguration).
func refuseAgentInstanceConfiguration(instance v1.VMCPInstance) error {
	if !vmcpconfig.InstanceTakesUserConfiguration(instance) {
		return types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("VMCP instance %q belongs to an agent, which uses only the configuration that the vMCP or the catalog entry fixes", instance.Name))
	}
	return nil
}

func convertVMCPInstance(instance v1.VMCPInstance) types.VMCPInstance {
	return types.VMCPInstance{
		LegacySlug:           instance.Spec.LegacySlug,
		Metadata:             MetadataFrom(&instance),
		VMCPInstanceManifest: instance.Spec.Manifest,
		UserID:               instance.Spec.UserID,
		Status: types.VMCPInstanceStatus{
			Configured:                   instance.Status.Configured,
			MissingRequiredConfiguration: instance.Status.MissingRequiredConfiguration,
			UserConfigurationHash:        instance.Status.UserConfigurationHash,
			ConfigurationCheckHash:       instance.Status.ConfigurationCheckHash,
		},
	}
}
