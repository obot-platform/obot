package executionbudget

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestConfigValidationRejectsUnboundedValues(t *testing.T) {
	config := DefaultConfig()
	config.PlatformMaxInFlight = 129
	if err := config.Validate(); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("expected invalid configuration, got %v", err)
	}
	config = DefaultConfig()
	config.DefaultPoolMaxInFlight = 0
	if err := config.Validate(); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("expected positive pool violation, got %v", err)
	}
	config = DefaultConfig()
	config.DedicatedPoolMaxInFlight = 33
	if err := config.Validate(); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("expected pool-to-platform violation, got %v", err)
	}
}

func TestResolveBudgetIdentityCanonicalizesEquivalentSources(t *testing.T) {
	primary := []CatalogReference{{SourceURL: "HTTPS://Example.COM:444/a/./b///%7e/%2f", EntryKey: " café\r"}}
	fallback := []CatalogReference{{SourceURL: "https://example.com:444/a/b/~/%2F", EntryKey: "café"}}
	first, err := ResolveBudgetIdentity(primary, nil, "GET /v1/{id}")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveBudgetIdentity(nil, fallback, "GET /v1/{id}")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent identities differ: %#v != %#v", first, second)
	}
}

func TestResolveBudgetIdentityCanonicalizesRootAndDefaultPort(t *testing.T) {
	primary := []CatalogReference{{SourceURL: "https://EXAMPLE.com:443", EntryKey: "entry"}}
	fallback := []CatalogReference{{SourceURL: "https://example.com/", EntryKey: "entry"}}
	first, err := ResolveBudgetIdentity(primary, nil, "GET /")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveBudgetIdentity(nil, fallback, "GET /")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent root sources produced different budget identities")
	}
}

func TestResolveBudgetIdentityRejectsAmbiguousOrUnsafeIdentity(t *testing.T) {
	references := []CatalogReference{
		{SourceURL: "https://one.example/a", EntryKey: "entry"},
		{SourceURL: "https://two.example/a", EntryKey: "entry"},
	}
	if _, err := ResolveBudgetIdentity(references, nil, "GET /a"); !errors.Is(err, ErrBudgetIdentityUnavailable) {
		t.Fatalf("expected identity rejection, got %v", err)
	}
	unsafe := []CatalogReference{{SourceURL: "https://user@example.com/a", EntryKey: "entry"}}
	if _, err := ResolveBudgetIdentity(unsafe, nil, "GET /a"); !errors.Is(err, ErrBudgetIdentityUnavailable) {
		t.Fatalf("expected userinfo rejection, got %v", err)
	}
}

func TestLimitReaderRejectsOneByteOverLimit(t *testing.T) {
	reader := LimitRequestBody(strings.NewReader("12345"), 4)
	output := make([]byte, 4)
	if _, err := reader.Read(output); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(make([]byte, 1)); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected limit error, got %v", err)
	}
}

func TestToBoundedMCPContentUsesDerivedEnvelope(t *testing.T) {
	raw := bytes.Repeat([]byte{7}, 1024)
	content, err := ToBoundedMCPContent(bytes.NewReader(raw), "application/octet-stream", 201, int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(content.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) || content.Truncated {
		t.Fatal("bounded MCP content did not preserve the complete body")
	}
	if _, err = ToBoundedMCPContent(bytes.NewReader(append(raw, 1)), "application/octet-stream", 200, int64(len(raw))); !errors.Is(err, ErrMCPContentTooLarge) {
		t.Fatalf("expected response limit, got %v", err)
	}
}

func TestLimitUpstreamBodyWithEncodingBoundsDecompression(t *testing.T) {
	raw := bytes.Repeat([]byte{9}, 2048)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := LimitUpstreamBodyWithEncoding(bytes.NewReader(compressed.Bytes()), "gzip", int64(compressed.Len()), 2048)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatal("bounded gzip reader did not preserve the complete response")
	}

	reader, err = LimitUpstreamBodyWithEncoding(bytes.NewReader(compressed.Bytes()), "gzip", int64(compressed.Len()), 2047)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(reader)
	_ = reader.Close()
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected decompressed response limit, got %v", err)
	}
	if _, err = LimitUpstreamBodyWithEncoding(bytes.NewReader(nil), "br", 1024, 1024); !errors.Is(err, ErrUnsupportedContentEncoding) {
		t.Fatalf("expected unsupported encoding, got %v", err)
	}
}

func TestCaptureBoundedAuditRedactsAndTruncates(t *testing.T) {
	secret := "synthetic-secret-value"
	header := http.Header{
		"Authorization": []string{secret}, "authtoken": []string{secret},
		"Content-Type": []string{"application/json"}, "X-Unknown": []string{secret},
	}
	capture, err := CaptureBoundedAudit(header, strings.NewReader(secret+"tail"), []string{"Authorization", "authtoken"}, 16)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Headers["Authorization"] != "[REDACTED]" || capture.Headers["authtoken"] != "[REDACTED]" {
		t.Fatalf("authentication headers were not redacted: %#v", capture.Headers)
	}
	if !capture.Truncated || len(capture.Body) != 16 || capture.OriginalBodyBytes < 17 {
		t.Fatalf("audit body was not bounded: %#v", capture)
	}
	if capture.ContainsSecret(secret) {
		t.Fatal("synthetic secret leaked through audit capture")
	}
	if Base64EnvelopeLimit(MiB) != 1463640 {
		t.Fatalf("unexpected base64 envelope limit: %d", Base64EnvelopeLimit(MiB))
	}
}

func TestCaptureBoundedAuditRejectsHeaderLimit(t *testing.T) {
	header := http.Header{"Content-Type": []string{strings.Repeat("x", 128)}}
	_, err := CaptureBoundedAuditWithLimits(
		header, strings.NewReader("body"), nil,
		AuditLimits{HeaderLimit: 16, BodyLimit: 16},
	)
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected audit header limit, got %v", err)
	}
}

func TestOutcomeImmediateReleaseMatrix(t *testing.T) {
	if !outcomeAllowsImmediateRelease(OutcomePreUpstreamCancelled) {
		t.Fatal("pre-upstream cancel must release immediately")
	}
	if !outcomeAllowsImmediateRelease(OutcomeBusinessResult) {
		t.Fatal("complete business result must release immediately")
	}
	if outcomeAllowsImmediateRelease(OutcomeTransportUnknown) {
		t.Fatal("uncertain transport outcome must remain reserved")
	}
	if DefaultConfig().MaxWait >= DefaultConfig().CallDeadline {
		t.Fatal("default wait budget must fit inside call deadline")
	}
	_ = time.Second
}

func TestBudgetContextUsesUnixMillisecondProtocolFields(t *testing.T) {
	context := BudgetContext{
		LeaseID: "request", LeaseIssuedAtUnixMs: 1, LeaseDeadlineUnixMs: 30001,
		FenceDeadlineUnixMs: 29001, DispatchedAtUnixMs: 1001, BudgetKeyHash: "hash",
		ClockSkewLimitMs: 500, FencingMarginMs: 1000,
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"leaseIssuedAtUnixMs", "leaseDeadlineUnixMs", "fenceDeadlineUnixMs",
		"dispatchedAtUnixMs", "clockSkewLimitMs", "fencingMarginMs",
	} {
		if _, ok := fields[name].(float64); !ok {
			t.Fatalf("%s must encode as a JSON number, got %#v", name, fields[name])
		}
	}
}
