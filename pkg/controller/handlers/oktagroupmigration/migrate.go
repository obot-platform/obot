package oktagroupmigration

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

// Migrate updates group subject IDs in AccessControlRule, SkillAccessRule, and ModelAccessPolicy CRDs
// using the old -> new mapping stored in the OktaGroupMigration task object.
// On success, the task object is deleted. On failure, the controller retries automatically.
func (h *Handler) Migrate(req router.Request, _ router.Response) error {
	migration := req.Object.(*v1.OktaGroupMigration)

	if err := migrateAccessControlRules(req, migration.Spec.IDMapping); err != nil {
		return err
	}
	if err := migrateSkillAccessRules(req, migration.Spec.IDMapping); err != nil {
		return err
	}
	if err := migrateModelAccessPolicies(req, migration.Spec.IDMapping); err != nil {
		return err
	}

	return req.Delete(migration)
}

func migrateAccessControlRules(req router.Request, idMap map[string]string) error {
	var list v1.AccessControlRuleList
	if err := req.Client.List(req.Ctx, &list); err != nil {
		return fmt.Errorf("okta group migration: failed to list AccessControlRules: %w", err)
	}
	for i := range list.Items {
		if updateSubjects(list.Items[i].Spec.Manifest.Subjects, idMap) {
			if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
				return fmt.Errorf("okta group migration: failed to update AccessControlRule %s: %w", list.Items[i].Name, err)
			}
		}
	}
	return nil
}

func migrateSkillAccessRules(req router.Request, idMap map[string]string) error {
	var list v1.SkillAccessRuleList
	if err := req.Client.List(req.Ctx, &list); err != nil {
		return fmt.Errorf("okta group migration: failed to list SkillAccessRules: %w", err)
	}
	for i := range list.Items {
		if updateSubjects(list.Items[i].Spec.Manifest.Subjects, idMap) {
			if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
				return fmt.Errorf("okta group migration: failed to update SkillAccessRule %s: %w", list.Items[i].Name, err)
			}
		}
	}
	return nil
}

func migrateModelAccessPolicies(req router.Request, idMap map[string]string) error {
	var list v1.ModelAccessPolicyList
	if err := req.Client.List(req.Ctx, &list); err != nil {
		return fmt.Errorf("okta group migration: failed to list ModelAccessPolicies: %w", err)
	}
	for i := range list.Items {
		if updateSubjects(list.Items[i].Spec.Manifest.Subjects, idMap) {
			if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
				return fmt.Errorf("okta group migration: failed to update ModelAccessPolicy %s: %w", list.Items[i].Name, err)
			}
		}
	}
	return nil
}

// updateSubjects replaces old-format group IDs with new-format IDs in a subject list.
// Returns true if any subjects were modified.
func updateSubjects(subjects []types2.Subject, idMap map[string]string) bool {
	changed := false
	for i, s := range subjects {
		if s.Type != types2.SubjectTypeGroup {
			continue
		}
		if newID, ok := idMap[s.ID]; ok {
			subjects[i].ID = newID
			changed = true
		}
	}
	return changed
}
