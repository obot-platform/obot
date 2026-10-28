package guardrails

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

var (
	// piiPatterns are the patterns the regex checker's PII template turns on.
	piiPatterns = map[string]string{
		"EMAIL": `[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`,
		// North American numbers, with or without +1, and international numbers written
		// with their + country code, such as +44 20 7946 0958.
		"PHONE":  `(?:\+?1[\s.\-]?)?\(?\b[2-9]\d{2}\)?[\s.\-]?\d{3}[\s.\-]?\d{4}\b|\+[1-9]\d{0,3}(?:[\s.\-]?(?:\(\d{1,5}\)|\d{1,5})){1,6}\b`,
		"US_SSN": `\b\d{3}-\d{2}-\d{4}\b`,
		// 13 to 16 digits, optionally grouped by spaces or dashes, but never ending in one.
		"CREDIT_CARD": `\b\d(?:[ \-]?\d){12,15}\b`,
		// IPv4 addresses, and candidates for IPv6 addresses that ipv6Valid confirms.
		"IP_ADDRESS": `\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b|[0-9A-Fa-f]{0,4}(?::[0-9A-Fa-f]{0,4}){2,7}(?:(?:\.\d{1,3}){3})?`,
		// IBANs, printed in groups of four or not; ibanValid confirms the check digits.
		"IBAN": `\b[A-Z]{2}\d{2}(?: ?[A-Z0-9]{4}){2,7}(?: ?[A-Z0-9]{1,4})?\b`,
	}

	// piiValidators confirm a PII pattern's match, given the text it was found in, to keep
	// look-alikes out: numbers that fail a checksum, or a C++ name taken for an IPv6 address.
	// They return where the match really ends, which may be before the pattern's match.
	piiValidators = map[string]func(text string, start, end int) (int, bool){
		"CREDIT_CARD": func(text string, start, end int) (int, bool) { return end, luhnValid(text[start:end]) },
		"PHONE":       phoneValid,
		"IP_ADDRESS":  ipAddressValid,
		"IBAN":        ibanMatch,
	}

	// datePattern finds dates such as 2026-10-17 or 17.10.2026, which a + before them would
	// make look like a phone number.
	datePattern = regexp.MustCompile(`\b(?:\d{4}[-./]\d{1,2}[-./]\d{1,2}|\d{1,2}[-./]\d{1,2}[-./]\d{4})\b`)
)

type namedPattern struct {
	label string
	re    *regexp.Regexp
	// valid confirms a match and returns where it ends; nil accepts every match.
	valid func(text string, start, end int) (int, bool)
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
			{Name: "piiTemplate", Label: "Detect common PII", Type: FieldList, Options: []string{"EMAIL", "PHONE", "US_SSN", "CREDIT_CARD", "IP_ADDRESS", "IBAN"}, Description: "Built-in patterns to use. Phone numbers are North American, or international with their + country code; IP addresses are IPv4 or IPv6; IBANs may be grouped by spaces. Credit card numbers and IBANs are confirmed with a checksum."},
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
		patterns = append(patterns, namedPattern{label: name, re: regexp.MustCompile(p), valid: piiValidators[name]})
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
				if p.valid != nil {
					end, ok := p.valid(seg.Text, loc[0], loc[1])
					if !ok {
						continue
					}
					loc = []int{loc[0], end}
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

// phoneValid confirms a phone number. A number written with its + country code must have as
// many digits as a phone number can (8 to 15), must not follow a letter or digit, as in a
// sum such as 3+4, and must not hold a date, as in "+1 2026-10-17".
func phoneValid(text string, start, end int) (int, bool) {
	if text[start] != '+' {
		return end, true
	}
	if start > 0 && isWordByte(text[start-1]) {
		return end, false
	}
	if datePattern.MatchString(text[start:end]) {
		return end, false
	}
	digits := 0
	for _, r := range text[start:end] {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return end, digits >= 8 && digits <= 15
}

// ipAddressValid confirms an IP address. An IPv4 address must not be part of a longer dotted
// run of numbers, such as the version 1.2.3.4.5. An IPv6 candidate must parse as one, must
// have a digit, which keeps out hex-looking names in code such as dead::beef or abc::def, and
// must stand on its own: not be part of a longer word, as in std::cout, a time such as
// 10:30:00, or a MAC address.
func ipAddressValid(text string, start, end int) (int, bool) {
	candidate := text[start:end]
	if !strings.Contains(candidate, ":") {
		if start > 1 && text[start-1] == '.' && isDigit(text[start-2]) {
			return end, false
		}
		if end+1 < len(text) && text[end] == '.' && isDigit(text[end+1]) {
			return end, false
		}
		return end, true
	}
	if !strings.ContainsAny(candidate, "0123456789") {
		// Names in code such as abc::def, and "::" alone, as in Haskell's x :: Int.
		return end, false
	}
	if start > 0 && (isWordByte(text[start-1]) || text[start-1] == ':' || text[start-1] == '.') {
		return end, false
	}
	if end < len(text) && (isWordByte(text[end]) || text[end] == ':') {
		return end, false
	}
	addr, err := netip.ParseAddr(candidate)
	return end, err == nil && addr.Is6()
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isWordByte(b byte) bool {
	return b == '_' || isDigit(b) || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

// ibanMatch confirms an IBAN. A printed IBAN's last group may be followed by a word in
// capitals that the pattern took for another group, so groups are dropped from the end until
// the rest is an IBAN.
func ibanMatch(text string, start, end int) (int, bool) {
	for {
		if ibanValid(text[start:end]) {
			return end, true
		}
		cut := strings.LastIndexByte(text[start:end], ' ')
		if cut < 0 {
			return end, false
		}
		end = start + cut
	}
}

// ibanValid confirms an IBAN's length and its ISO 13616 check digits: moving the first four
// characters to the end and reading letters as 10 to 35 gives a number whose remainder
// modulo 97 is 1.
func ibanValid(s string) bool {
	s = strings.ReplaceAll(s, " ", "")
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	remainder := 0
	for _, r := range s[4:] + s[:4] {
		switch {
		case r >= '0' && r <= '9':
			remainder = (remainder*10 + int(r-'0')) % 97
		case r >= 'A' && r <= 'Z':
			remainder = (remainder*100 + int(r-'A') + 10) % 97
		default:
			return false
		}
	}
	return remainder == 1
}
