package guardrails

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAzureLanguagePIIFindsAndLocatesEntities(t *testing.T) {
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/language/:analyze-text" || r.URL.Query().Get("api-version") != "2023-04-01" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != "secret" {
			t.Errorf("unexpected key %q", got)
		}
		var req azureLanguageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		p := req.Parameters
		if req.Kind != "PiiEntityRecognition" || p.StringIndexType != "UnicodeCodePoint" || p.ModelVersion != "latest" || p.Domain != "phi" || len(p.PIICategories) != 1 || p.PIICategories[0] != "Email" {
			t.Errorf("unexpected request %+v", req)
		}
		batchSizes = append(batchSizes, len(req.AnalysisInput.Documents))

		var docs []string
		for _, d := range req.AnalysisInput.Documents {
			if d.Language != "en" {
				t.Errorf("unexpected language %q", d.Language)
			}
			entities := "[]"
			if strings.HasSuffix(d.Text, "bob@example.com") {
				// Offsets are code points: "é" is one character but two bytes.
				entities = `[{"text":"bob@example.com","category":"Email","offset":6,"length":15,"confidenceScore":0.9},{"text":"héllo","category":"Person","offset":0,"length":5,"confidenceScore":0.5}]`
			}
			docs = append(docs, fmt.Sprintf(`{"id":%q,"redactedText":"***","entities":%s,"warnings":[]}`, d.ID, entities))
		}
		_, _ = fmt.Fprintf(w, `{"kind":"PiiEntityRecognitionResults","results":{"documents":[%s],"errors":[],"modelVersion":"2021-01-15"}}`, strings.Join(docs, ","))
	}))
	defer srv.Close()

	c, err := newChecker("azure-language-pii", map[string]any{
		"endpoint":   srv.URL,
		"domain":     "phi",
		"categories": []any{"Email"},
	}, map[string]string{"apiKey": "secret"}, Deps{HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	// Seven non-empty segments need two requests of at most five documents.
	segments := []Segment{{Role: RoleSystem}}
	for range 6 {
		segments = append(segments, Segment{Role: RoleUser, Text: "nothing here"})
	}
	segments = append(segments, Segment{Role: RoleToolResult, Name: "lookup", Text: "héllo bob@example.com"})

	result, err := c.Check(t.Context(), CheckInput{Stage: StageInput, Segments: segments})
	if err != nil {
		t.Fatal(err)
	}
	if len(batchSizes) != 2 || batchSizes[0] != 5 || batchSizes[1] != 2 {
		t.Fatalf("unexpected batches %v", batchSizes)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected one finding, got %+v", result.Findings)
	}
	f := result.Findings[0]
	if f.Category != "pii" || f.Message != "Email detected" || f.Score != 0.9 {
		t.Fatalf("unexpected finding %+v", f)
	}
	if s := f.Spans[0]; s.Segment != 7 || s.Start != 7 || s.End != 22 {
		t.Fatalf("unexpected span %+v", s)
	}
	if got := ApplySpans(segments, f.Spans)[7].Text; got != "héllo [REDACTED:EMAIL]" {
		t.Fatalf("unexpected redaction %q", got)
	}
}

func TestAzureLanguagePIINoFindingsAndErrors(t *testing.T) {
	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch status {
		case http.StatusOK:
			_, _ = w.Write([]byte(`{"results":{"documents":[{"id":"0","entities":[]}],"errors":[]}}`))
		case http.StatusMultiStatus:
			// A document-level error means the content went unchecked.
			_, _ = w.Write([]byte(`{"results":{"documents":[],"errors":[{"id":"0","error":{"code":"InvalidArgument","message":"Invalid document"}}]}}`))
		default:
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"401","message":"Access denied due to invalid subscription key."}}`))
		}
	}))
	defer srv.Close()

	c, err := newChecker("azure-language-pii", map[string]any{"endpoint": srv.URL}, map[string]string{"apiKey": "secret"}, Deps{HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	in := CheckInput{Stage: StageOutput, Segments: []Segment{{Role: RoleAssistant, Text: "nothing to see"}}}

	status = http.StatusOK
	if result, err := c.Check(t.Context(), in); err != nil || len(result.Findings) != 0 {
		t.Fatalf("expected no findings, got %+v, %v", result, err)
	}
	status = http.StatusMultiStatus
	if _, err := c.Check(t.Context(), in); err == nil || !strings.Contains(err.Error(), "InvalidArgument") {
		t.Fatalf("expected a document error, got %v", err)
	}
	status = http.StatusUnauthorized
	if _, err := c.Check(t.Context(), in); err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid subscription key") {
		t.Fatalf("expected an error with the status and body, got %v", err)
	}
	if _, err := newChecker("azure-language-pii", map[string]any{"endpoint": srv.URL}, nil, Deps{}); err == nil {
		t.Fatal("the API key should be required")
	}
}
