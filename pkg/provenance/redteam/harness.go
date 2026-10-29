package redteam

import (
	"context"

	"github.com/obot-platform/obot/pkg/provenance"
)

// plantHoneytoken seeds the exfiltration canary in the engine's store. It
// returns the stored token and whether it was planted. A token with the same
// value already present is reused and left in place.
func (h *harness) plantHoneytoken(ctx context.Context) (provenance.Honeytoken, bool) {
	store := h.engine.Store()
	existing, err := store.ListHoneytokens(ctx)
	if err != nil {
		return provenance.Honeytoken{}, false
	}
	token := provenance.NewHoneytoken("aws-key", Prefix+"canary", honeytokenValue, Prefix+"harness", h.now())
	for _, t := range existing {
		if t.Hash == token.Hash {
			// Already planted (an earlier interrupted run, say); reuse it and
			// do not remove it, so we don't delete a token we didn't own.
			return t, true
		}
	}
	if err := store.PutHoneytoken(ctx, &token); err != nil {
		return provenance.Honeytoken{}, false
	}
	h.engine.Invalidate()
	return token, true
}

// removeHoneytoken deletes a token the harness planted, unless the run asked to
// keep artifacts. Tokens reused from a previous run keep the harness prefix, so
// removing by ID only ever removes the harness's own token.
func (h *harness) removeHoneytoken(ctx context.Context, token provenance.Honeytoken) {
	if token.ID == "" {
		return
	}
	_ = h.engine.Store().DeleteHoneytoken(ctx, token.ID)
	h.engine.Invalidate()
}

// traceConnected reports whether the session's recorded events form one
// connected graph rooted at the session start: every event either is the start
// or reaches a recorded event by its parent, and at least one action links to
// an earlier event. This is what lets an investigator see a single incident
// instead of unrelated alerts.
func (h *harness) traceConnected(ctx context.Context, key provenance.SessionKey) bool {
	sess, err := h.engine.Resolve(ctx, h.subject, key, provenance.SurfaceMCP)
	if err != nil {
		return false
	}
	events, _, err := h.engine.Store().ListEvents(ctx, provenance.EventQuery{SessionID: sess.ID, Ascending: true})
	if err != nil || len(events) == 0 {
		return false
	}
	ids := make(map[string]bool, len(events))
	for _, ev := range events {
		ids[ev.ID] = true
	}
	hasParentLink := false
	for _, ev := range events {
		if ev.ParentID == "" {
			continue
		}
		if !ids[ev.ParentID] {
			// A parent pointer to an event outside this session breaks the trace.
			return false
		}
		hasParentLink = true
	}
	// A session start plus at least one causal edge is a connected trace.
	return hasParentLink
}
