package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

const (
	bedrockActionIntervened = "GUARDRAIL_INTERVENED"
	bedrockActionAnonymized = "ANONYMIZED"
	bedrockActionNone       = "NONE"

	// providerMaxResponseBytes bounds how much of an integration's response is read.
	providerMaxResponseBytes = 10 << 20
	// providerMaxErrorBody bounds how much of an error response is included in errors.
	providerMaxErrorBody = 512
)

// bedrockChecker calls the AWS Bedrock Guardrails ApplyGuardrail API.
// https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_ApplyGuardrail.html
type bedrockChecker struct {
	client      *http.Client
	endpoint    string
	region      string
	guardrailID string
	version     string
	creds       aws.CredentialsProvider
}

type bedrockApplyRequest struct {
	Source  string                `json:"source"`
	Content []bedrockContentBlock `json:"content"`
}

type bedrockContentBlock struct {
	Text bedrockText `json:"text"`
}

type bedrockText struct {
	Text string `json:"text"`
}

type bedrockApplyResponse struct {
	Action     string              `json:"action"`
	Outputs    []bedrockText       `json:"outputs"`
	Assessment []bedrockAssessment `json:"assessments"`
}

// bedrockEntry covers the fields shared by every assessment entry type.
type bedrockEntry struct {
	Action     string   `json:"action"`
	Detected   *bool    `json:"detected"`
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Confidence string   `json:"confidence"`
	Score      *float64 `json:"score"`
	Threshold  *float64 `json:"threshold"`
}

type bedrockAssessment struct {
	TopicPolicy *struct {
		Topics []bedrockEntry `json:"topics"`
	} `json:"topicPolicy"`
	ContentPolicy *struct {
		Filters []bedrockEntry `json:"filters"`
	} `json:"contentPolicy"`
	WordPolicy *struct {
		CustomWords      []bedrockEntry `json:"customWords"`
		ManagedWordLists []bedrockEntry `json:"managedWordLists"`
	} `json:"wordPolicy"`
	SensitiveInformationPolicy *struct {
		PIIEntities []bedrockEntry `json:"piiEntities"`
		Regexes     []bedrockEntry `json:"regexes"`
	} `json:"sensitiveInformationPolicy"`
	ContextualGroundingPolicy *struct {
		Filters []bedrockEntry `json:"filters"`
	} `json:"contextualGroundingPolicy"`
	AutomatedReasoningPolicy *struct {
		Findings []json.RawMessage `json:"findings"`
	} `json:"automatedReasoningPolicy"`
}

// mergedFindings merges findings with the same category and message, keeping first-seen order.
type mergedFindings struct {
	byKey map[string]*Finding
	order []string
}

func init() {
	Register(ProviderInfo{
		Type:        "aws-bedrock",
		Label:       "AWS Bedrock Guardrails",
		Category:    "Integrations",
		Description: "Checks content with an Amazon Bedrock guardrail's denied topics, content filters, word filters, sensitive information filters and contextual grounding checks. Content the guardrail anonymizes is redacted with its masked text.",
		Redacts:     true,
		Fields: []Field{
			{Name: "region", Label: "Region", Type: FieldText, Destination: true, Required: true, Description: "AWS region of the guardrail, such as us-east-1."},
			{Name: "guardrailId", Label: "Guardrail ID", Type: FieldText, Required: true, Description: "The guardrail's ID or ARN."},
			{Name: "guardrailVersion", Label: "Guardrail version", Type: FieldText, Default: "DRAFT", Description: "A version number, or DRAFT for the working draft."},
			{Name: "accessKeyId", Label: "Access key ID", Type: FieldPassword, Sensitive: true, Description: "Leave empty to use the default AWS credential chain (environment, shared config, or IAM role)."},
			{Name: "secretAccessKey", Label: "Secret access key", Type: FieldPassword, Sensitive: true},
			{Name: "sessionToken", Label: "Session token", Type: FieldPassword, Sensitive: true, Description: "Only for temporary credentials."},
			{Name: "endpoint", Label: "Endpoint", Type: FieldText, Destination: true, Description: "Overrides the Bedrock runtime endpoint, such as a VPC endpoint. Defaults to https://bedrock-runtime.<region>.amazonaws.com."},
		},
	}, newBedrockChecker)
}

func newBedrockChecker(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
	region := ConfigString(config, "region", "")
	c := &bedrockChecker{
		client:      providerHTTPClient(deps),
		region:      region,
		guardrailID: ConfigString(config, "guardrailId", ""),
		version:     ConfigString(config, "guardrailVersion", "DRAFT"),
		endpoint:    strings.TrimRight(ConfigString(config, "endpoint", "https://bedrock-runtime."+region+".amazonaws.com"), "/"),
	}
	if region == "" || c.guardrailID == "" {
		return nil, fmt.Errorf("region and guardrail ID are required")
	}

	accessKey, secretKey := secrets["accessKeyId"], secrets["secretAccessKey"]
	switch {
	case accessKey != "" && secretKey != "":
		c.creds = credentials.NewStaticCredentialsProvider(accessKey, secretKey, secrets["sessionToken"])
	case accessKey != "" || secretKey != "":
		return nil, fmt.Errorf("both an access key ID and a secret access key are required")
	default:
		// Credentials are resolved lazily, on first use, by the default chain.
		cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("failed to load default AWS credentials: %w", err)
		}
		c.creds = cfg.Credentials
	}
	return c, nil
}

func (c *bedrockChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	indexes := checkableSegments(in.Segments)
	if len(indexes) == 0 {
		return Result{}, nil
	}

	// One content block per segment, so masked outputs map back to their segments.
	body := bedrockApplyRequest{Source: "INPUT"}
	if in.Stage == StageOutput {
		body.Source = "OUTPUT"
	}
	for _, i := range indexes {
		body.Content = append(body.Content, bedrockContentBlock{Text: bedrockText{Text: in.Segments[i].Text}})
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}

	u := fmt.Sprintf("%s/guardrail/%s/version/%s/apply", c.endpoint, url.PathEscape(c.guardrailID), url.PathEscape(c.version))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	creds, err := c.creds.Retrieve(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get AWS credentials: %w", err)
	}
	if err := signAWSRequest(req, creds, "bedrock", c.region, time.Now()); err != nil {
		return Result{}, fmt.Errorf("failed to sign Bedrock request: %w", err)
	}

	var resp bedrockApplyResponse
	if err := providerDo(c.client, req, "AWS Bedrock Guardrails", &resp); err != nil {
		return Result{}, err
	}
	return bedrockResult(in.Segments, indexes, resp), nil
}

// bedrockResult converts an ApplyGuardrail response. Only an intervention counts: a guardrail
// in detect mode reports action NONE, and its assessments are informational.
func bedrockResult(segments []Segment, indexes []int, resp bedrockApplyResponse) Result {
	if resp.Action != bedrockActionIntervened {
		return Result{}
	}

	var (
		fs = newMergedFindings()
		// maskOnly reports whether every intervening entry anonymized rather than blocked,
		// in which case the outputs are the input with masking applied.
		maskOnly = true
	)
	add := func(e bedrockEntry, category, message string) {
		if e.Detected != nil && !*e.Detected {
			return
		}
		if e.Detected == nil && (e.Action == "" || e.Action == bedrockActionNone) {
			return
		}
		if e.Action != bedrockActionAnonymized && e.Action != bedrockActionNone {
			maskOnly = false
		}
		fs.add(Finding{Category: category, Message: message, Count: 1})
	}

	for _, a := range resp.Assessment {
		if a.TopicPolicy != nil {
			for _, e := range a.TopicPolicy.Topics {
				add(e, "topic", "Denied topic: "+e.Name)
			}
		}
		if a.ContentPolicy != nil {
			for _, e := range a.ContentPolicy.Filters {
				msg := strings.ToUpper(e.Type) + " content"
				if e.Confidence != "" {
					msg += " (" + strings.ToLower(e.Confidence) + " confidence)"
				}
				add(e, "content_filter", msg)
			}
		}
		if a.WordPolicy != nil {
			for _, e := range a.WordPolicy.CustomWords {
				add(e, "word_filter", "Blocked word")
			}
			for _, e := range a.WordPolicy.ManagedWordLists {
				add(e, "word_filter", strings.ToUpper(e.Type)+" word list match")
			}
		}
		if a.SensitiveInformationPolicy != nil {
			for _, e := range a.SensitiveInformationPolicy.PIIEntities {
				add(e, "pii", strings.ToUpper(e.Type)+" detected")
			}
			for _, e := range a.SensitiveInformationPolicy.Regexes {
				add(e, "sensitive_information", e.Name+" pattern matched")
			}
		}
		if a.ContextualGroundingPolicy != nil {
			for _, e := range a.ContextualGroundingPolicy.Filters {
				msg := strings.ToUpper(e.Type) + " check failed"
				if e.Score != nil && e.Threshold != nil {
					msg = fmt.Sprintf("%s score %.2f below threshold %.2f", strings.ToUpper(e.Type), *e.Score, *e.Threshold)
				}
				add(e, "contextual_grounding", msg)
			}
		}
		if a.AutomatedReasoningPolicy != nil && len(a.AutomatedReasoningPolicy.Findings) > 0 {
			maskOnly = false
			fs.add(Finding{Category: "automated_reasoning", Message: "Automated reasoning check failed", Count: len(a.AutomatedReasoningPolicy.Findings)})
		}
	}

	result := Result{Findings: fs.list()}
	if len(result.Findings) == 0 {
		// The guardrail intervened without saying why.
		maskOnly = false
		result.Findings = []Finding{{Category: "guardrail", Message: "AWS Bedrock guardrail intervened"}}
	}
	// When content is only masked, the outputs are the content blocks in request order with
	// masking applied. A blocked request returns a single canned message instead, which must
	// not replace content, so redacting policies then block.
	if maskOnly && len(resp.Outputs) == len(indexes) {
		for i, out := range resp.Outputs {
			seg := indexes[i]
			if out.Text != segments[seg].Text {
				if result.Replacements == nil {
					result.Replacements = map[int]string{}
				}
				result.Replacements[seg] = out.Text
			}
		}
	}
	return result
}

// providerHTTPClient returns the client integrations use for external calls.
func providerHTTPClient(deps Deps) *http.Client {
	if deps.HTTPClient != nil {
		return deps.HTTPClient
	}
	return http.DefaultClient
}

// providerTransport returns the transport of an integration's HTTP client.
func providerTransport(client *http.Client) http.RoundTripper {
	if client.Transport != nil {
		return client.Transport
	}
	return http.DefaultTransport
}

// providerDo sends a request to an integration and decodes its JSON response into out. A
// non-2xx status is an error that includes the status and the start of the response body.
func providerDo(client *http.Client, req *http.Request, service string, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", service, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, providerMaxResponseBytes))
	if err != nil {
		return fmt.Errorf("failed to read %s response: %w", service, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s returned %s: %s", service, resp.Status, providerErrorBody(body))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("invalid %s response: %w", service, err)
	}
	return nil
}

// providerErrorBody trims an error response for inclusion in an error message.
func providerErrorBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > providerMaxErrorBody {
		s = s[:providerMaxErrorBody]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
		s += "..."
	}
	if s == "" {
		return "(empty response)"
	}
	return s
}

// checkableSegments returns the indexes of segments with text to check.
func checkableSegments(segments []Segment) []int {
	var out []int
	for i, s := range segments {
		if strings.TrimSpace(s.Text) != "" {
			out = append(out, i)
		}
	}
	return out
}

// codepointToByteOffset converts an offset in Unicode code points into a byte offset in s.
// It returns -1 when the offset is outside s.
func codepointToByteOffset(s string, cp int) int {
	if cp < 0 {
		return -1
	}
	n := 0
	for i := range s {
		if n == cp {
			return i
		}
		n++
	}
	if n == cp {
		return len(s)
	}
	return -1
}

// codepointSpan converts a code point range in segment seg's text into a byte span. When
// expected is set and the range doesn't cover it, the first occurrence of expected is used
// instead. It reports false when the range can't be located.
func codepointSpan(segments []Segment, seg, start, end int, expected, label string) (Span, bool) {
	if seg < 0 || seg >= len(segments) {
		return Span{}, false
	}
	text := segments[seg].Text
	bs, be := codepointToByteOffset(text, start), codepointToByteOffset(text, end)
	if bs >= 0 && be > bs && (expected == "" || text[bs:be] == expected) {
		return Span{Segment: seg, Start: bs, End: be, Label: label}, true
	}
	if expected != "" {
		if i := strings.Index(text, expected); i >= 0 {
			return Span{Segment: seg, Start: i, End: i + len(expected), Label: label}, true
		}
	}
	return Span{}, false
}

func newMergedFindings() *mergedFindings {
	return &mergedFindings{byKey: map[string]*Finding{}}
}

// add merges f into the set: counts add up, spans accumulate and the highest score wins.
func (s *mergedFindings) add(f Finding) {
	key := f.Category + "\x00" + f.Message
	existing, ok := s.byKey[key]
	if !ok {
		c := f
		s.byKey[key] = &c
		s.order = append(s.order, key)
		return
	}
	existing.Count += f.Count
	existing.Spans = append(existing.Spans, f.Spans...)
	if f.Score > existing.Score {
		existing.Score = f.Score
	}
}

func (s *mergedFindings) list() []Finding {
	out := make([]Finding, 0, len(s.order))
	for _, k := range s.order {
		f := *s.byKey[k]
		sort.SliceStable(f.Spans, func(i, j int) bool {
			if f.Spans[i].Segment != f.Spans[j].Segment {
				return f.Spans[i].Segment < f.Spans[j].Segment
			}
			return f.Spans[i].Start < f.Spans[j].Start
		})
		out = append(out, f)
	}
	return out
}
