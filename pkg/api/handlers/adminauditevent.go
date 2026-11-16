package handlers

import (
	"strconv"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
)

const (
	adminAuditEventDefaultLimit = 100
	adminAuditEventMaxLimit     = 1000
)

// AdminAuditEventHandler serves the read API of the admin change audit: who changed agents, their API keys, the policies
// and profiles that name agents, and profile configuration and OAuth grants. It is read-only, and only administrators,
// owners and auditors are granted it (pkg/api/authz); agents never reach it.
//
// It is served whether or not authentication is enabled, as the agent admin API is.
type AdminAuditEventHandler struct{}

func NewAdminAuditEventHandler() *AdminAuditEventHandler {
	return &AdminAuditEventHandler{}
}

// RegisterRoutes registers the route with handle.
func (h *AdminAuditEventHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("GET /api/admin-audit-events", h.List)
}

// List lists events, newest first, filtered by resourceKind, resourceId, actor (a user ID, or "system") and action,
// each of which may be repeated or comma-separated to match any of its values, and by since and until (RFC 3339; since is
// inclusive, until exclusive), and paged with limit (default 100, at most 1000) and
// offset. A filter that is not valid is 400, rather than ignored, so that a mistyped one never reads as no events.
func (h *AdminAuditEventHandler) List(req api.Context) error {
	q := req.URL.Query()
	opts := gclient.AdminAuditEventOptions{
		ResourceKinds: parseStringList(q, "resourceKind"),
		ResourceIDs:   parseStringList(q, "resourceId"),
		Actors:        parseStringList(q, "actor"),
		Actions:       parseStringList(q, "action"),
		Limit:         adminAuditEventDefaultLimit,
	}
	for name, dest := range map[string]*time.Time{"since": &opts.Since, "until": &opts.Until} {
		if !q.Has(name) {
			continue
		}
		t, err := time.Parse(time.RFC3339, q.Get(name))
		if err != nil {
			return types.NewErrBadRequest("invalid %s %q: must be an RFC 3339 time", name, q.Get(name))
		}
		*dest = t
	}
	if q.Has("limit") {
		limit, err := strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > adminAuditEventMaxLimit {
			return types.NewErrBadRequest("invalid limit %q: must be between 1 and %d", q.Get("limit"), adminAuditEventMaxLimit)
		}
		opts.Limit = limit
	}
	if q.Has("offset") {
		offset, err := strconv.Atoi(q.Get("offset"))
		if err != nil || offset < 0 {
			return types.NewErrBadRequest("invalid offset %q: must be 0 or more", q.Get("offset"))
		}
		opts.Offset = offset
	}

	events, total, err := req.GatewayClient.ListAdminAuditEvents(req.Context(), opts)
	if err != nil {
		return err
	}

	// Each actor is looked up once. A user who has been deleted is still named, by the record the gateway keeps.
	names := map[string]string{}
	items := make([]types.AdminAuditEvent, 0, len(events))
	for _, event := range events {
		name, ok := names[event.ActorUserID]
		// Only a user is looked up: the bootstrap administrator has no user row, and the system actor is not a
		// principal.
		if p, parseErr := principal.Parse(event.ActorUserID); !ok && parseErr == nil && p.Kind == principal.KindUser {
			if user, err := req.GatewayClient.UserByIDIncludeDeleted(req.Context(), event.ActorUserID); err == nil {
				name = user.DisplayName
			}
		}
		names[event.ActorUserID] = name
		items = append(items, gtypes.ConvertAdminAuditEvent(event, name))
	}
	return req.Write(types.AdminAuditEventList{Items: items, Total: total, Limit: opts.Limit, Offset: opts.Offset})
}
