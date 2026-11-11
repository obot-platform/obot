package handlers

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/groupref"
	"gorm.io/gorm"
)

// writeNewSubjects runs write, which saves subjects in place of previous, once the agent subjects it adds are known
// to name agents that exist (checkNewAgentSubjects), and the group subjects it adds are known to reach groups of a
// SCIM connection's auth provider. The write is recorded while it runs, so a deletion of unreferenced groups sees its
// references. See groupref.WriteNewSubjects.
func writeNewSubjects(req api.Context, subjects, previous []types.Subject, write func() error) error {
	if err := checkNewAgentSubjects(req, subjects, previous); err != nil {
		return err
	}
	return groupref.WriteNewSubjects(req.Context(), req.GatewayClient, req.Storage, subjects, previous, write)
}

// checkNewAgentSubjects refuses subjects that add an explicit agent subject ({agent, "<id>"}) to previous, unless the
// agent exists and is not deleted. A disabled agent may be named: disabling an agent keeps what it is granted. The
// agent wildcard names no agent in particular, so it is never looked up.
//
// Only added subjects are checked, as only added group subjects are, so a subject naming an agent deleted since it
// was saved does not block unrelated edits. Such a subject stays in place and matches nobody: a deleted agent cannot
// authenticate, and its ID is never given to another agent.
//
// Manifest validation has already refused agent subjects in the policy systems that cannot hold them, and checked
// that each explicit agent subject's ID is a canonical decimal ID.
func checkNewAgentSubjects(req api.Context, subjects, previous []types.Subject) error {
	var checked []string
	for _, subject := range subjects {
		if subject.Type != types.SubjectTypeAgent || subject.ID == types.SubjectWildcardID ||
			slices.Contains(previous, subject) || slices.Contains(checked, subject.ID) {
			continue
		}
		checked = append(checked, subject.ID)

		// Manifest validation has checked that the ID is canonical, so it parses.
		id, _ := strconv.ParseUint(subject.ID, 10, 0)
		if _, err := req.GatewayClient.GetAgent(req.Context(), uint(id)); errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrBadRequest("agent %q not found", subject.ID)
		} else if err != nil {
			return fmt.Errorf("failed to get agent %s: %w", subject.ID, err)
		}
	}
	return nil
}

// guardrailFilterSubjects returns the subjects of an MCP filter's guardrail as the policy subjects that name the same
// callers, for writeNewSubjects. A subject of an unknown type, such as the legacy selector, names no agent or group,
// so it is left out.
func guardrailFilterSubjects(manifest types.MCPWebhookValidationManifest) []types.Subject {
	if manifest.Guardrail == nil {
		return nil
	}
	var subjects []types.Subject
	for _, s := range manifest.Guardrail.Subjects {
		if subject, ok := s.Subject(); ok {
			subjects = append(subjects, subject)
		}
	}
	return subjects
}
