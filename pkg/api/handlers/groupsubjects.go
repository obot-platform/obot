package handlers

import (
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
