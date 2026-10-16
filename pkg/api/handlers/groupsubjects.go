package handlers

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/groupref"
)

// writeNewGroupSubjects runs write, which saves subjects in place of previous, once the group subjects it adds are
// known to reach groups of a SCIM connection's auth provider. The write is recorded while it runs, so a deletion of
// unreferenced groups sees its references. See groupref.WriteNewSubjects.
func writeNewGroupSubjects(req api.Context, subjects, previous []types.Subject, write func() error) error {
	return groupref.WriteNewSubjects(req.Context(), req.GatewayClient, req.Storage, subjects, previous, write)
}

// writeNewGroupIDs runs write, which saves subjects in place of previous, through the group reference guard, as
// writeNewGroupSubjects does, for the guardrail filters whose subjects are a type and an ID each.
func writeNewGroupIDs(req api.Context, groupIDs, previous []string, write func() error) error {
	var added []string
	for _, id := range groupIDs {
		if !slices.Contains(previous, id) && !slices.Contains(added, id) {
			added = append(added, id)
		}
	}
	return groupref.WriteNewGroups(req.Context(), req.GatewayClient, req.Storage, added, write)
}

// guardrailFilterGroups returns the groups an MCP filter's guardrail is assigned to.
func guardrailFilterGroups(manifest types.MCPWebhookValidationManifest) []string {
	if manifest.Guardrail == nil {
		return nil
	}
	var ids []string
	for _, subject := range manifest.Guardrail.Subjects {
		if subject.Type == types.MCPGuardrailSubjectGroup {
			ids = append(ids, subject.ID)
		}
	}
	return ids
}
