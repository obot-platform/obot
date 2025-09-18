package threads

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureUpgradeAvailable ensures that a thread's UpgradeAvailable status reflects its ability to
// be upgraded from its source thread.
//
// At a high level, this handler accounts for the following scenarios:
// - A user has copied a template and manually modified the resulting thread and/or its associated resources (UpgradeAvailable -> false)
// - A template has been updated and the thread has not been manually modified (UpgradeAvailable -> true)
// - A thread that a template was created from has changes (UpgradeAvailable -> true)
func (t *Handler) EnsureUpgradeAvailable(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.SourceThreadName == "" {
		// Don't check for non-copied or non-project threads
		return nil
	}

	var (
		source           v1.Thread
		upgradeAvailable bool
	)
	if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Spec.SourceThreadName), &source); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		// We can't find the source thread, don't set the `upgradeAvailable` variable to ensure the status field is false
	} else {
		if !source.Spec.Template {
			// Project was not copied from a template, just check if the latest revision matches the source thread.
			// The source thread will only have a single revision in its ConfigRevisions status field, representing the latest revision.
			upgradeAvailable = source.GetLatestConfigRevision() != thread.GetLatestConfigRevision()
		} else {
			// Project was copied from a template, this means the source thread will have every previously valid revision
			// in its ConfigRevisions status field.
			// In this case, if we can't find the thread's latest revision in the source thread's history,
			// we can assume the thread is either mid-upgrade or has been directly modified by the user.
			// If we find the revision, but it's the latest revision, there's no new upgrade available.
			found, latest := source.HasRevision(thread.GetLatestConfigRevision())
			upgradeAvailable = found && !latest
		}

		upgradeAvailable = !source.Status.UpgradeInProgress && upgradeAvailable
	}

	if thread.Status.UpgradeAvailable == upgradeAvailable {
		// No change, bail out
		return nil
	}

	// Update the status with the new value
	thread.Status.UpgradeAvailable = upgradeAvailable
	return req.Client.Status().Update(req.Ctx, thread)
}

// HandleUpgrade manages the upgrade process for project threads that were copied from a source thread.
func (t *Handler) UpgradeThread(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.SourceThreadName == "" || thread.Spec.ParentThreadName != "" {
		// Only copied top-level projects participate
		return nil
	}

	var source v1.Thread
	if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Spec.SourceThreadName), &source); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		// If the source thread isn't found, we can't upgrade.
		// Unapprove in-progress upgrades and unset target revision.
		if thread.Spec.UpgradeApproved || thread.Spec.TargetConfigRevision != "" {
			thread.Spec.UpgradeApproved = false
			thread.Spec.TargetConfigRevision = ""
			return req.Client.Update(req.Ctx, thread)
		}
	}

	if source.Status.UpgradeInProgress {
		// The source thread is upgrading, wait for it to complete before checking upgrade status
		return nil
	}

	if thread.Status.UpgradeInProgress {
		if sourceRevision := source.GetLatestConfigRevision(); thread.Spec.TargetConfigRevision != sourceRevision {
			// The thread has diverged from the source thread during an upgrade.
			// Bump the target digest and set the upgrade approved flag to restart the upgrade
			thread.Spec.UpgradeApproved = true
			thread.Spec.TargetConfigRevision = sourceRevision
			return req.Client.Update(req.Ctx, thread)
		}

		if threadRevision := thread.GetLatestConfigRevision(); thread.Spec.TargetConfigRevision == threadRevision {
			// Thread is up to date, clear the upgrade in progress flag
			thread.Status.UpgradeInProgress = false
			thread.Status.LastUpgraded = metav1.NewTime(time.Now().UTC())
			return req.Client.Status().Update(req.Ctx, thread)
		}
	}

	sourceRevision := source.GetLatestConfigRevision()
	if !thread.Spec.UpgradeApproved || sourceRevision == "" {
		// Upgrade hasn't been approved or the source thread has no revisions, bail out
		return nil
	}

	// Clear derived statuses to trigger downstream copy controllers
	thread.Status.CopiedTasks = false
	thread.Status.CopiedTools = false
	thread.Status.UpgradeAvailable = false
	thread.Status.SharedKnowledgeSetName = ""
	thread.Status.KnowledgeSetNames = nil
	thread.Status.UpgradeInProgress = true
	if err := req.Client.Status().Update(req.Ctx, thread); err != nil {
		return err
	}

	// Update the thread's spec AFTER clearing derived status.
	// This ensures that if the spec update fails, the thread's status will still be cleared
	// when this handler is called again.
	thread.Spec.Manifest = source.Spec.Manifest
	thread.Spec.TargetConfigRevision = sourceRevision
	thread.Spec.UpgradeApproved = false
	return req.Client.Update(req.Ctx, thread)
}

// EnsurePublicID ensures that the thread has a public ID if it's a project thread that was copied from a template.
func (t *Handler) EnsurePublicID(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.Template || thread.Spec.ParentThreadName != "" || thread.Spec.SourceThreadName == "" {
		return nil
	}

	// Attempt to find the ThreadShare for the template thread
	var threadShare v1.ThreadShare
	if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Spec.SourceThreadName), &threadShare); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	var publicID string
	if threadShare.Spec.Template {
		publicID = threadShare.Spec.PublicID
	}

	if thread.Status.UpgradePublicID == publicID {
		return nil
	}

	thread.Status.UpgradePublicID = publicID
	return req.Client.Status().Update(req.Ctx, thread)
}

// EnsureLatestConfigRevision recalculates the thread's latest config revision and ensures the thread's
// revision history reflects the latest revision.
func (t *Handler) EnsureLatestConfigRevision(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	if !thread.Status.Created || !thread.Spec.Project || thread.Spec.ParentThreadName != "" {
		// Don't compute the config hash for threads that aren't created yet or are non-project child threads
		return nil
	}

	// Fetch all resources required to get the config revision for the thread
	tasks, knowledgeFiles, projectMCPs, mcpServers, mcpServerInstances, err := t.fetchThreadResources(req.Ctx, req.Client, thread)
	if err != nil {
		return err
	}

	// Calculate the config revision
	config := newProjectThreadConfig(
		thread.Spec.Manifest,
		tasks,
		knowledgeFiles,
		projectMCPs,
		mcpServers,
		mcpServerInstances,
	)
	if changed := thread.SetLatestConfigRevision(config.Revision()); !changed {
		// No change, bail out
		return nil
	}

	// Latest revision has changed, update the status with the new digest
	return req.Client.Status().Update(req.Ctx, thread)
}

// fetchThreadResources fetches all the resources related to the given thread.
// Use this method to gather all the objects necessary to compute a thread's config revision.
func (*Handler) fetchThreadResources(ctx context.Context, c kclient.Client, thread *v1.Thread) ([]v1.Workflow, []v1.KnowledgeFile, []v1.ProjectMCPServer, []v1.MCPServer, []v1.MCPServerInstance, error) {
	// Fetch workflows (tasks)
	var tasks v1.WorkflowList
	if err := c.List(ctx, &tasks, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// Fetch knowledge files
	var knowledgeFiles v1.KnowledgeFileList
	if thread.Status.SharedKnowledgeSetName != "" {
		if err := c.List(ctx, &knowledgeFiles, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
			"spec.knowledgeSetName": thread.Status.SharedKnowledgeSetName,
		}); err != nil {
			return nil, nil, nil, nil, nil, err
		}
	}

	// Fetch project MCP servers
	var projectMCPs v1.ProjectMCPServerList
	if err := c.List(ctx, &projectMCPs, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// Fetch the user's MCP servers
	var mcpServers v1.MCPServerList
	if err := c.List(ctx, &mcpServers, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.userID": thread.Spec.UserID,
	}); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// Fetch the user's MCP server instances
	var mcpServerInstances v1.MCPServerInstanceList
	if err := c.List(ctx, &mcpServerInstances, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.userID": thread.Spec.UserID,
	}); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return tasks.Items, knowledgeFiles.Items, projectMCPs.Items, mcpServers.Items, mcpServerInstances.Items, nil
}

// newProjectThreadConfig returns a resource that can be used to compute a thread config revision.
func newProjectThreadConfig(
	manifest types.ThreadManifest,
	tasks []v1.Workflow,
	knowledgeFiles []v1.KnowledgeFile,
	projectMCPs []v1.ProjectMCPServer,
	mcpServers []v1.MCPServer,
	mcpServerInstances []v1.MCPServerInstance,
) projectThreadConfig {
	config := projectThreadConfig{
		Intro:         manifest.IntroductionMessage,
		Prompt:        manifest.Prompt,
		ModelProvider: manifest.ModelProvider,
		Model:         manifest.Model,
	}

	// Add sorted starter messages and tools
	config.Starters = make([]string, len(manifest.StarterMessages))
	copy(config.Starters, manifest.StarterMessages)
	slices.Sort(config.Starters)

	config.Tools = make([]string, len(manifest.Tools))
	copy(config.Tools, manifest.Tools)
	slices.Sort(config.Tools)

	mcpServerSpecs := make(map[string]v1.MCPServerSpec, len(mcpServers))
	for _, mcpServer := range mcpServers {
		spec := v1.MCPServerSpec{
			Manifest:                  mcpServer.Spec.Manifest,
			UnsupportedTools:          mcpServer.Spec.UnsupportedTools,
			Alias:                     mcpServer.Spec.Alias,
			PowerUserWorkspaceID:      mcpServer.Spec.PowerUserWorkspaceID,
			MCPCatalogID:              mcpServer.Spec.MCPCatalogID,
			MCPServerCatalogEntryName: mcpServer.Spec.MCPServerCatalogEntryName,
		}
		slices.Sort(spec.UnsupportedTools)

		mcpServerSpecs[mcpServer.Name] = spec
	}

	mcpServerInstancesSpecs := make(map[string]v1.MCPServerInstanceSpec, len(mcpServerInstances))
	for _, mcpServerInstance := range mcpServerInstances {
		spec := v1.MCPServerInstanceSpec{
			MCPServerName:             mcpServerInstance.Spec.MCPServerName,
			MCPCatalogName:            mcpServerInstance.Spec.MCPCatalogName,
			MCPServerCatalogEntryName: mcpServerInstance.Spec.MCPServerCatalogEntryName,
			PowerUserWorkspaceID:      mcpServerInstance.Spec.PowerUserWorkspaceID,
		}
		mcpServerInstancesSpecs[mcpServerInstance.Name] = spec
	}

	// Build allowed MCP tools data with normalized MCP IDs
	config.AllowedMCPTools = make(map[string][]string, len(manifest.AllowedMCPTools))
	for projectMCPID, toolNames := range manifest.AllowedMCPTools {
		// Normalize MCP ID to catalog entry name
		mcpID := projectMCPID
		if spec, ok := mcpServerSpecs[mcpID]; ok {
			mcpID = strings.Join([]string{spec.MCPCatalogID, spec.MCPServerCatalogEntryName}, "/")
		}

		// Sort tool names for deterministic ordering
		tools := make([]string, len(toolNames))
		copy(tools, toolNames)
		slices.Sort(tools)

		config.AllowedMCPTools[mcpID] = tools
	}

	config.TaskDigests = make([]string, 0, len(tasks))
	for _, task := range tasks {
		manifest := task.Spec.Manifest
		// Clear the alias, this is a unique randomly generated value that will differ between
		// a copied task and the original.
		manifest.Alias = ""
		config.TaskDigests = append(config.TaskDigests, hash.Digest(manifest))
	}
	slices.Sort(config.TaskDigests)

	// Build knowledge data
	config.KnowledgeFileDigests = make([]string, 0, len(knowledgeFiles))
	for _, f := range knowledgeFiles {
		config.KnowledgeFileDigests = append(config.KnowledgeFileDigests, hash.ID(f.Spec.FileName, f.Spec.Checksum))
	}
	// Sort for deterministic ordering
	slices.Sort(config.KnowledgeFileDigests)

	// Build project MCP servers data
	config.ProjectMCPDigests = make([]string, 0, len(projectMCPs))
	for _, pms := range projectMCPs {
		var (
			specHash string
			mcpID    = pms.Spec.Manifest.MCPID
		)
		if spec, ok := mcpServerSpecs[mcpID]; ok {
			specHash = hash.Digest(spec)
		} else if spec, ok := mcpServerInstancesSpecs[mcpID]; ok {
			specHash = hash.Digest(spec)
		}

		if specHash == "" {
			// Skip missing specs, this will result in a hash mismatch
			continue
		}

		config.ProjectMCPDigests = append(config.ProjectMCPDigests, specHash)
	}
	slices.Sort(config.ProjectMCPDigests)

	return config
}

// projectThreadConfig represents a project thread's configuration and is used to compute a revision
// that can be used to determine if a thread has diverged from its source thread.
type projectThreadConfig struct {
	// Manifest data
	Intro         string   `json:"intro"`
	Starters      []string `json:"starters"`
	Prompt        string   `json:"prompt"`
	ModelProvider string   `json:"modelProvider"`
	Model         string   `json:"model"`

	// AllowedMCPTools contains a map of normalized project MCP server IDs to a map of sorted tool names.
	AllowedMCPTools map[string][]string `json:"allowedMCPTools"`

	// Tools is the set of tool names in the project manifest
	Tools []string `json:"tools"`

	// HashedTasks is a sorted list containing the hashed manifest of tasks belonging to the project thread.
	// Each hash excludes the alias so that display-only changes don't affect the resulting hash.
	TaskDigests []string `json:"taskDigests"`

	// HashedKnowledgeFiles is a sorted list containing the hashed knowledge files belonging to the project thread.
	KnowledgeFileDigests []string `json:"knowledgeFileDigests"`

	// Project MCP servers (sorted by catalog entry name)
	ProjectMCPDigests []string `json:"projectMCPDigests"`
}

// Revision returns a revision string created by taking the digest of the projectThreadConfig.
//
// Revision strings produced by this method are deterministic and can be used to check for
// relevant differences between a thread and its source thread.
func (c projectThreadConfig) Revision() string {
	return hash.Digest(c)
}
