package guardrails

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	azureLanguageAPIVersion = "2023-04-01"
	// Synchronous PII detection accepts up to 5 documents per request, each at most 5,120
	// text elements long. Chunks are measured in code points, which never undercount text
	// elements, and kept a little under the limit.
	azureLanguageMaxDocuments = 5
	azureLanguageMaxChars     = 5000
)

type azureLanguagePIIChecker struct {
	client        *http.Client
	url           string
	apiKey        string
	language      string
	domain        string
	categories    []string
	minConfidence float64
}

type azureLanguageRequest struct {
	Kind          string                  `json:"kind"`
	Parameters    azureLanguageParameters `json:"parameters"`
	AnalysisInput struct {
		Documents []azureLanguageDocument `json:"documents"`
	} `json:"analysisInput"`
}

type azureLanguageParameters struct {
	ModelVersion    string   `json:"modelVersion"`
	Domain          string   `json:"domain,omitempty"`
	PIICategories   []string `json:"piiCategories,omitempty"`
	StringIndexType string   `json:"stringIndexType"`
	LoggingOptOut   bool     `json:"loggingOptOut"`
}

type azureLanguageDocument struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Text     string `json:"text"`
}

type azureLanguageResponse struct {
	Results struct {
		Documents []struct {
			ID       string `json:"id"`
			Entities []struct {
				Category        string  `json:"category"`
				Offset          int     `json:"offset"`
				Length          int     `json:"length"`
				ConfidenceScore float64 `json:"confidenceScore"`
			} `json:"entities"`
		} `json:"documents"`
		Errors []struct {
			ID    string `json:"id"`
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"errors"`
	} `json:"results"`
}

// azureLanguageChunk is a document sent for analysis and where it came from.
type azureLanguageChunk struct {
	segment int
	chunk   textChunk
}

func init() {
	Register(ProviderInfo{
		Type:        "azure-language-pii",
		Label:       "Azure AI Language PII",
		Category:    "Integrations",
		Description: "Detects personal and health data with Azure AI Language PII detection, and locates it for redaction.",
		Redacts:     true,
		Fields: []Field{
			{Name: "endpoint", Label: "Endpoint", Type: FieldText, Destination: true, Required: true, Description: "The Language resource endpoint, e.g. https://my-resource.cognitiveservices.azure.com."},
			{Name: "apiKey", Label: "API key", Type: FieldPassword, Required: true, Sensitive: true},
			{Name: "language", Label: "Language", Type: FieldText, Default: "en", Description: "Language code of the analyzed text."},
			{Name: "domain", Label: "Domain", Type: FieldSelect, Options: []string{"none", "phi"}, Default: "none", Description: "Use phi to also detect protected health information."},
			{Name: "categories", Label: "Categories", Type: FieldList, Description: "PII categories to detect, e.g. Email, PhoneNumber or USSocialSecurityNumber. Leave empty to detect every category."},
			{Name: "minConfidence", Label: "Minimum confidence", Type: FieldNumber, Default: 0.8, Description: "Confidence from 0 to 1 an entity needs to count as a finding."},
		},
	}, func(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
		domain := ConfigString(config, "domain", "")
		if domain == "none" {
			domain = ""
		}
		return &azureLanguagePIIChecker{
			client:        integrationClient(deps),
			url:           strings.TrimRight(ConfigString(config, "endpoint", ""), "/") + "/language/:analyze-text?api-version=" + azureLanguageAPIVersion,
			apiKey:        secrets["apiKey"],
			language:      ConfigString(config, "language", "en"),
			domain:        domain,
			categories:    ConfigStrings(config, "categories"),
			minConfidence: ConfigFloat(config, "minConfidence", 0.8),
		}, nil
	})
}

func (c *azureLanguagePIIChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	var chunks []azureLanguageChunk
	for i, seg := range in.Segments {
		if seg.Text == "" {
			continue
		}
		for _, chunk := range chunkByCodePoints(seg.Text, azureLanguageMaxChars) {
			chunks = append(chunks, azureLanguageChunk{segment: i, chunk: chunk})
		}
	}

	findings := newFindingSet("pii")
	for start := 0; start < len(chunks); start += azureLanguageMaxDocuments {
		batch := chunks[start:min(start+azureLanguageMaxDocuments, len(chunks))]
		if err := c.analyze(ctx, batch, findings); err != nil {
			return Result{}, err
		}
	}
	return findings.result(), nil
}

// analyze sends one request's worth of documents, identified by their index in batch, and
// adds what it finds. The service's redactedText isn't used: it masks every entity
// regardless of confidence, while spans honor the minimum confidence.
func (c *azureLanguagePIIChecker) analyze(ctx context.Context, batch []azureLanguageChunk, findings *findingSet) error {
	req := azureLanguageRequest{
		Kind: "PiiEntityRecognition",
		Parameters: azureLanguageParameters{
			ModelVersion:    "latest",
			Domain:          c.domain,
			PIICategories:   c.categories,
			StringIndexType: "UnicodeCodePoint",
			// Don't let the service keep the checked text for troubleshooting.
			LoggingOptOut: true,
		},
	}
	for i, ch := range batch {
		req.AnalysisInput.Documents = append(req.AnalysisInput.Documents, azureLanguageDocument{
			ID:       strconv.Itoa(i),
			Language: c.language,
			Text:     ch.chunk.Text,
		})
	}

	var resp azureLanguageResponse
	if err := integrationPost(ctx, c.client, "Azure AI Language", c.url, map[string]string{"Ocp-Apim-Subscription-Key": c.apiKey}, req, &resp); err != nil {
		return err
	}
	// A document the service couldn't analyze is content that went unchecked.
	if len(resp.Results.Errors) > 0 {
		e := resp.Results.Errors[0].Error
		return fmt.Errorf("could not analyze a document with Azure AI Language: %s: %s", e.Code, e.Message)
	}

	for _, doc := range resp.Results.Documents {
		idx, err := strconv.Atoi(doc.ID)
		if err != nil || idx < 0 || idx >= len(batch) {
			return fmt.Errorf("unknown document id %q in the Azure AI Language response", doc.ID)
		}
		ch := batch[idx]
		offsets := codePointByteOffsets(ch.chunk.Text)
		for _, e := range doc.Entities {
			end := e.Offset + e.Length
			if e.ConfidenceScore < c.minConfidence || e.Offset < 0 || e.Length <= 0 || end >= len(offsets) {
				continue
			}
			findings.add(e.Category, e.ConfidenceScore, Span{
				Segment: ch.segment,
				Start:   ch.chunk.Offset + offsets[e.Offset],
				End:     ch.chunk.Offset + offsets[end],
				Label:   e.Category,
			})
		}
	}
	return nil
}
