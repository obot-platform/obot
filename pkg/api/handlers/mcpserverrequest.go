package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/mcpserverrequest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MCPServerRequestHandler handles users' requests to add MCP servers to the catalog and
// administrators' decisions on them. The built-in Obot MCP servers call its exported
// methods, so a request made from an agent follows the same path as one made in the UI.
type MCPServerRequestHandler struct {
	vmcps    *VMCPHandler
	catalogs *MCPCatalogHandler
	notifier *mcpserverrequest.Notifier
}

func NewMCPServerRequestHandler(vmcps *VMCPHandler, catalogs *MCPCatalogHandler, notifier *mcpserverrequest.Notifier) *MCPServerRequestHandler {
	return &MCPServerRequestHandler{vmcps: vmcps, catalogs: catalogs, notifier: notifier}
}

func (h *MCPServerRequestHandler) Create(req api.Context) error {
	var manifest types.MCPServerRequestManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read request: %v", err)
	}
	request, err := h.CreateRequest(req, manifest)
	if err != nil {
		return err
	}
	return req.WriteCreated(request)
}

func (h *MCPServerRequestHandler) List(req api.Context) error {
	// Administrators see every request unless they ask for their own.
	all := req.UserIsAdmin() && req.URL.Query().Get("mine") != "true"
	requests, err := h.ListRequests(req, all, types.MCPServerRequestState(req.URL.Query().Get("state")))
	if err != nil {
		return err
	}
	return req.Write(types.MCPServerRequestList{Items: requests})
}

func (h *MCPServerRequestHandler) Get(req api.Context) error {
	request, err := h.GetRequest(req, req.PathValue("request_id"))
	if err != nil {
		return err
	}
	return req.Write(request)
}

func (h *MCPServerRequestHandler) Approve(req api.Context) error {
	var approval types.MCPServerRequestApproval
	if err := req.Read(&approval); err != nil {
		return types.NewErrBadRequest("failed to read approval: %v", err)
	}
	request, err := h.ApproveRequest(req, req.PathValue("request_id"), approval)
	if err != nil {
		return err
	}
	return req.Write(request)
}

func (h *MCPServerRequestHandler) Deny(req api.Context) error {
	var denial types.MCPServerRequestDenial
	if err := req.Read(&denial); err != nil {
		return types.NewErrBadRequest("failed to read denial: %v", err)
	}
	request, err := h.DenyRequest(req, req.PathValue("request_id"), denial.Reason)
	if err != nil {
		return err
	}
	return req.Write(request)
}

// ReviewURL is the page where an administrator decides on the request.
func (h *MCPServerRequestHandler) ReviewURL(id string) string {
	return h.notifier.ReviewURL(id)
}

// CreateRequest records the caller's request and alerts administrators.
func (h *MCPServerRequestHandler) CreateRequest(req api.Context, manifest types.MCPServerRequestManifest) (types.MCPServerRequest, error) {
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.ExistingCatalogEntryID != "" {
		// A request for a server already in the catalog: describe it from the entry, which the
		// requester must be able to see.
		entry, err := h.catalogs.VisibleCatalogEntry(req, manifest.ExistingCatalogEntryID)
		if err != nil {
			return types.MCPServerRequest{}, err
		}
		manifest = types.MCPServerRequestManifest{
			Name:                   entry.Spec.Manifest.Name,
			Description:            cmpOr(entry.Spec.Manifest.ShortDescription, entry.Spec.Manifest.Description),
			Reason:                 manifest.Reason,
			SourceURL:              entry.Spec.Manifest.RepoURL,
			Runtime:                entry.Spec.Manifest.Runtime,
			SuggestedVMCPID:        manifest.SuggestedVMCPID,
			ExistingCatalogEntryID: entry.Name,
		}
	}
	if err := manifest.Validate(); err != nil {
		return types.MCPServerRequest{}, types.NewErrBadRequest("invalid request: %v", err)
	}

	request := v1.MCPServerRequest{
		GenerateName: system.MCPServerRequestPrefix,
		Namespace:    req.Namespace(),
		Spec: v1.MCPServerRequestSpec{
			Manifest:    manifest,
			RequesterID: req.User.GetUID(),
		},
	}
	if user, err := req.GatewayClient.UserByID(req.Context(), req.User.GetUID()); err == nil {
		request.Spec.RequesterName = cmpOr(user.DisplayName, user.Username)
		request.Spec.RequesterEmail = user.Email
	}
	if err := req.Create(&request); err != nil {
		return types.MCPServerRequest{}, fmt.Errorf("failed to create MCP server request: %w", err)
	}
	if err := h.setStatus(req, request.Name, func(status *v1.MCPServerRequestStatus) error {
		status.State = types.MCPServerRequestStatePending
		return nil
	}, &request); err != nil {
		return types.MCPServerRequest{}, err
	}

	converted := convertMCPServerRequest(request)
	if h.notifier.Enabled() {
		// Alerting must not hold up the requester, and a failure to alert must not lose the
		// request: it is still listed for administrators in Obot.
		go h.notify(req.Storage, req.Namespace(), converted)
	}
	return converted, nil
}

func (h *MCPServerRequestHandler) notify(client kclient.Client, namespace string, request types.MCPServerRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := h.notifier.NotifyRequested(ctx, request); err != nil {
		slog.Error("Failed to alert administrators about an MCP server request", "request", request.ID, "error", err)
		return
	}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var stored v1.MCPServerRequest
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: request.ID}, &stored); err != nil {
			return err
		}
		stored.Status.Notified = true
		return client.Status().Update(ctx, &stored)
	}); err != nil {
		slog.Warn("Failed to record that administrators were alerted", "request", request.ID, "error", err)
	}
}

// ListRequests lists every request when all is set, and otherwise the caller's own.
// An empty state lists requests in any state.
func (h *MCPServerRequestHandler) ListRequests(req api.Context, all bool, state types.MCPServerRequestState) ([]types.MCPServerRequest, error) {
	if all && !req.UserIsAdmin() {
		return nil, types.NewErrForbidden("only administrators can list every MCP server request")
	}
	var list v1.MCPServerRequestList
	if err := req.List(&list); err != nil {
		return nil, err
	}
	items := make([]types.MCPServerRequest, 0, len(list.Items))
	for _, request := range list.Items {
		if !all && request.Spec.RequesterID != req.User.GetUID() {
			continue
		}
		if state != "" && requestState(request) != state {
			continue
		}
		items = append(items, convertMCPServerRequest(request))
	}
	// Newest first.
	slices.SortFunc(items, func(a, b types.MCPServerRequest) int {
		return b.Created.Time.Compare(a.Created.Time)
	})
	return items, nil
}

// GetRequest returns a request to its requester or an administrator.
func (h *MCPServerRequestHandler) GetRequest(req api.Context, id string) (types.MCPServerRequest, error) {
	request, err := h.getVisible(req, id)
	if err != nil {
		return types.MCPServerRequest{}, err
	}
	return h.withAccess(req, convertMCPServerRequest(request)), nil
}

// requesterInfo is the requester as authorization sees them, or nil if they can't be found.
func (h *MCPServerRequestHandler) requesterInfo(req api.Context, id string) kuser.Info {
	uid, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return nil
	}
	info, err := req.GatewayClient.UserInfoByID(req.Context(), uint(uid))
	if err != nil {
		return nil
	}
	return info
}

// uniqueComponentName returns name, or name with a numeric suffix if a component already
// uses it.
func uniqueComponentName(components []types.VMCPComponent, name string) string {
	taken := func(n string) bool {
		return slices.ContainsFunc(components, func(c types.VMCPComponent) bool { return c.Name == n })
	}
	candidate := name
	for i := 2; taken(candidate); i++ {
		candidate = fmt.Sprintf("%s-%d", name, i)
	}
	return candidate
}

// grantRequester adds the component named componentName to the requester's own profile,
// creating the profile if needed.
func grantRequester(manifest *types.VMCPManifest, spec v1.MCPServerRequestSpec, componentName string) {
	var componentID string
	for _, c := range manifest.Components {
		if c.Name == componentName {
			componentID = c.ID
		}
	}
	if componentID == "" {
		return
	}
	subject := types.Subject{Type: types.SubjectTypeUser, ID: spec.RequesterID}
	for i := range manifest.Profiles {
		p := &manifest.Profiles[i]
		if len(p.Subjects) == 1 && p.Subjects[0] == subject && !p.Permissions.AllowAllComponents {
			if p.Permissions.AllowedComponents == nil {
				p.Permissions.AllowedComponents = map[string]types.VMCPComponentSet{}
			}
			p.Permissions.AllowedComponents[componentID] = types.VMCPComponentSet{}
			return
		}
	}
	name := "Requested by " + cmpOr(spec.RequesterEmail, spec.RequesterName, spec.RequesterID)
	for i := 2; slices.ContainsFunc(manifest.Profiles, func(p types.VMCPProfile) bool { return p.Name == name }); i++ {
		name = fmt.Sprintf("Requested by %s (%d)", cmpOr(spec.RequesterEmail, spec.RequesterName, spec.RequesterID), i)
	}
	manifest.Profiles = append(manifest.Profiles, types.VMCPProfile{
		Name:     name,
		Subjects: []types.Subject{subject},
		Permissions: types.VMCPProfilePermissions{
			AllowedComponents: map[string]types.VMCPComponentSet{componentID: {}},
		},
	})
}

// withAccess reports whether the requester can connect to the vMCP an approved request
// added the server to.
func (h *MCPServerRequestHandler) withAccess(req api.Context, request types.MCPServerRequest) types.MCPServerRequest {
	if request.VMCPID == "" {
		return request
	}
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, request.VMCPID); err != nil {
		return request
	}
	requester := h.requesterInfo(req, request.RequesterID)
	if requester == nil {
		return request
	}
	canConnect := authz.UserCanConnectVMCP(requester, &vmcp)
	request.RequesterCanConnect = &canConnect
	return request
}

func (h *MCPServerRequestHandler) getVisible(req api.Context, id string) (v1.MCPServerRequest, error) {
	var request v1.MCPServerRequest
	if !strings.HasPrefix(id, system.MCPServerRequestPrefix) {
		return request, types.NewErrNotFound("MCP server request %q not found", id)
	}
	if err := req.Get(&request, id); err != nil {
		if apierrors.IsNotFound(err) {
			return request, types.NewErrNotFound("MCP server request %q not found", id)
		}
		return request, err
	}
	if request.Spec.RequesterID != req.User.GetUID() && !req.UserIsAdmin() {
		return request, types.NewErrNotFound("MCP server request %q not found", id)
	}
	return request, nil
}

// ApproveRequest adds the requested server to the default catalog and then to an existing
// or a new vMCP. Nothing is left behind when a step fails.
func (h *MCPServerRequestHandler) ApproveRequest(req api.Context, id string, approval types.MCPServerRequestApproval) (types.MCPServerRequest, error) {
	if !req.UserIsAdmin() {
		return types.MCPServerRequest{}, types.NewErrForbidden("only administrators can approve MCP server requests")
	}
	if err := approval.Validate(); err != nil {
		return types.MCPServerRequest{}, types.NewErrBadRequest("invalid approval: %v", err)
	}
	request, err := h.getPending(req, id)
	if err != nil {
		return types.MCPServerRequest{}, err
	}

	// Check the target vMCP before changing anything.
	var target v1.VMCP
	if approval.VMCPID != "" {
		if err := req.Get(&target, approval.VMCPID); err != nil {
			if apierrors.IsNotFound(err) {
				return types.MCPServerRequest{}, types.NewErrBadRequest("vMCP %q not found", approval.VMCPID)
			}
			return types.MCPServerRequest{}, err
		}
		if target.Spec.UserID != "" {
			return types.MCPServerRequest{}, types.NewErrBadRequest("vMCP %q is a personal vMCP; choose a shared vMCP", approval.VMCPID)
		}
		if target.Spec.SourceURL != "" {
			return types.MCPServerRequest{}, types.NewErrBadRequest("vMCP %q is synced from a catalog and can't be changed in Obot", approval.VMCPID)
		}
	}

	var (
		entry       v1.MCPServerCatalogEntry
		removeEntry = func(cause error) error { return cause }
	)
	if id := request.Spec.Manifest.ExistingCatalogEntryID; id != "" {
		// The server is in the catalog already; only the vMCP changes.
		if err := req.Get(&entry, id); err != nil {
			if apierrors.IsNotFound(err) {
				return types.MCPServerRequest{}, types.NewErrBadRequest("catalog entry %q no longer exists", id)
			}
			return types.MCPServerRequest{}, err
		}
	} else {
		manifest := request.Spec.Manifest.CatalogEntryManifest()
		if approval.Manifest != nil {
			manifest = *approval.Manifest
		}
		entry, err = h.catalogs.CreateCatalogEntry(req, system.DefaultCatalog, "", manifest)
		if err != nil {
			return types.MCPServerRequest{}, err
		}
		removeEntry = func(cause error) error {
			if err := req.Delete(&entry); err != nil && !apierrors.IsNotFound(err) {
				return errors.Join(cause, fmt.Errorf("failed to remove catalog entry %s: %w", entry.Name, err))
			}
			return cause
		}
	}

	component := types.VMCPComponent{
		Name:                    componentName(entry.Spec.Manifest.Name),
		MCPCatalogID:            cmpOr(entry.Spec.MCPCatalogName, system.DefaultCatalog),
		MCPServerCatalogEntryID: entry.Name,
	}
	// Users supply the values the server needs, such as their own API keys.
	for _, config := range entry.Spec.Manifest.Config {
		if !config.Static {
			component.Configuration = append(component.Configuration, types.VMCPConfigurationPolicy{
				Key:    config.Key,
				Policy: types.VMCPConfigurationPolicyUserAllowed,
			})
		}
	}

	var vmcpID string
	if approval.NewVMCP != nil {
		vmcpManifest := types.VMCPManifest{
			DisplayName: strings.TrimSpace(approval.NewVMCP.DisplayName),
			Description: approval.NewVMCP.Description,
			Components:  []types.VMCPComponent{component},
			Profiles: []types.VMCPProfile{{
				Name: "default",
				Subjects: []types.Subject{
					{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin},
					{Type: types.SubjectTypeUser, ID: request.Spec.RequesterID},
				},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}},
		}
		vmcp, err := h.vmcps.CreateVMCP(req, vmcpManifest, false)
		if err != nil {
			return types.MCPServerRequest{}, removeEntry(err)
		}
		vmcpID = vmcp.Name
	} else {
		grant := approval.GrantRequesterAccess == nil || *approval.GrantRequesterAccess
		requester := h.requesterInfo(req, request.Spec.RequesterID)
		var addedName string
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := req.Get(&target, approval.VMCPID); err != nil {
				return err
			}
			vmcpManifest := *target.Spec.Manifest.DeepCopy()
			added := component
			added.Name = uniqueComponentName(vmcpManifest.Components, component.Name)
			addedName = added.Name
			vmcpManifest.Components = append(vmcpManifest.Components, added)
			return h.vmcps.UpdateVMCP(req, &target, vmcpManifest)
		}); err != nil {
			return types.MCPServerRequest{}, removeEntry(err)
		}
		vmcpID = target.Name
		if grant && requester != nil && !authz.UserCanConnectVMCP(requester, &target) {
			// The update assigned the new component its ID. Grant the requester that component
			// alone, in a profile of their own, so nothing else in the vMCP opens up to them. The
			// server is in the vMCP either way, so a failure here is reported, not undone.
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := req.Get(&target, approval.VMCPID); err != nil {
					return err
				}
				vmcpManifest := *target.Spec.Manifest.DeepCopy()
				grantRequester(&vmcpManifest, request.Spec, addedName)
				return h.vmcps.UpdateVMCP(req, &target, vmcpManifest)
			}); err != nil {
				slog.Error("Failed to grant the requester access to an approved MCP server", "request", request.Name, "vmcp", vmcpID, "error", err)
			}
		}
	}

	if err := h.setStatus(req, request.Name, func(status *v1.MCPServerRequestStatus) error {
		if status.State != types.MCPServerRequestStatePending {
			return types.NewErrBadRequest("MCP server request %q was already %s", request.Name, status.State)
		}
		status.State = types.MCPServerRequestStateApproved
		status.DecidedBy = req.User.GetUID()
		status.DecidedAt = &metav1.Time{Time: time.Now()}
		status.DecisionReason = approval.Reason
		status.CatalogEntryID = entry.Name
		status.VMCPID = vmcpID
		return nil
	}, &request); err != nil {
		// The server is in the catalog and the vMCP now; keep it rather than undo the vMCP
		// change, and report the failure.
		return types.MCPServerRequest{}, fmt.Errorf("added %s to vMCP %s but failed to record the approval: %w", entry.Name, vmcpID, err)
	}
	return h.withAccess(req, convertMCPServerRequest(request)), nil
}

// DenyRequest closes the request without adding anything.
func (h *MCPServerRequestHandler) DenyRequest(req api.Context, id, reason string) (types.MCPServerRequest, error) {
	if !req.UserIsAdmin() {
		return types.MCPServerRequest{}, types.NewErrForbidden("only administrators can deny MCP server requests")
	}
	request, err := h.getPending(req, id)
	if err != nil {
		return types.MCPServerRequest{}, err
	}
	if err := h.setStatus(req, request.Name, func(status *v1.MCPServerRequestStatus) error {
		if status.State != types.MCPServerRequestStatePending {
			return types.NewErrBadRequest("MCP server request %q was already %s", request.Name, status.State)
		}
		status.State = types.MCPServerRequestStateDenied
		status.DecidedBy = req.User.GetUID()
		status.DecidedAt = &metav1.Time{Time: time.Now()}
		status.DecisionReason = strings.TrimSpace(reason)
		return nil
	}, &request); err != nil {
		return types.MCPServerRequest{}, err
	}
	return convertMCPServerRequest(request), nil
}

func (h *MCPServerRequestHandler) getPending(req api.Context, id string) (v1.MCPServerRequest, error) {
	request, err := h.getVisible(req, id)
	if err != nil {
		return request, err
	}
	if state := requestState(request); state != types.MCPServerRequestStatePending {
		return request, types.NewErrBadRequest("MCP server request %q was already %s", id, state)
	}
	return request, nil
}

// setStatus applies change to the stored request's status, retrying on conflict, and
// leaves the result in out.
func (h *MCPServerRequestHandler) setStatus(req api.Context, id string, change func(*v1.MCPServerRequestStatus) error, out *v1.MCPServerRequest) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Get(out, id); err != nil {
			return err
		}
		if err := change(&out.Status); err != nil {
			return err
		}
		return req.Storage.Status().Update(req.Context(), out)
	})
}

func requestState(request v1.MCPServerRequest) types.MCPServerRequestState {
	if request.Status.State == "" {
		return types.MCPServerRequestStatePending
	}
	return request.Status.State
}

// componentName derives a vMCP component name from a server's display name.
func componentName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return cmpOr(strings.Trim(b.String(), "-"), "server")
}

func convertMCPServerRequest(request v1.MCPServerRequest) types.MCPServerRequest {
	converted := types.MCPServerRequest{
		Metadata:                 MetadataFrom(&request),
		MCPServerRequestManifest: request.Spec.Manifest,
		RequesterID:              request.Spec.RequesterID,
		RequesterName:            request.Spec.RequesterName,
		RequesterEmail:           request.Spec.RequesterEmail,
		State:                    requestState(request),
		DecidedBy:                request.Status.DecidedBy,
		DecisionReason:           request.Status.DecisionReason,
		CatalogEntryID:           request.Status.CatalogEntryID,
		VMCPID:                   request.Status.VMCPID,
	}
	if request.Status.DecidedAt != nil {
		converted.DecidedAt = types.NewTime(request.Status.DecidedAt.Time)
	}
	return converted
}

func cmpOr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
