package guardrails

import (
	"context"
	"math"
	"regexp"
)

var (
	// secretRules detect credentials by their well-known formats. They favor precision: each
	// pattern is specific to one issuer's token format, so ordinary text rarely matches.
	secretRules = []struct {
		label string
		re    *regexp.Regexp
		// group selects the submatch holding the secret; 0 means the whole match.
		group int
	}{
		{"PRIVATE_KEY", regexp.MustCompile(`-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----[\s\S]{16,}?-----END[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----`), 0},
		{"AWS_ACCESS_KEY_ID", regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16}\b`), 0},
		{"AWS_SECRET_ACCESS_KEY", regexp.MustCompile(`(?i)aws.{0,20}?(?:secret|private).{0,20}?['"=:\s]\s*['"]?([A-Za-z0-9/+=]{40})\b`), 1},
		{"GITHUB_TOKEN", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36,255}|github_pat_[A-Za-z0-9_]{82})\b`), 0},
		{"GITLAB_TOKEN", regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{20}\b`), 0},
		{"SLACK_TOKEN", regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,72}\b`), 0},
		{"SLACK_WEBHOOK", regexp.MustCompile(`https://hooks\.slack\.com/(?:services|workflows)/[A-Za-z0-9+/]{43,56}`), 0},
		{"STRIPE_KEY", regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{24,99}\b`), 0},
		{"GOOGLE_API_KEY", regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), 0},
		{"OPENAI_API_KEY", regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_\-]{20,}T3BlbkFJ[A-Za-z0-9_\-]{20,}\b|\bsk-proj-[A-Za-z0-9_\-]{40,}\b`), 0},
		{"ANTHROPIC_API_KEY", regexp.MustCompile(`\bsk-ant-(?:api|admin)\d{2}-[A-Za-z0-9_\-]{80,}\b`), 0},
		{"AZURE_STORAGE_KEY", regexp.MustCompile(`AccountKey=([A-Za-z0-9+/]{86}==)`), 1},
		{"TWILIO_API_KEY", regexp.MustCompile(`\bSK[0-9a-fA-F]{32}\b`), 0},
		{"SENDGRID_API_KEY", regexp.MustCompile(`\bSG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}\b`), 0},
		{"NPM_TOKEN", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), 0},
		{"HUGGINGFACE_TOKEN", regexp.MustCompile(`\bhf_[A-Za-z]{34}\b`), 0},
		{"JWT", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`), 0},
		{"DATABASE_URL", regexp.MustCompile(`\b(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp)://[^\s:@/]+:([^\s@/]{3,})@`), 1},
		{"OBOT_API_KEY", regexp.MustCompile(`\bok1-\d+-\d+-[A-Za-z0-9]{20,}\b`), 0},
	}

	// genericSecret finds values assigned to secret-looking names; they only count when
	// random enough to be a real credential.
	genericSecret = regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|passw(?:or)?d|credential|auth)[A-Za-z0-9_-]{0,20}\s*[:=]\s*['"]?([A-Za-z0-9_\-+/=.]{16,})`)
)

type secretsChecker struct {
	generic    bool
	minEntropy float64
}

func init() {
	Register(ProviderInfo{
		Type:        "secrets",
		Label:       "Secrets Detection",
		Category:    "Built-in",
		Description: "Detects leaked API keys, tokens, private keys and connection strings for AWS, GitHub, GitLab, Slack, Stripe, Google, OpenAI, Anthropic, Azure and more.",
		Redacts:     true,
		Fields: []Field{
			{Name: "generic", Label: "Also detect generic secrets", Type: FieldBool, Default: true, Description: "Flag high-entropy values assigned to names like api_key, token or password."},
			{Name: "minEntropy", Label: "Minimum entropy for generic secrets", Type: FieldNumber, Default: 3.5, Description: "Shannon entropy in bits per character; higher values flag fewer strings."},
		},
	}, func(config map[string]any, _ map[string]string, _ Deps) (Checker, error) {
		return &secretsChecker{
			generic:    ConfigBool(config, "generic", true),
			minEntropy: ConfigFloat(config, "minEntropy", 3.5),
		}, nil
	})
}

func (c *secretsChecker) Check(_ context.Context, in CheckInput) (Result, error) {
	byLabel := map[string]*Finding{}
	var order []string
	add := func(label string, seg, start, end int) {
		f, ok := byLabel[label]
		if !ok {
			f = &Finding{Category: "secret", Message: label + " detected"}
			byLabel[label] = f
			order = append(order, label)
		}
		f.Count++
		f.Spans = append(f.Spans, Span{Segment: seg, Start: start, End: end, Label: label})
	}
	for i, seg := range in.Segments {
		covered := make([]bool, len(seg.Text)+1)
		for _, r := range secretRules {
			for _, m := range r.re.FindAllStringSubmatchIndex(seg.Text, -1) {
				start, end := m[2*r.group], m[2*r.group+1]
				if start < 0 {
					continue
				}
				add(r.label, i, start, end)
				for p := start; p < end; p++ {
					covered[p] = true
				}
			}
		}
		if !c.generic {
			continue
		}
		for _, m := range genericSecret.FindAllStringSubmatchIndex(seg.Text, -1) {
			start, end := m[2], m[3]
			if covered[start] || shannonEntropy(seg.Text[start:end]) < c.minEntropy {
				continue
			}
			add("GENERIC_SECRET", i, start, end)
		}
	}
	var result Result
	for _, label := range order {
		result.Findings = append(result.Findings, *byLabel[label])
	}
	return result, nil
}

func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	var h float64
	n := float64(len([]rune(s)))
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}
