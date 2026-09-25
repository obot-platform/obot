package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/sebastienrousseau/scout-reporting/attestation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AttestationMaxBytes bounds a fetched statement. A statement carries one verdict per
	// check and no request bodies, so real ones are tens of kilobytes.
	AttestationMaxBytes = 1 << 20
	// AttestationFetchTimeout bounds one fetch, independent of the client's own timeout.
	AttestationFetchTimeout = 15 * time.Second
	// attestationMaxFailedChecks bounds the ids copied into the status for display. The
	// server that published the statement chose them, so they are untrusted input. The
	// policy never reads this list: it reads FailedCategories, which is complete.
	attestationMaxFailedChecks = 32
	// attestationMaxFailedCategories bounds the distinct categories a statement may fail
	// in. scout has about a dozen phases and id prefixes, so a statement listing more is
	// not one scout wrote; it is refused as unverified rather than truncated, because a
	// truncated set is exactly what would let a denied category slip past the policy.
	attestationMaxFailedCategories = 64
	// attestationTransport is the only transport a catalog entry can attest: a remote
	// entry is reached over HTTP, and scout records it as such.
	attestationTransport = "http"
)

// AttestationPolicy is what a catalog entry's attestation must satisfy before a server is
// created from it. The zero value admits any verified statement whose subject matches.
type AttestationPolicy struct {
	// MinScore is the lowest statement score admitted; 0 disables the gate.
	MinScore float64
	// DenyFailIn lists check categories in which a failed check refuses admission. A
	// failed check is in a category when its id prefix before the first dot, or its
	// phase, equals it.
	DenyFailIn []string
}

// NewAttestationPolicy normalises operator input: categories are trimmed, lower-cased and
// de-duplicated, and MinScore must lie in [0, 100].
func NewAttestationPolicy(minScore float64, denyFailIn []string) (AttestationPolicy, error) {
	if minScore < 0 || minScore > 100 {
		return AttestationPolicy{}, fmt.Errorf("attestation minimum score must be between 0 and 100, got %v", minScore)
	}
	policy := AttestationPolicy{MinScore: minScore}
	for _, category := range denyFailIn {
		category = strings.ToLower(strings.TrimSpace(category))
		if category == "" || slices.Contains(policy.DenyFailIn, category) {
			continue
		}
		policy.DenyFailIn = append(policy.DenyFailIn, category)
	}
	slices.Sort(policy.DenyFailIn)
	return policy, nil
}

// ValidateAttestationRef rejects manifests whose attestation reference cannot be verified:
// it must be an https URL without credentials, and the entry must be a remote entry with a
// fixed, non-templated URL, since that URL is what the statement's subject is checked against.
func ValidateAttestationRef(manifest types.MCPServerCatalogEntryManifest) error {
	ref := manifest.Attestation
	if ref == nil {
		return nil
	}
	if manifest.Runtime != types.RuntimeRemote {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "attestation",
			Message: "attestations are only supported for remote entries",
		}
	}
	if manifest.RemoteConfig == nil || strings.TrimSpace(manifest.RemoteConfig.FixedURL) == "" || len(extractEnvRefs(manifest.RemoteConfig.FixedURL)) > 0 {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "attestation",
			Message: "attestations require remoteConfig.fixedURL without template references",
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(ref.URL))
	if err != nil {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "attestation.url",
			Message: fmt.Sprintf("invalid URL format: %v", err),
		}
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "attestation.url",
			Message: "must be an https URL with a hostname",
		}
	}
	if parsed.User != nil {
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "attestation.url",
			Message: "must not include user information",
		}
	}
	return nil
}

// VerifyAttestation fetches the statement at statementURL and reports whether it is a valid
// scout statement about endpoint. Every failure is recorded in the returned status rather
// than returned, because "could not verify" is a status the entry carries, not a
// reconcile error to retry blindly.
//
// Only the statement's structure and its subject binding are checked. Signature
// verification (DSSE, cosign) is out of scope; see adr/2026-09-23-scout-attestation-admission.md.
func VerifyAttestation(ctx context.Context, client *http.Client, statementURL, endpoint string) v1.MCPAttestationStatus {
	now := metav1.Now()
	status := v1.MCPAttestationStatus{CheckedAt: &now}

	body, err := fetchAttestation(ctx, client, statementURL)
	if err != nil {
		status.Error = fmt.Sprintf("fetch attestation: %v", err)
		return status
	}

	statement, err := attestation.Parse(body)
	if err != nil {
		status.Error = fmt.Sprintf("parse attestation: %v", err)
		return status
	}
	if err := statement.Validate(); err != nil {
		status.Error = fmt.Sprintf("invalid attestation: %v", err)
		return status
	}
	status.Verified = true

	predicate := statement.Predicate
	status.Instrument = strings.TrimSpace(predicate.Instrument.Name + " " + predicate.Instrument.Version)
	if !predicate.RanAt.IsZero() {
		status.RanAt = &metav1.Time{Time: predicate.RanAt}
	}
	if predicate.Score != nil {
		status.Score = predicate.Score.Total
		status.Grade = predicate.Score.Grade
	}
	status.FailCount = predicate.Counts.Fail
	// Every failed verdict is read, in whatever order the publisher chose: the category
	// set is what the policy judges, so it must be complete. Only the display list of ids
	// is capped.
	categories := map[string]bool{}
	for _, verdict := range predicate.Verdicts {
		if verdict.Status != "fail" {
			continue
		}
		if len(status.FailedChecks) < attestationMaxFailedChecks {
			status.FailedChecks = append(status.FailedChecks, verdict.ID)
		}
		for _, category := range failedCategories(verdict) {
			categories[category] = true
		}
	}
	slices.Sort(status.FailedChecks)
	if len(categories) > attestationMaxFailedCategories {
		status.Verified = false
		status.FailedChecks = nil
		status.Error = fmt.Sprintf("attestation records failures in %d categories, more than the %d a scout statement can have", len(categories), attestationMaxFailedCategories)
		return status
	}
	for category := range categories {
		status.FailedCategories = append(status.FailedCategories, category)
	}
	slices.Sort(status.FailedCategories)

	if !statement.Covers(attestationTransport, endpoint) {
		status.Error = fmt.Sprintf("attestation is about %s %s, not %s", predicate.Target.Transport, predicate.Target.Endpoint, endpoint)
		return status
	}
	status.SubjectMatch = true
	return status
}

// Admit reports why a server must not be created from entry, or nil. An entry without an
// attestation reference is admitted; an entry with one must carry a current, verified
// status that satisfies the policy.
func (p AttestationPolicy) Admit(entry v1.MCPServerCatalogEntry, manifestHash string) error {
	if entry.Spec.Manifest.Attestation == nil {
		return nil
	}
	status := entry.Status.Attestation
	if status == nil {
		return errors.New("catalog entry attestation has not been verified yet")
	}
	if status.ManifestHash != manifestHash {
		return errors.New("catalog entry attestation was verified for an earlier version of the entry")
	}
	return p.Evaluate(*status)
}

// Evaluate applies the policy to a verification result.
func (p AttestationPolicy) Evaluate(status v1.MCPAttestationStatus) error {
	if !status.Verified {
		return fmt.Errorf("catalog entry attestation could not be verified: %s", status.Error)
	}
	if !status.SubjectMatch {
		return fmt.Errorf("catalog entry attestation does not cover the entry's URL: %s", status.Error)
	}
	if p.MinScore > 0 && status.Score < p.MinScore {
		return fmt.Errorf("catalog entry attestation score %v is below the required minimum %v", status.Score, p.MinScore)
	}
	var denied []string
	for _, category := range status.FailedCategories {
		if slices.Contains(p.DenyFailIn, category) {
			denied = append(denied, category)
		}
	}
	if len(denied) > 0 {
		return fmt.Errorf("catalog entry attestation records failed checks in a denied category: %s", strings.Join(denied, ", "))
	}
	return nil
}

// failedCategories are the categories a failed verdict belongs to: its id prefix before
// the first dot and its phase, lower-cased, without repeats.
func failedCategories(verdict attestation.Verdict) []string {
	prefix, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(verdict.ID)), ".")
	phase := strings.ToLower(strings.TrimSpace(verdict.Phase))
	var out []string
	for _, c := range []string{prefix, phase} {
		if c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func fetchAttestation(ctx context.Context, client *http.Client, statementURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, AttestationFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statementURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, AttestationMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > AttestationMaxBytes {
		return nil, fmt.Errorf("statement exceeds %d bytes", AttestationMaxBytes)
	}
	return body, nil
}
