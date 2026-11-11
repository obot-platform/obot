// Package openapi source drift probing.
//
// Probe re-fetches the Source of an OpenAPI catalog entry through the same
// SSRF-protected Importer used on save, compiles both the stored snapshot and
// the fresh schema into canonical operation manifests, and reports the
// differences. It never mutates the stored snapshot, never publishes anything,
// and never receives business credentials. Adoption of detected changes stays
// the existing explicit administrator flow: re-save the entry, then update the
// vMCP to publish a new version.
package openapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapiworker"
)

// DriftReport is the bounded, non-sensitive summary of one probe. Operation
// lists are keys ("METHOD /path") only; schema bodies are reduced to digests.
type DriftReport struct {
	SourceDigest   string   `json:"sourceDigest,omitempty"`
	SnapshotDigest string   `json:"snapshotDigest,omitempty"`
	Added          []string `json:"addedOperations,omitempty"`
	Removed        []string `json:"removedOperations,omitempty"`
	Changed        []string `json:"changedOperations,omitempty"`
}

// HasDrift is true when the source's canonical operation set differs from the
// stored snapshot's, in any direction.
func (r DriftReport) HasDrift() bool {
	return len(r.Added) > 0 || len(r.Removed) > 0 || len(r.Changed) > 0
}

// ProbeSourceDrift re-imports config.Source and diffs the compiled operation
// manifests. storedSchema is the entry's persisted snapshot (Schema.Raw).
// probeConfig must carry the same BaseURL and header configuration semantics
// used at save time so both sides compile under identical rules.
func ProbeSourceDrift(fetched *Result, storedSchema []byte, config types.OpenAPIRuntimeConfig, headers []types.MCPConfig) (DriftReport, error) {
	if fetched == nil || len(fetched.Schema) == 0 {
		return DriftReport{}, fmt.Errorf("probe produced no schema")
	}
	if len(storedSchema) == 0 {
		return DriftReport{}, fmt.Errorf("entry has no stored snapshot to compare")
	}

	snapshotManifest, _, _, _, err := openapiworker.BuildManifest(storedSchema, config, headers)
	if err != nil {
		return DriftReport{}, fmt.Errorf("stored snapshot no longer compiles: %w", err)
	}
	sourceManifest, _, _, _, err := openapiworker.BuildManifest(fetched.Schema, config, headers)
	if err != nil {
		return DriftReport{}, fmt.Errorf("fresh source schema does not compile: %w", err)
	}

	report := DriftReport{
		SourceDigest:   digestOf(fetched.Schema),
		SnapshotDigest: digestOf(storedSchema),
	}
	snapshotOps := operationMap(snapshotManifest)
	sourceOps := operationMap(sourceManifest)

	for key, op := range snapshotOps {
		other, ok := sourceOps[key]
		if !ok {
			report.Removed = append(report.Removed, key)
			continue
		}
		if operationSignature(op) != operationSignature(other) {
			report.Changed = append(report.Changed, key)
		}
	}
	for key := range sourceOps {
		if _, ok := snapshotOps[key]; !ok {
			report.Added = append(report.Added, key)
		}
	}
	sortStrings(report.Added)
	sortStrings(report.Removed)
	sortStrings(report.Changed)
	return report, nil
}

func operationMap(manifest openapiworker.CanonicalOperationManifest) map[string]openapiworker.Operation {
	ops := make(map[string]openapiworker.Operation, len(manifest.Operations))
	for _, op := range manifest.Operations {
		ops[op.OperationKey] = op
	}
	return ops
}

// operationSignature reduces an operation to the fields whose change can break
// published callers: parameters, request body, input schema, and media types.
// Tool naming/display changes are reported as Changed too, because they alter
// the published manifest and would shift the execution digest on adoption.
func operationSignature(op openapiworker.Operation) string {
	h := sha256.New()
	writeJSON := func(v any) {
		bytes, err := json.Marshal(v)
		if err == nil {
			_, _ = h.Write(bytes)
		}
	}
	writeJSON(op.Parameters)
	writeJSON(op.RequestBody)
	writeJSON(op.InputSchema)
	writeJSON(op.RequestMediaTypes)
	writeJSON(op.OperationID)
	writeJSON(op.DisplayName)
	writeJSON(op.Description)
	return hex.EncodeToString(h.Sum(nil))
}

func digestOf(schema []byte) string {
	sum := sha256.Sum256(schema)
	return hex.EncodeToString(sum[:])
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
