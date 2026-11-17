package controller

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// recordedLog is one record a recordingHandler kept, with its attributes as strings.
type recordedLog struct {
	level   slog.Level
	message string
	attrs   map[string]string
}

// recordingHandler keeps every record logged through it.
type recordingHandler struct {
	mu      sync.Mutex
	records []recordedLog
}

// failingLists fails List for one kind of list, and serves every other from the wrapped client.
type failingLists struct {
	kclient.Client
	fail kclient.ObjectList
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := map[string]string{}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.String()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, recordedLog{level: record.Level, message: record.Message, attrs: attrs})
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordingHandler) WithGroup(string) slog.Handler { return h }

// warnings returns the kind and name of each warning about a user wildcard subject.
func (h *recordingHandler) warnings() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var warned []string
	for _, record := range h.records {
		if record.level == slog.LevelWarn {
			warned = append(warned, record.attrs["kind"]+"/"+record.attrs["name"])
		}
	}
	slices.Sort(warned)
	return warned
}

func (h *recordingHandler) errors() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var messages []string
	for _, record := range h.records {
		if record.level == slog.LevelError {
			messages = append(messages, record.message)
		}
	}
	return messages
}

func (c *failingLists) List(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) error {
	if c.fail != nil && sameListType(list, c.fail) {
		return errors.New("injected list failure")
	}
	return c.Client.List(ctx, list, opts...)
}

func sameListType(a, b kclient.ObjectList) bool {
	switch a.(type) {
	case *v1.VMCPList:
		_, ok := b.(*v1.VMCPList)
		return ok
	case *v1.ModelAccessPolicyList:
		_, ok := b.(*v1.ModelAccessPolicyList)
		return ok
	case *v1.MCPWebhookValidationList:
		_, ok := b.(*v1.MCPWebhookValidationList)
		return ok
	case *v1.SkillAccessRuleList:
		_, ok := b.(*v1.SkillAccessRuleList)
		return ok
	}
	return false
}

// captureLogs sends the default logger to a recordingHandler until the test ends.
func captureLogs(t *testing.T) *recordingHandler {
	t.Helper()
	handler := &recordingHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return handler
}

func userWildcardPolicyObjects() []kclient.Object {
	meta := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace}
	}
	users := types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}
	everyone := types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}
	oneUser := types.Subject{Type: types.SubjectTypeUser, ID: "7"}
	agents := types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}

	return []kclient.Object{
		&v1.VMCP{ObjectMeta: meta("vmcp-users"), Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Users vMCP",
			Profiles: []types.VMCPProfile{
				{Name: "agents", Subjects: []types.Subject{agents}},
				{Name: "users", Subjects: []types.Subject{users}},
			},
		}}},
		&v1.VMCP{ObjectMeta: meta("vmcp-everyone"), Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Everyone vMCP",
			Profiles:    []types.VMCPProfile{{Name: "everyone", Subjects: []types.Subject{everyone}}},
		}}},
		&v1.ModelAccessPolicy{ObjectMeta: meta("map-users"), Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
			DisplayName: "Users models",
			Subjects:    []types.Subject{users},
		}}},
		&v1.ModelAccessPolicy{ObjectMeta: meta("map-one-user"), Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
			DisplayName: "One user's models",
			Subjects:    []types.Subject{oneUser},
		}}},
		&v1.SkillAccessRule{ObjectMeta: meta("sar-users"), Spec: v1.SkillAccessRuleSpec{Manifest: types.SkillAccessRuleManifest{
			DisplayName: "Users skills",
			Subjects:    []types.Subject{users},
		}}},
		&v1.SkillAccessRule{ObjectMeta: meta("sar-everyone"), Spec: v1.SkillAccessRuleSpec{Manifest: types.SkillAccessRuleManifest{
			DisplayName: "Everyone's skills",
			Subjects:    []types.Subject{everyone},
		}}},
		&v1.MCPWebhookValidation{ObjectMeta: meta("filter-users"), Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			Name: "Users filter",
			Guardrail: &types.MCPGuardrailFilterConfig{Subjects: []types.MCPGuardrailFilterSubject{
				{Type: types.MCPGuardrailSubjectUser, ID: types.SubjectWildcardID},
			}},
		}}},
		&v1.MCPWebhookValidation{ObjectMeta: meta("filter-everyone"), Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			Name: "Everyone filter",
			Guardrail: &types.MCPGuardrailFilterConfig{Subjects: []types.MCPGuardrailFilterSubject{
				{Type: types.MCPGuardrailSubjectEveryone},
			}},
		}}},
		&v1.MCPWebhookValidation{ObjectMeta: meta("webhook"), Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			Name: "Webhook filter",
		}}},
		// Access control rules keep the old meaning of the user wildcard, which is no one, so they are not reported.
		&v1.AccessControlRule{ObjectMeta: meta("acr-users"), Spec: v1.AccessControlRuleSpec{Manifest: types.AccessControlRuleManifest{
			DisplayName: "Users catalog access",
			Subjects:    []types.Subject{users},
		}}},
	}
}

func TestWarnUserWildcardSubjectsReportsEachPolicyThatNamesEveryUser(t *testing.T) {
	logs := captureLogs(t)
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(userWildcardPolicyObjects()...).Build()

	warnUserWildcardSubjects(t.Context(), client)

	assert.Equal(t, []string{
		"MCPWebhookValidation/filter-users",
		"ModelAccessPolicy/map-users",
		"SkillAccessRule/sar-users",
		"VMCP/vmcp-users",
	}, logs.warnings())
	assert.Empty(t, logs.errors())

	logs.mu.Lock()
	defer logs.mu.Unlock()
	for _, record := range logs.records {
		if record.attrs["kind"] == "VMCP" {
			assert.Equal(t, "profile users", record.attrs["detail"], "the warning names the vMCP profile")
			assert.Equal(t, "Users vMCP", record.attrs["displayName"])
		}
	}
}

func TestWarnUserWildcardSubjectsIsSilentWithoutUserWildcards(t *testing.T) {
	logs := captureLogs(t)
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()

	warnUserWildcardSubjects(t.Context(), client)

	assert.Empty(t, logs.warnings())
	assert.Empty(t, logs.errors())
}

func TestWarnUserWildcardSubjectsKeepsCheckingAfterAListFails(t *testing.T) {
	for _, tc := range []struct {
		name         string
		fail         kclient.ObjectList
		wantWarnings []string
	}{
		{
			name:         "vMCPs",
			fail:         &v1.VMCPList{},
			wantWarnings: []string{"MCPWebhookValidation/filter-users", "ModelAccessPolicy/map-users", "SkillAccessRule/sar-users"},
		},
		{
			name:         "model access policies",
			fail:         &v1.ModelAccessPolicyList{},
			wantWarnings: []string{"MCPWebhookValidation/filter-users", "SkillAccessRule/sar-users", "VMCP/vmcp-users"},
		},
		{
			name:         "MCP filters",
			fail:         &v1.MCPWebhookValidationList{},
			wantWarnings: []string{"ModelAccessPolicy/map-users", "SkillAccessRule/sar-users", "VMCP/vmcp-users"},
		},
		{
			name:         "skill access rules",
			fail:         &v1.SkillAccessRuleList{},
			wantWarnings: []string{"MCPWebhookValidation/filter-users", "ModelAccessPolicy/map-users", "VMCP/vmcp-users"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			client := &failingLists{
				Client: fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(userWildcardPolicyObjects()...).Build(),
				fail:   tc.fail,
			}

			require.NotPanics(t, func() { warnUserWildcardSubjects(t.Context(), client) })

			assert.Equal(t, tc.wantWarnings, logs.warnings())
			assert.Len(t, logs.errors(), 1, "the failed list is logged as an error")
		})
	}
}
