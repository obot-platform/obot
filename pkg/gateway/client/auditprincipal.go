package client

import (
	"context"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"gorm.io/gorm"
)

// The activity tables that attribute their rows by principal (llm_audit_logs, mcp_audit_logs and run_token_activities)
// share the columns user_id, principal_kind, principal_id and on_behalf_of, so the predicates below apply to any of
// them. An agent's rows have no user_id, and a user's rows have no principal_id, so a row is an agent's when its
// principal_kind is "agent", and is otherwise the user in its user_id. An agent's row that it made for a user has the
// user in on_behalf_of, and is only a user's row when a reader asks for those too.

const (
	// agentDisplayNameLookupBatchSize bounds how many agent IDs one query looks up.
	agentDisplayNameLookupBatchSize = 500

	// ownerExpr is the SQL expression of a row's principal in its storage encoding: "agent:<id>" for an agent's row,
	// and otherwise its user_id.
	ownerExpr = "CASE WHEN principal_kind = 'agent' THEN 'agent:' || principal_id ELSE user_id END"
)

// ownerFilter selects the rows of any of the principals that owners name, each in its storage encoding: the ID of a
// user, or "agent:<id>" for an agent. It reports false when owners is empty. A value that is not an encoded agent is
// matched against user_id as it was before agents existed, whatever else it is, and with includeOnBehalfOf against
// on_behalf_of as well, so that a user's rows include those agents made on their behalf.
func ownerFilter(owners []string, includeOnBehalfOf bool) (string, []any, bool) {
	var users, agentIDs []string
	for _, owner := range owners {
		if agentID, ok := principal.ParseAgent(owner); ok {
			agentIDs = append(agentIDs, agentID)
		} else {
			users = append(users, owner)
		}
	}

	var (
		conditions []string
		args       []any
	)
	if len(users) > 0 {
		conditions, args = append(conditions, "user_id IN ?"), append(args, users)
		if includeOnBehalfOf {
			conditions, args = append(conditions, "on_behalf_of IN ?"), append(args, users)
		}
	}
	if len(agentIDs) > 0 {
		conditions, args = append(conditions, "(principal_kind = 'agent' AND principal_id IN ?)"), append(args, agentIDs)
	}
	if len(conditions) == 0 {
		return "", nil, false
	}
	return "(" + strings.Join(conditions, " OR ") + ")", args, true
}

// applyOwnerFilter narrows db to the rows of the principals that owners name; see ownerFilter.
func applyOwnerFilter(db *gorm.DB, owners []string, includeOnBehalfOf bool) *gorm.DB {
	if condition, args, ok := ownerFilter(owners, includeOnBehalfOf); ok {
		return db.Where(condition, args...)
	}
	return db
}

// applyOnBehalfOfFilter narrows db to the rows that agents made on behalf of any of the users that onBehalfOf names by
// ID. Unlike includeOnBehalfOf, which widens a filter by user, it is a filter of its own that every other filter
// narrows further, so that a reader can ask for what one agent did for one user. A user's own rows, and an agent's
// rows for itself, have no on_behalf_of and never match.
func applyOnBehalfOfFilter(db *gorm.DB, onBehalfOf []string) *gorm.DB {
	if len(onBehalfOf) > 0 {
		db = db.Where("on_behalf_of IN ?", onBehalfOf)
	}
	return db
}

// applyActorFilters narrows db to the rows made by the given kinds of principal ("user" or "agent"). An agent is
// selected by its owner encoding through applyOwnerFilter.
func applyActorFilters(db *gorm.DB, kinds []string) *gorm.DB {
	if len(kinds) > 0 {
		db = db.Where("principal_kind IN ?", kinds)
	}
	return db
}

// agentDisplayNameCondition selects the rows of agents whose display name matches the LIKE pattern, deleted agents
// included: their rows keep their history.
func agentDisplayNameCondition(like string) string {
	// The agents table is read as a plain table, not through the soft-delete scope of the model.
	return "(principal_kind = 'agent' AND principal_id IN (SELECT CAST(id AS TEXT) FROM agents WHERE display_name " + like + " ?))"
}

// agentDisplayNames returns the display names of the agents with the given IDs, deleted agents included, so that a
// row stays readable after its agent is deleted. An agent that has no row is left out.
func (c *Client) agentDisplayNames(ctx context.Context, ids map[uint]struct{}) (map[uint]string, error) {
	names := make(map[uint]string, len(ids))
	unique := make([]uint, 0, len(ids))
	for id := range ids {
		unique = append(unique, id)
	}
	for offset := 0; offset < len(unique); offset += agentDisplayNameLookupBatchSize {
		batch := unique[offset:min(offset+agentDisplayNameLookupBatchSize, len(unique))]
		var agents []types.Agent
		if err := c.db.WithContext(ctx).Unscoped().Select("id", "display_name").Where("id IN ?", batch).Find(&agents).Error; err != nil {
			return nil, err
		}
		for _, agent := range agents {
			names[agent.ID] = agent.DisplayName
		}
	}
	return names, nil
}

// fillAgentNames fills in the current display name of the agent of each row that an agent made. ref returns a row's
// principal kind and ID, and where its agent's display name goes.
func fillAgentNames[T any](ctx context.Context, c *Client, rows []T, ref func(row *T) (kind, id string, name *string)) error {
	agentOfRow := map[int]uint{}
	ids := map[uint]struct{}{}
	for i := range rows {
		kind, id, _ := ref(&rows[i])
		if !principal.IsAgentKind(kind) {
			continue
		}
		if parsed, err := strconv.ParseUint(id, 10, 0); err == nil {
			agentOfRow[i] = uint(parsed)
			ids[uint(parsed)] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	names, err := c.agentDisplayNames(ctx, ids)
	if err != nil {
		return err
	}
	for i, id := range agentOfRow {
		_, _, name := ref(&rows[i])
		*name = names[id]
	}
	return nil
}

// enrichLLMAuditLogAgentNames fills in the current display name of the agent of each log that an agent made.
func (c *Client) enrichLLMAuditLogAgentNames(ctx context.Context, logs []types.LLMAuditLog) error {
	return fillAgentNames(ctx, c, logs, func(log *types.LLMAuditLog) (string, string, *string) {
		return log.PrincipalKind, log.PrincipalID, &log.AgentDisplayName
	})
}

// enrichMCPAuditLogAgentNames fills in the current display name of the agent of each log that an agent made.
func (c *Client) enrichMCPAuditLogAgentNames(ctx context.Context, logs []types.MCPAuditLog) error {
	return fillAgentNames(ctx, c, logs, func(log *types.MCPAuditLog) (string, string, *string) {
		return log.PrincipalKind, log.PrincipalID, &log.AgentDisplayName
	})
}
