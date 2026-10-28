package guardrails

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestModelArmorChecker(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	// "Café: é@x.io" — the email starts at code point 6 but byte 7.
	const text = "Café: é@x.io"
	emailStart := strings.Index(text, "é@")

	tests := []struct {
		name             string
		stage            string
		wantPath         string
		wantField        string
		response         string
		status           int
		wantErr          string
		wantFindings     []string
		wantSpans        [][2]int
		wantReplacements map[int]string
	}{
		{
			name:         "no match",
			stage:        StageInput,
			wantPath:     "/v1/projects/proj/locations/us-central1/templates/tmpl:sanitizeUserPrompt",
			wantField:    "userPromptData",
			response:     `{"sanitizationResult":{"filterMatchState":"NO_MATCH_FOUND","invocationResult":"SUCCESS","filterResults":{}}}`,
			wantFindings: nil,
		},
		{
			name:      "inspection findings locate sensitive data by code point",
			stage:     StageOutput,
			wantPath:  "/v1/projects/proj/locations/us-central1/templates/tmpl:sanitizeModelResponse",
			wantField: "modelResponseData",
			response: `{"sanitizationResult":{"filterMatchState":"MATCH_FOUND","invocationResult":"SUCCESS","filterResults":{
				"sdp":{"sdpFilterResult":{"inspectResult":{"executionState":"EXECUTION_SUCCESS","matchState":"MATCH_FOUND","findings":[
					{"infoType":"EMAIL_ADDRESS","likelihood":"LIKELY","location":{"byteRange":{"start":"7","end":"14"},"codepointRange":{"start":"6","end":"12"}}}]}}},
				"csam":{"csamFilterFilterResult":{"executionState":"EXECUTION_SUCCESS","matchState":"NO_MATCH_FOUND"}}}}}`,
			wantFindings: []string{"sensitive_data: EMAIL_ADDRESS detected"},
			wantSpans:    [][2]int{{emailStart, len(text)}},
		},
		{
			name:      "de-identified text replaces the segment",
			stage:     StageInput,
			wantPath:  "/v1/projects/proj/locations/us-central1/templates/tmpl:sanitizeUserPrompt",
			wantField: "userPromptData",
			response: `{"sanitizationResult":{"filterMatchState":"MATCH_FOUND","invocationResult":"SUCCESS","filterResults":{
				"sdp":{"sdpFilterResult":{"deidentifyResult":{"executionState":"EXECUTION_SUCCESS","matchState":"MATCH_FOUND","data":{"text":"Café: [EMAIL_ADDRESS]"},"infoTypes":["EMAIL_ADDRESS"]}}}}}}`,
			wantFindings:     []string{"sensitive_data: EMAIL_ADDRESS detected"},
			wantReplacements: map[int]string{0: "Café: [EMAIL_ADDRESS]"},
		},
		{
			name:      "unlocatable findings drop replacements",
			stage:     StageInput,
			wantPath:  "/v1/projects/proj/locations/us-central1/templates/tmpl:sanitizeUserPrompt",
			wantField: "userPromptData",
			response: `{"sanitizationResult":{"filterMatchState":"MATCH_FOUND","invocationResult":"SUCCESS","filterResults":{
				"pi_and_jailbreak":{"piAndJailbreakFilterResult":{"executionState":"EXECUTION_SUCCESS","matchState":"MATCH_FOUND","confidenceLevel":"MEDIUM_AND_ABOVE"}},
				"rai":{"raiFilterResult":{"executionState":"EXECUTION_SUCCESS","matchState":"MATCH_FOUND","raiFilterTypeResults":{
					"dangerous":{"matchState":"MATCH_FOUND","confidenceLevel":"HIGH"},
					"harassment":{"matchState":"NO_MATCH_FOUND"}}}},
				"sdp":{"sdpFilterResult":{"deidentifyResult":{"matchState":"MATCH_FOUND","data":{"text":"Café: [EMAIL_ADDRESS]"},"infoTypes":["EMAIL_ADDRESS"]}}}}}}`,
			wantFindings: []string{"prompt_injection: Prompt injection or jailbreak attempt (medium and above confidence)", "responsible_ai: dangerous content (high confidence)", "sensitive_data: EMAIL_ADDRESS detected"},
		},
		{
			name:      "error status",
			stage:     StageInput,
			wantPath:  "/v1/projects/proj/locations/us-central1/templates/tmpl:sanitizeUserPrompt",
			wantField: "userPromptData",
			status:    http.StatusForbidden,
			response:  `{"error":{"code":403,"message":"permission denied"}}`,
			wantErr:   "403",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokenRequests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					tokenRequests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != tt.wantPath {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Errorf("Authorization = %q", got)
				}
				var body map[string]map[string]string
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &body); err != nil {
					t.Errorf("invalid body: %v", err)
				}
				if len(body) != 1 || body[tt.wantField]["text"] != text {
					t.Errorf("unexpected body %s", raw)
				}
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				_, _ = w.Write([]byte(tt.response))
			}))
			defer srv.Close()

			sa, _ := json.Marshal(map[string]string{
				"type":           "service_account",
				"project_id":     "proj",
				"private_key_id": "kid",
				"private_key":    string(keyPEM),
				"client_email":   "guard@proj.iam.gserviceaccount.com",
				"token_uri":      srv.URL + "/token",
			})
			c, err := newChecker("google-model-armor", map[string]any{
				"project":  "proj",
				"location": "us-central1",
				"template": "tmpl",
				"endpoint": srv.URL,
			}, map[string]string{"serviceAccountJSON": string(sa)}, Deps{HTTPClient: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			segments := []Segment{{Role: RoleUser, Text: text}, {Role: RoleAssistant, Text: ""}}
			result, err := c.Check(context.Background(), CheckInput{Stage: tt.stage, Segments: segments})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "permission denied") {
					t.Fatalf("error = %v, want it to mention %q and the body", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tokenRequests.Load() != 1 {
				t.Errorf("token requests = %d, want 1", tokenRequests.Load())
			}
			assertProviderFindings(t, result.Findings, tt.wantFindings)
			assertProviderReplacements(t, result.Replacements, tt.wantReplacements)
			var spans [][2]int
			for _, f := range result.Findings {
				for _, s := range f.Spans {
					if s.Segment != 0 {
						t.Errorf("span on segment %d", s.Segment)
					}
					spans = append(spans, [2]int{s.Start, s.End})
				}
			}
			if len(spans) != len(tt.wantSpans) || (len(spans) > 0 && spans[0] != tt.wantSpans[0]) {
				t.Fatalf("spans = %v, want %v", spans, tt.wantSpans)
			}
			if len(spans) > 0 {
				if got := ApplySpans(segments, result.Findings[0].Spans)[0].Text; got != "Café: [REDACTED:EMAIL_ADDRESS]" {
					t.Errorf("redacted text = %q", got)
				}
			}
		})
	}
}

// Application default credentials are the Obot server's own identity, so they are only
// sent to Google: a guardrail without a service account key can't name another host, through
// the endpoint or through the location the default endpoint is built from.
func TestModelArmorKeepsDefaultCredentialsOnGoogle(t *testing.T) {
	tests := []struct {
		name     string
		location string
		endpoint string
		key      string
		wantErr  bool
	}{
		{
			name:     "default endpoint",
			location: "us-central1",
		},
		{
			name:     "regional googleapis.com endpoint",
			location: "us-central1",
			endpoint: "https://modelarmor.us-central1.rep.googleapis.com",
		},
		{
			name:     "Private Service Connect endpoint",
			location: "us-central1",
			endpoint: "https://modelarmor-psc.p.googleapis.com",
		},
		{
			name:     "another host",
			location: "us-central1",
			endpoint: "https://attacker.example",
			wantErr:  true,
		},
		{
			name:     "a host that only ends like Google's",
			location: "us-central1",
			endpoint: "https://evilgoogleapis.com",
			wantErr:  true,
		},
		{
			name:     "another googleapis.com host",
			location: "us-central1",
			endpoint: "https://my-bucket.storage.googleapis.com",
			wantErr:  true,
		},
		{
			name:     "plain http",
			location: "us-central1",
			endpoint: "http://modelarmor.us-central1.rep.googleapis.com",
			wantErr:  true,
		},
		{
			name:     "user info hiding the host",
			location: "us-central1",
			endpoint: "https://googleapis.com@attacker.example",
			wantErr:  true,
		},
		{
			name:     "location that changes the default host",
			location: "x@attacker.example/",
			wantErr:  true,
		},
		{
			name:     "another host with a service account key",
			location: "us-central1",
			endpoint: "https://modelarmor.internal.example",
			key:      `{"type":"service_account"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{"project": "proj", "location": tt.location, "template": "tmpl"}
			if tt.endpoint != "" {
				config["endpoint"] = tt.endpoint
			}
			secrets := map[string]string{}
			if tt.key != "" {
				secrets["serviceAccountJSON"] = tt.key
			}
			_, err := newChecker("google-model-armor", config, secrets, Deps{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
