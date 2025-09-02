package threads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SourceThreadUpgradeAvailable sets SourceThreadUpgradeAvailable when the thread's configuration
// has diverged from the source thread's configuration because the source thread has been updated.
func (t *Handler) SourceThreadUpgradeAvailable(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.SourceThreadName == "" {
		// Don't check for non-copied or non-project threads
		return nil
	}

	var source v1.Thread
	if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Spec.SourceThreadName), &source); err != nil {
		return err
	}

	upgradeAvailable := thread.Status.ThreadConfigHash != source.Status.ThreadConfigHash
	if thread.Status.SourceThreadUpgradeAvailable == upgradeAvailable {
		return nil
	}

	thread.Status.SourceThreadUpgradeAvailable = upgradeAvailable
	return req.Client.Status().Update(req.Ctx, thread)
}

// HandleSourceThreadUpgrade watches for user approval and, when present,
// resets the derived state then copies spec fields from the source thread to kick off re-sync.
func (t *Handler) HandleSourceThreadUpgrade(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.SourceThreadName == "" || thread.Spec.ParentThreadName != "" {
		// Only copied top-level projects participate
		return nil
	}

	var source v1.Thread
	if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Spec.SourceThreadName), &source); err != nil {
		return err
	}

	if thread.Status.UpgradeInProgress && thread.Status.ThreadConfigHash == source.Status.ThreadConfigHash {
		thread.Status.UpgradeInProgress = false
		return req.Client.Status().Update(req.Ctx, thread)
	}

	if thread.Annotations[v1.ThreadUpgradeApprovedAnnotation] != "true" || thread.Status.UpgradeInProgress {
		return nil
	}

	// Copy spec fields from source
	desired := thread.DeepCopy()
	delete(desired.Annotations, v1.ThreadUpgradeApprovedAnnotation)
	desired.Spec.Manifest = source.Spec.Manifest

	// Clear derived statuses to trigger downstream copy controllers
	desired.Status.CopiedTasks = false
	desired.Status.CopiedTools = false
	desired.Status.ThreadConfigHash = ""
	desired.Status.SourceThreadUpgradeAvailable = false
	desired.Status.SharedKnowledgeSetName = ""
	desired.Status.KnowledgeSetNames = nil
	desired.Status.UpgradeInProgress = true

	if err := req.Client.Update(req.Ctx, desired); err != nil {
		return err
	}
	if err := req.Client.Status().Update(req.Ctx, desired); err != nil {
		return err
	}

	return nil
}

// ComputeThreadConfigHash computes a deterministic hash over the thread's configuration
// (introduction message, starter messages, prompt, model provider/model, tool set,
// tasks, knowledge files, and the set of project MCP servers.
func (t *Handler) ComputeThreadConfigHash(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	if !thread.Status.Created || !thread.Spec.Project || thread.Spec.ParentThreadName != "" {
		// Don't compute the config hash for threads that aren't created yet or are non-project child threads
		return nil
	}

	if thread.Spec.SourceThreadName != "" && (!thread.Status.CopiedTasks || !thread.Status.CopiedTools) {
		// Don't compute hash for copied project threads until tasks and tools are copied
		return nil
	}

	manifestHash, err := hashManifest(&thread.Spec.Manifest)
	if err != nil {
		return err
	}

	toolsHash, err := hashTools(req.Ctx, req.Client, thread)
	if err != nil {
		return err
	}

	tasksHash, err := hashTasks(req.Ctx, req.Client, thread)
	if err != nil {
		return err
	}

	knowledgeHash, err := hashKnowledge(req.Ctx, req.Client, thread)
	if err != nil {
		return err
	}

	pmsHash, err := hashProjectMCPs(req.Ctx, req.Client, thread)
	if err != nil {
		return err
	}

	var (
		combined = manifestHash + toolsHash + tasksHash + knowledgeHash + pmsHash
		sum      = sha256.Sum256([]byte(combined))
		newHash  = hex.EncodeToString(sum[:])
	)

	if thread.Status.ThreadConfigHash == newHash {
		// No change, bail out
		return nil
	}

	// Update the status with the new hash
	thread.Status.ThreadConfigHash = newHash
	return req.Client.Status().Update(req.Ctx, thread)
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return hashBytes(b), nil
}

func hashManifest(manifest *types.ThreadManifest) (string, error) {
	if manifest == nil {
		return hashBytes(nil), nil
	}
	slices.Sort(manifest.StarterMessages)

	state := struct {
		Intro         string   `json:"intro"`
		Starters      []string `json:"starters"`
		Prompt        string   `json:"prompt"`
		ModelProvider string   `json:"modelProvider"`
		Model         string   `json:"model"`
	}{
		Intro:         manifest.IntroductionMessage,
		Starters:      manifest.StarterMessages,
		Prompt:        manifest.Prompt,
		ModelProvider: manifest.ModelProvider,
		Model:         manifest.Model,
	}

	return hashJSON(state)
}

func hashTools(ctx context.Context, c kclient.Client, thread *v1.Thread) (string, error) {
	var toolList v1.ToolList
	if err := c.List(ctx, &toolList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return "", err
	}

	toolStrings := make([]string, 0, len(toolList.Items))
	for _, tool := range toolList.Items {
		b, err := json.Marshal(tool.Spec.Manifest)
		if err != nil {
			return "", err
		}
		toolStrings = append(toolStrings, string(b))
	}

	sort.Strings(toolStrings)
	combined := strings.Join(toolStrings, "")
	return hashBytes([]byte(combined)), nil
}

func hashTasks(ctx context.Context, c kclient.Client, thread *v1.Thread) (string, error) {
	var wfList v1.WorkflowList
	if err := c.List(ctx, &wfList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return "", err
	}

	taskStrings := make([]string, 0, len(wfList.Items))
	for _, wf := range wfList.Items {
		manifest := wf.Spec.Manifest
		// Exclude alias so identity-only changes don't affect the hash
		manifest.Alias = ""

		b, err := json.Marshal(manifest)
		if err != nil {
			return "", err
		}

		taskStrings = append(taskStrings, string(b))
	}

	sort.Strings(taskStrings)
	combined := strings.Join(taskStrings, "")
	return hashBytes([]byte(combined)), nil
}

func hashKnowledge(ctx context.Context, c kclient.Client, thread *v1.Thread) (string, error) {
	if thread.Status.SharedKnowledgeSetName == "" {
		return "", nil
	}

	var kfList v1.KnowledgeFileList
	if err := c.List(ctx, &kfList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.knowledgeSetName": thread.Status.SharedKnowledgeSetName,
	}); err != nil {
		return "", err
	}

	knowledgeKeys := make([]string, 0, len(kfList.Items))
	for _, f := range kfList.Items {
		key := f.Spec.FileName + "\x00" + f.Spec.Checksum
		knowledgeKeys = append(knowledgeKeys, key)
	}

	sort.Strings(knowledgeKeys)
	combined := strings.Join(knowledgeKeys, "")
	return hashBytes([]byte(combined)), nil
}

func getCatalogEntryName(ctx context.Context, c kclient.Client, namespace string, mcpID string) (string, error) {
	if system.IsMCPServerID(mcpID) {
		var srv v1.MCPServer
		if err := c.Get(ctx, router.Key(namespace, mcpID), &srv); err != nil {
			return "", err
		}
		return srv.Spec.MCPServerCatalogEntryName, nil
	}
	return mcpID, nil
}

func hashProjectMCPs(ctx context.Context, c kclient.Client, thread *v1.Thread) (string, error) {
	var pmsList v1.ProjectMCPServerList
	if err := c.List(ctx, &pmsList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return "", err
	}

	entries := make([]string, 0, len(pmsList.Items))
	for _, pms := range pmsList.Items {
		entry, err := getCatalogEntryName(ctx, c, thread.Namespace, pms.Spec.Manifest.MCPID)
		if err != nil {
			return "", err
		}
		entries = append(entries, entry)
	}

	sort.Strings(entries)
	combined := strings.Join(entries, "")
	return hashBytes([]byte(combined)), nil
}
