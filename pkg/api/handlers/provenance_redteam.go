package handlers

import (
	"context"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/provenance/redteam"
)

const (
	// redTeamTimeout bounds one run of the assurance harness.
	redTeamTimeout = 2 * time.Minute
)

// RunRedTeam drives the policy engine through the built-in assurance scenarios and
// returns the scorecard.
func (h *ProvenanceHandler) RunRedTeam(req api.Context) error {
	ctx, cancel := context.WithTimeout(req.Context(), redTeamTimeout)
	defer cancel()
	return req.Write(redteam.Run(ctx, h.engine, redteam.Options{}))
}
