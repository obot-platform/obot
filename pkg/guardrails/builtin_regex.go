package guardrails

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	// piiPatterns are the patterns the regex checker's PII template turns on.
	piiPatterns = map[string]string{
		"EMAIL":       `[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`,
		"PHONE":       `(?:\+?1[\s.\-]?)?\(?\b[2-9]\d{2}\)?[\s.\-]?\d{3}[\s.\-]?\d{4}\b`,
		"US_SSN":      `\b\d{3}-\d{2}-\d{4}\b`,
		"CREDIT_CARD": `\b(?:\d[ \-]?){13,16}\b`,
		"IP_ADDRESS":  `\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b`,
		"IBAN":        `\b[A-Z]{2}\d{2}[A-Z0-9]{11,30}\b`,
	}
)

type namedPattern struct {
	label string
	re    *regexp.Regexp
}

type regexChecker struct {
	patterns []namedPattern
}

func init() {
	Register(ProviderInfo{
		Type:        "regex",
		Label:       "Custom Regex",
		Category:    "Built-in",
		Description: "Matches content against regular expressions, with ready-made patterns for common personal data.",
		Redacts:     true,
		Fields: []Field{
			{Name: "piiTemplate", Label: "Detect common PII", Type: FieldList, Options: []string{"EMAIL", "PHONE", "US_SSN", "CREDIT_CARD", "IP_ADDRESS", "IBAN"}, Description: "Built-in patterns to use. Credit card numbers are confirmed with a checksum."},
			{Name: "patterns", Label: "Custom patterns", Type: FieldList, Description: "One pattern per entry, in RE2 syntax. Name a pattern with LABEL=pattern, e.g. EMPLOYEE_ID=E\\d{6}."},
			{Name: "caseInsensitive", Label: "Case insensitive", Type: FieldBool},
		},
	}, newRegexChecker)
}

func newRegexChecker(config map[string]any, _ map[string]string, _ Deps) (Checker, error) {
	flags := ""
	if ConfigBool(config, "caseInsensitive", false) {
		flags = "(?i)"
	}
	var patterns []namedPattern
	for _, name := range ConfigStrings(config, "piiTemplate") {
		p, ok := piiPatterns[name]
		if !ok {
			return nil, fmt.Errorf("unknown PII type %q", name)
		}
		patterns = append(patterns, namedPattern{label: name, re: regexp.MustCompile(p)})
	}
	for i, raw := range ConfigStrings(config, "patterns") {
		label, expr := fmt.Sprintf("PATTERN_%d", i+1), raw
		if name, rest, ok := strings.Cut(raw, "="); ok && regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(name) {
			label, expr = name, rest
		}
		re, err := regexp.Compile(flags + expr)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", raw, err)
		}
		patterns = append(patterns, namedPattern{label: label, re: re})
	}
	if len(patterns) == 0 {
		return nil, fmt.Errorf("add at least one pattern or PII type")
	}
	return &regexChecker{patterns: patterns}, nil
}

func (c *regexChecker) Check(_ context.Context, in CheckInput) (Result, error) {
	return matchPatterns(c.patterns, in.Segments, "pii"), nil
}

// matchPatterns reports every match as a finding per label, with spans for redaction.
func matchPatterns(patterns []namedPattern, segments []Segment, category string) Result {
	byLabel := map[string]*Finding{}
	var order []string
	for i, seg := range segments {
		for _, p := range patterns {
			for _, loc := range p.re.FindAllStringIndex(seg.Text, -1) {
				if p.label == "CREDIT_CARD" && !luhnValid(seg.Text[loc[0]:loc[1]]) {
					continue
				}
				f, ok := byLabel[p.label]
				if !ok {
					f = &Finding{Category: category, Message: p.label + " detected"}
					byLabel[p.label] = f
					order = append(order, p.label)
				}
				f.Count++
				f.Spans = append(f.Spans, Span{Segment: i, Start: loc[0], End: loc[1], Label: p.label})
			}
		}
	}
	var result Result
	for _, label := range order {
		result.Findings = append(result.Findings, *byLabel[label])
	}
	return result
}

func luhnValid(s string) bool {
	var digits []int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits = append(digits, int(r-'0'))
		}
	}
	if len(digits) < 13 {
		return false
	}
	sum := 0
	for i, d := range slices.Backward(digits) {
		if (len(digits)-1-i)%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}
