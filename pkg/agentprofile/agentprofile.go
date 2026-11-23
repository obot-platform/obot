// Package agentprofile holds the rules of an agent's profiles (types.AgentProfile): the IDs of its entries, and how
// they are validated. It is pure: its callers read storage, and hand it what they read.
package agentprofile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// Validate checks an agent's profiles before they are written: each on its own (types.AgentProfile.Validate), and
// that their names are unique.
func Validate(entries []types.AgentProfile) error {
	names := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("agent profile %q: %w", entry.Name, err)
		}
		name := strings.TrimSpace(entry.Name)
		if _, ok := names[name]; ok {
			return fmt.Errorf("duplicate agent profile name %q", name)
		}
		names[name] = struct{}{}
	}
	return nil
}

// newID returns a random entry ID.
func newID() string {
	id := make([]byte, 16)
	// crypto/rand.Read never fails; it crashes the program instead.
	_, _ = rand.Read(id)
	return types.AgentProfileIDPrefix + hex.EncodeToString(id)
}

// InitializeIDs assigns new IDs to the entries of an agent that is being created. Clients cannot choose IDs, because
// audit records name entries by them.
func InitializeIDs(entries []types.AgentProfile) error {
	for index := range entries {
		if entries[index].ID != "" {
			return fmt.Errorf("agent profile %q: id is server-assigned and must be omitted", entries[index].Name)
		}
		entries[index].ID = newID()
	}
	return nil
}

// ReconcileIDs keeps the IDs of entries during an update, as vMCP profile IDs are kept (vmcp.ReconcileProfileIDs). An
// entry sent with an ID keeps it, whatever its name. An entry sent without one is new and gets a new ID, even if it
// has the name of a stored entry. An ID that no stored entry has, or that two entries share, is refused.
func ReconcileIDs(existing, desired []types.AgentProfile) error {
	stored := make(map[string]struct{}, len(existing))
	for _, entry := range existing {
		stored[entry.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(desired))
	for index := range desired {
		entry := &desired[index]
		if entry.ID == "" {
			entry.ID = newID()
			continue
		}
		if _, ok := stored[entry.ID]; !ok {
			return fmt.Errorf("agent profile %q has unknown id %q", entry.Name, entry.ID)
		}
		if _, ok := seen[entry.ID]; ok {
			return fmt.Errorf("agent profile %q has the id %q of another entry", entry.Name, entry.ID)
		}
		seen[entry.ID] = struct{}{}
	}
	return nil
}
