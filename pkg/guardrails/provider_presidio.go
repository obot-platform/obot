package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	presidioService = "Presidio"

	// integrationErrorBodyLimit bounds how much of a failed response is quoted in errors.
	integrationErrorBodyLimit = 512
)

var (
	// presidioDefaultEntities are the entity types a Presidio guardrail looks for when none are
	// chosen: personal data that is rarely legitimate in prompts. The Analyzer's other built-in
	// types (PERSON, LOCATION, DATE_TIME, NRP, URL) match ordinary text, such as names of public
	// figures, cities, "Friday" and links, and redacting them by default breaks conversations.
	presidioDefaultEntities = []string{
		"EMAIL_ADDRESS",
		"PHONE_NUMBER",
		"CREDIT_CARD",
		"US_SSN",
		"US_ITIN",
		"US_PASSPORT",
		"US_DRIVER_LICENSE",
		"US_BANK_NUMBER",
		"IBAN_CODE",
		"IP_ADDRESS",
	}
)

type presidioChecker struct {
	client         *http.Client
	url            string
	apiKey         string
	language       string
	entities       []string
	scoreThreshold float64
}

type presidioRequest struct {
	Text           string   `json:"text"`
	Language       string   `json:"language"`
	Entities       []string `json:"entities,omitempty"`
	ScoreThreshold float64  `json:"score_threshold"`
}

// presidioEntity is one entity the analyzer recognized. Start and End are Python string
// indices, i.e. Unicode code point offsets.
type presidioEntity struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

// textChunk is a piece of a segment's text, for services that limit input length.
type textChunk struct {
	Text string
	// Offset is the chunk's byte offset in the segment's text.
	Offset int
}

// findingSet collects findings by label, in the order labels are first seen.
type findingSet struct {
	category string
	byLabel  map[string]*Finding
	order    []string
}

func init() {
	Register(ProviderInfo{
		Type:        "presidio",
		Label:       "Microsoft Presidio",
		Category:    "Integrations",
		Description: "Detects personal data with a self-hosted Microsoft Presidio Analyzer, and locates it for redaction.",
		Redacts:     true,
		Fields: []Field{
			{Name: "analyzerURL", Label: "Analyzer URL", Type: FieldText, Destination: true, Required: true, Description: "Base URL of the Presidio Analyzer service, e.g. http://presidio-analyzer:3000."},
			{Name: "language", Label: "Language", Type: FieldText, Default: "en", Description: "Language code of the analyzed text."},
			{Name: "entities", Label: "Entities", Type: FieldList, Default: presidioDefaultEntities, OptionsFromService: true, Description: "Entity types to detect. With none chosen, the default personal data types are used."},
			{Name: "scoreThreshold", Label: "Minimum score", Type: FieldNumber, Default: 0.5, Description: "Confidence from 0 to 1 an entity needs to count as a finding."},
			{Name: "apiKey", Label: "API key", Type: FieldPassword, Sensitive: true, Description: "Sent as a bearer token, for analyzers behind an authenticating proxy."},
		},
	}, func(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
		entities := ConfigStrings(config, "entities")
		if len(entities) == 0 {
			entities = presidioDefaultEntities
		}
		return &presidioChecker{
			client:         integrationClient(deps),
			url:            strings.TrimRight(ConfigString(config, "analyzerURL", ""), "/") + "/analyze",
			apiKey:         secrets["apiKey"],
			language:       ConfigString(config, "language", "en"),
			entities:       entities,
			scoreThreshold: ConfigFloat(config, "scoreThreshold", 0.5),
		}, nil
	})
	RegisterOptions("presidio", "entities", presidioSupportedEntities)
}

// presidioSupportedEntities lists the entity types the Analyzer supports for the configured
// language, including its custom recognizers.
func presidioSupportedEntities(ctx context.Context, config map[string]any, secrets map[string]string, deps Deps) ([]string, error) {
	base := strings.TrimRight(ConfigString(config, "analyzerURL", ""), "/")
	if base == "" {
		return nil, fmt.Errorf("analyzer URL is required")
	}
	u, err := url.Parse(base + "/supportedentities")
	if err != nil {
		return nil, fmt.Errorf("invalid %s URL: %w", presidioService, err)
	}
	u.RawQuery = url.Values{"language": {ConfigString(config, "language", "en")}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid %s URL: %w", presidioService, err)
	}
	req.Header.Set("Accept", "application/json")
	if key := secrets["apiKey"]; key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := integrationClient(deps).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", presidioService, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, integrationErrorBodyLimit))
		return nil, fmt.Errorf("%s returned %s: %s", presidioService, resp.Status, strings.TrimSpace(strings.ToValidUTF8(string(msg), "")))
	}
	var entities []string
	if err := json.NewDecoder(resp.Body).Decode(&entities); err != nil {
		return nil, fmt.Errorf("invalid %s response: %w", presidioService, err)
	}
	slices.Sort(entities)
	return slices.Compact(entities), nil
}

func (c *presidioChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	var headers map[string]string
	if c.apiKey != "" {
		headers = map[string]string{"Authorization": "Bearer " + c.apiKey}
	}
	findings := newFindingSet("pii")
	for i, seg := range in.Segments {
		if seg.Text == "" {
			continue
		}
		var entities []presidioEntity
		if err := integrationPost(ctx, c.client, presidioService, c.url, headers, presidioRequest{
			Text:           seg.Text,
			Language:       c.language,
			Entities:       c.entities,
			ScoreThreshold: c.scoreThreshold,
		}, &entities); err != nil {
			return Result{}, err
		}
		offsets := codePointByteOffsets(seg.Text)
		for _, e := range entities {
			if e.Score < c.scoreThreshold || e.Start < 0 || e.End <= e.Start || e.End >= len(offsets) {
				continue
			}
			findings.add(e.EntityType, e.Score, Span{Segment: i, Start: offsets[e.Start], End: offsets[e.End], Label: e.EntityType})
		}
	}
	return findings.result(), nil
}

func newFindingSet(category string) *findingSet {
	return &findingSet{category: category, byLabel: map[string]*Finding{}}
}

// add records a located match. The finding's score is the highest of its matches.
func (s *findingSet) add(label string, score float64, span Span) {
	f, ok := s.byLabel[label]
	if !ok {
		f = &Finding{Category: s.category, Message: label + " detected"}
		s.byLabel[label] = f
		s.order = append(s.order, label)
	}
	f.Count++
	f.Score = max(f.Score, score)
	f.Spans = append(f.Spans, span)
}

func (s *findingSet) result() Result {
	var result Result
	for _, label := range s.order {
		result.Findings = append(result.Findings, *s.byLabel[label])
	}
	return result
}

// integrationClient returns the HTTP client for calls to external guardrail services.
func integrationClient(deps Deps) *http.Client {
	if deps.HTTPClient != nil {
		return deps.HTTPClient
	}
	return http.DefaultClient
}

// integrationPost sends body as JSON to an external guardrail service and decodes the JSON
// response into out. Non-2xx responses are errors that quote the start of the body.
func integrationPost(ctx context.Context, client *http.Client, service, url string, headers map[string]string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid %s URL: %w", service, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", service, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, integrationErrorBodyLimit))
		return fmt.Errorf("%s returned %s: %s", service, resp.Status, strings.TrimSpace(strings.ToValidUTF8(string(msg), "")))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("invalid %s response: %w", service, err)
	}
	return nil
}

// codePointByteOffsets maps code point offsets in text to byte offsets: element i is the
// byte offset of the i-th code point, and the last element is len(text). Invalid UTF-8
// bytes count as one code point each, matching how encoding/json replaces them with U+FFFD.
func codePointByteOffsets(text string) []int {
	offsets := make([]int, 0, utf8.RuneCountInString(text)+1)
	for i := range text {
		offsets = append(offsets, i)
	}
	return append(offsets, len(text))
}

// chunkByCodePoints splits text into chunks of at most limit code points. It prefers to
// break after whitespace near the end of a chunk so that words aren't split between
// chunks, where a service could miss an entity that straddles the boundary.
func chunkByCodePoints(text string, limit int) []textChunk {
	offsets := codePointByteOffsets(text)
	count := len(offsets) - 1
	var chunks []textChunk
	for start := 0; start < count; {
		end := min(start+limit, count)
		if end < count {
			// Look back up to a tenth of the limit for a place to break.
			for i := end; i > end-limit/10 && i > start+1; i-- {
				r, _ := utf8.DecodeRuneInString(text[offsets[i-1]:])
				if unicode.IsSpace(r) {
					end = i
					break
				}
			}
		}
		chunks = append(chunks, textChunk{Text: text[offsets[start]:offsets[end]], Offset: offsets[start]})
		start = end
	}
	return chunks
}
