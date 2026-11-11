package mcpservercatalogentry

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// openAPIDriftProbeInterval is the default period between two probes of
	// the same OpenAPI source. Probes are read-only fetches through the
	// SSRF-protected importer; this constant trades upstream load against
	// detection latency and can be overridden per deployment through the
	// MCPOpenAPIDriftProbeInterval environment variable (Go duration).
	openAPIDriftProbeInterval = 6 * time.Hour
	// openAPIDriftProbeTimeout bounds one upstream schema fetch.
	openAPIDriftProbeTimeout = 30 * time.Second
)

// driftProbeInterval resolves the effective probe interval once per process.
func driftProbeInterval() time.Duration {
	if value := strings.TrimSpace(os.Getenv("MCPOpenAPIDriftProbeInterval")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed >= time.Second {
			return parsed
		}
	}
	return openAPIDriftProbeInterval
}

// ProbeOpenAPISourceDrift re-imports the Source of OpenAPI catalog entries and
// reports schema drift on the entry status. It runs on the leader-elected
// router only, never mutates stored snapshots, and never receives business
// credentials: the importer's client strips headers and token sources by
// construction. Detected drift is surfaced for administrators; adoption
// remains the explicit save-and-republish flow.
func (h *Handler) ProbeOpenAPISourceDrift(req router.Request, resp router.Response) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)

	// Only OpenAPI entries with a remote source are probeable. Inline
	// content sources cannot drift: the content is the configuration.
	config := entry.Spec.Manifest.OpenAPIConfig
	if entry.Spec.Manifest.Runtime != types.RuntimeOpenAPI || config == nil || config.Source.URL == "" || config.Source.Content != "" {
		return nil
	}
	// Entries without a stored snapshot have nothing to compare; the save
	// flow owns the initial import.
	if config.Schema == nil {
		return nil
	}

	now := time.Now()
	if drift := entry.Status.OpenAPISourceDrift; drift != nil && now.Before(drift.NextProbeAt.Time) {
		resp.RetryAfter(time.Until(drift.NextProbeAt.Time))
		return nil
	}

	probeConfig := *config
	probeConfig.Schema = nil
	stored := []byte(config.Schema.Raw)

	drift := &v1.OpenAPISourceDriftStatus{LastProbedAt: metav1.NewTime(now)}
	if h.OpenAPIImporter != nil {
		ctx, cancel := context.WithTimeout(req.Ctx, openAPIDriftProbeTimeout)
		defer cancel()
		fetched, err := h.OpenAPIImporter.Import(ctx, probeConfig)
		if err != nil {
			drift.ProbeError = boundedDriftError(err)
		} else {
			// Both sides compile under the same (empty) header config: header
			// configuration only shapes the policy document, never the
			// operation set or signatures being diffed here.
			report, err := openapi.ProbeSourceDrift(fetched, stored, probeConfig, nil)
			if err != nil {
				drift.ProbeError = boundedDriftError(err)
			} else {
				drift.Detected = report.HasDrift()
				drift.SnapshotDigest = report.SnapshotDigest
				drift.SourceDigest = report.SourceDigest
				drift.AddedOperations = report.Added
				drift.RemovedOperations = report.Removed
				drift.ChangedOperations = report.Changed
			}
		}
	} else {
		drift.ProbeError = "drift probe is not configured"
	}

	interval := driftProbeInterval()
	drift.NextProbeAt = metav1.NewTime(now.Add(interval))
	if !equality.Semantic.DeepEqual(entry.Status.OpenAPISourceDrift, drift) {
		entry.Status.OpenAPISourceDrift = drift
		if err := req.Client.Status().Update(req.Ctx, entry); err != nil {
			return fmt.Errorf("update openapi drift status: %w", err)
		}
	}
	resp.RetryAfter(interval)
	return nil
}

func boundedDriftError(err error) string {
	message := err.Error()
	// Importer errors already redact URLs and transport detail; keep a hard
	// bound regardless so a future verbose error cannot bloat the status.
	if len(message) > 256 {
		message = message[:256]
	}
	return message
}
