package guardrails

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	azureContentSafetyAPIVersion = "2024-09-01"
	// azureContentSafetyMaxChars is the most text, in code points, text analysis accepts per
	// request, and the most Prompt Shields accepts in the user prompt and in all documents.
	azureContentSafetyMaxChars = 10000
	// azureContentSafetyMaxDocuments is the most documents Prompt Shields accepts per request.
	azureContentSafetyMaxDocuments = 5
)

var (
	azureContentSafetyCategories = []string{"Hate", "SelfHarm", "Sexual", "Violence"}
)

type azureContentSafetyChecker struct {
	client            *http.Client
	analyzeURL        string
	shieldURL         string
	apiKey            string
	categories        []string
	severityThreshold int
	promptShields     bool
}

type azureContentSafetyAnalyzeRequest struct {
	Text       string   `json:"text"`
	Categories []string `json:"categories,omitempty"`
	OutputType string   `json:"outputType"`
}

type azureContentSafetyAnalyzeResponse struct {
	CategoriesAnalysis []struct {
		Category string `json:"category"`
		Severity int    `json:"severity"`
	} `json:"categoriesAnalysis"`
}

type azureContentSafetyShieldRequest struct {
	UserPrompt string   `json:"userPrompt,omitempty"`
	Documents  []string `json:"documents,omitempty"`
}

type azureContentSafetyShieldResponse struct {
	UserPromptAnalysis azureContentSafetyAttack   `json:"userPromptAnalysis"`
	DocumentsAnalysis  []azureContentSafetyAttack `json:"documentsAnalysis"`
}

type azureContentSafetyAttack struct {
	AttackDetected bool `json:"attackDetected"`
}

func init() {
	Register(ProviderInfo{
		Type:        "azure-content-safety",
		Label:       "Azure AI Content Safety",
		Category:    "Integrations",
		Description: "Detects hateful, sexual, violent and self-harm content with Azure AI Content Safety, and optionally prompt injection attacks with Prompt Shields.",
		Fields: []Field{
			{Name: "endpoint", Label: "Endpoint", Type: FieldText, Destination: true, Required: true, Description: "The Content Safety resource endpoint, e.g. https://my-resource.cognitiveservices.azure.com."},
			{Name: "apiKey", Label: "API key", Type: FieldPassword, Required: true, Sensitive: true},
			{Name: "categories", Label: "Categories", Type: FieldList, Options: azureContentSafetyCategories, Description: "Harm categories to check. Leave empty to check all of them."},
			{Name: "severityThreshold", Label: "Severity threshold", Type: FieldNumber, Default: 4, Description: "Severity (0, 2, 4 or 6) at which content counts as a finding."},
			{Name: "promptShields", Label: "Detect prompt attacks", Type: FieldBool, Description: "Also check user prompts for jailbreak attempts and tool results for indirect prompt injection, on input."},
		},
	}, func(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
		categories := ConfigStrings(config, "categories")
		for _, c := range categories {
			if !slices.Contains(azureContentSafetyCategories, c) {
				return nil, fmt.Errorf("unknown Azure AI Content Safety category %q", c)
			}
		}
		endpoint := strings.TrimRight(ConfigString(config, "endpoint", ""), "/")
		return &azureContentSafetyChecker{
			client:            integrationClient(deps),
			analyzeURL:        endpoint + "/contentsafety/text:analyze?api-version=" + azureContentSafetyAPIVersion,
			shieldURL:         endpoint + "/contentsafety/text:shieldPrompt?api-version=" + azureContentSafetyAPIVersion,
			apiKey:            secrets["apiKey"],
			categories:        categories,
			severityThreshold: int(ConfigFloat(config, "severityThreshold", 4)),
			promptShields:     ConfigBool(config, "promptShields", false),
		}, nil
	})
}

func (c *azureContentSafetyChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	headers := map[string]string{"Ocp-Apim-Subscription-Key": c.apiKey}

	// The service can't locate what it finds, so segments are packed together into as few
	// requests as possible. Severity is kept per category, as the highest seen.
	var texts []string
	for _, seg := range in.Segments {
		texts = append(texts, seg.Text)
	}
	severities := map[string]int{}
	for _, text := range joinChunks(texts, azureContentSafetyMaxChars) {
		var resp azureContentSafetyAnalyzeResponse
		if err := integrationPost(ctx, c.client, "Azure AI Content Safety", c.analyzeURL, headers, azureContentSafetyAnalyzeRequest{
			Text:       text,
			Categories: c.categories,
			OutputType: "FourSeverityLevels",
		}, &resp); err != nil {
			return Result{}, err
		}
		for _, a := range resp.CategoriesAnalysis {
			if s, ok := severities[a.Category]; !ok || a.Severity > s {
				severities[a.Category] = a.Severity
			}
		}
	}

	var result Result
	for _, category := range azureContentSafetyCategories {
		if s, ok := severities[category]; ok && s >= c.severityThreshold {
			result.Findings = append(result.Findings, Finding{
				Category: azureContentSafetyFindingCategory(category),
				Message:  fmt.Sprintf("%s content detected (severity %d)", category, s),
				Score:    float64(s),
			})
		}
	}

	if c.promptShields && in.Stage == StageInput {
		findings, err := c.shield(ctx, headers, in.Segments)
		if err != nil {
			return Result{}, err
		}
		result.Findings = append(result.Findings, findings...)
	}
	return result, nil
}

// shield checks user messages for direct prompt attacks and tool results for indirect ones.
func (c *azureContentSafetyChecker) shield(ctx context.Context, headers map[string]string, segments []Segment) ([]Finding, error) {
	var user, tool []string
	for _, seg := range segments {
		switch seg.Role {
		case RoleUser:
			user = append(user, seg.Text)
		case RoleToolResult:
			tool = append(tool, seg.Text)
		}
	}
	prompts := joinChunks(user, azureContentSafetyMaxChars)
	documents := groupChunks(tool, azureContentSafetyMaxChars, azureContentSafetyMaxChars, azureContentSafetyMaxDocuments, 0)

	var direct, indirect bool
	// Each request carries the next part of the user prompt and the next batch of documents.
	for i := 0; i < max(len(prompts), len(documents)); i++ {
		var req azureContentSafetyShieldRequest
		if i < len(prompts) {
			req.UserPrompt = prompts[i]
		}
		if i < len(documents) {
			req.Documents = documents[i]
		}
		var resp azureContentSafetyShieldResponse
		if err := integrationPost(ctx, c.client, "Azure AI Content Safety", c.shieldURL, headers, req, &resp); err != nil {
			return nil, err
		}
		direct = direct || (req.UserPrompt != "" && resp.UserPromptAnalysis.AttackDetected)
		indirect = indirect || slices.ContainsFunc(resp.DocumentsAnalysis, func(d azureContentSafetyAttack) bool { return d.AttackDetected })
	}

	var findings []Finding
	if direct {
		findings = append(findings, Finding{Category: "prompt_injection", Message: "Prompt attack detected in the user prompt"})
	}
	if indirect {
		findings = append(findings, Finding{Category: "prompt_injection", Message: "Indirect prompt attack detected in a tool result"})
	}
	return findings, nil
}

// azureContentSafetyFindingCategory turns a harm category like SelfHarm into self_harm.
func azureContentSafetyFindingCategory(category string) string {
	var b strings.Builder
	for i, r := range category {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// groupChunks splits the non-empty texts into chunks of at most chunkLimit code points and
// groups consecutive chunks, at most maxChunks to a group, so that each group totals at
// most groupLimit code points counting sepLen code points between chunks.
func groupChunks(texts []string, chunkLimit, groupLimit, maxChunks, sepLen int) [][]string {
	var (
		groups [][]string
		group  []string
		size   int
	)
	for _, text := range texts {
		if text == "" {
			continue
		}
		for _, chunk := range chunkByCodePoints(text, chunkLimit) {
			n := utf8.RuneCountInString(chunk.Text)
			if len(group) > 0 && (len(group) == maxChunks || size+sepLen+n > groupLimit) {
				groups = append(groups, group)
				group, size = nil, 0
			}
			if len(group) > 0 {
				size += sepLen
			}
			group = append(group, chunk.Text)
			size += n
		}
	}
	if len(group) > 0 {
		groups = append(groups, group)
	}
	return groups
}

// joinChunks packs the non-empty texts, separated by blank lines, into as few texts of at
// most limit code points as possible.
func joinChunks(texts []string, limit int) []string {
	var out []string
	for _, group := range groupChunks(texts, limit, limit, math.MaxInt, 2) {
		out = append(out, strings.Join(group, "\n\n"))
	}
	return out
}
