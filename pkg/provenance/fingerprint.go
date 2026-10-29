package provenance

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Fingerprints let the engine recognize sensitive content from an earlier event
// when it appears again in an outbound action, without storing the content.
//
// Two kinds are kept. Exact fingerprints are hashes of structured identifiers
// (email addresses, phone numbers, card and account numbers, API keys, IP
// addresses) and match only an identical identifier. Shingle fingerprints are
// hashes of normalized word n-grams and match a run of words copied with light
// edits. A model that paraphrases defeats shingles; it does not defeat the
// session-level taint, which is why lineage only ever lowers a decision's
// false-positive rate and never grants permission on its own.

const (
	shingleWords = 5
	// maxShinglesPerEvent bounds the fingerprints kept for one event so a huge
	// tool result cannot inflate session state without bound.
	maxShinglesPerEvent = 2048
	maxExactPerEvent    = 512
	minShingleTextWords = 12
)

var (
	emailRe   = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}\b`)
	phoneRe   = regexp.MustCompile(`\+?\d[\d\s().-]{8,}\d`)
	cardRe    = regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`)
	ipRe      = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	ssnRe     = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	apiKeyRe  = regexp.MustCompile(`\b(?:sk|pk|rk|ghp|gho|ghu|ghs|xox[bap]|AKIA|ASIA|sk-ant|hf)[-_][A-Za-z0-9_\-]{12,}\b|\bAKIA[0-9A-Z]{16}\b`)
	awsKeyRe  = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)
	jwtRe     = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	privKeyRe = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

	// identifierPatterns are the structured identifiers extracted exactly, with
	// the label each one carries.
	identifierPatterns = []struct {
		re    *regexp.Regexp
		label Label
	}{
		{privKeyRe, LabelSecret},
		{awsKeyRe, LabelSecret},
		{apiKeyRe, LabelSecret},
		{jwtRe, LabelSecret},
		{emailRe, LabelPII},
		{ssnRe, LabelPII},
		{cardRe, LabelFinancial},
		{phoneRe, LabelPII},
		{ipRe, LabelConfidential},
	}
)

// Fingerprint is one hashed span of content with its origin.
type Fingerprint struct {
	Hash  uint64 `json:"h"`
	Exact bool   `json:"x,omitempty"`
	// Label is the sensitivity label the span carries (pii, secret, ...).
	Label Label `json:"l"`
	// OriginEventID is the event that introduced the span.
	OriginEventID string `json:"o"`
}

// Classification is what a scan found in a piece of content.
type Classification struct {
	// Labels are the sensitivity labels the content carries.
	Labels Labels
	// Identifiers are the exact structured identifiers found, by label.
	Identifiers map[Label][]string
	// Spans locates each identifier in the scanned text.
	Spans []LabeledSpan
}

// LabeledSpan is a span with the label of what it contains.
type LabeledSpan struct {
	Span
	Label Label
}

// Classify scans text for structured sensitive identifiers. It is the built-in
// classifier; the guardrails checkers (Presidio, external services) can add
// labels on top of it through Engine options.
func Classify(text string) Classification {
	c := Classification{Identifiers: map[Label][]string{}}
	if text == "" {
		return c
	}
	seen := map[string]bool{}
	for _, p := range identifierPatterns {
		for _, loc := range p.re.FindAllStringIndex(text, -1) {
			id := text[loc[0]:loc[1]]
			if p.label == LabelFinancial && !luhnValid(id) {
				continue
			}
			if p.label == LabelPII && p.re == phoneRe && digitCount(id) < 10 {
				continue
			}
			key := string(p.label) + "\x00" + id
			if seen[key] {
				continue
			}
			seen[key] = true
			c.Identifiers[p.label] = append(c.Identifiers[p.label], id)
			c.Spans = append(c.Spans, LabeledSpan{Start: loc[0], End: loc[1], Label: p.label})
			c.Labels = c.Labels.Add(p.label)
		}
	}
	c.Labels = c.Labels.Normalize()
	return c
}

// FingerprintContent produces the fingerprints to remember for content that
// entered the session with the given labels. Exact fingerprints are made for
// every identifier found; shingles are made for the whole text when it is
// labelled confidential and long enough to be worth matching.
func FingerprintContent(originEventID, text string, labels Labels, class Classification) []Fingerprint {
	var out []Fingerprint
	for label, ids := range class.Identifiers {
		for _, id := range ids {
			if len(out) >= maxExactPerEvent {
				break
			}
			out = append(out, Fingerprint{Hash: hashExact(id), Exact: true, Label: label, OriginEventID: originEventID})
		}
	}
	if labels.Confidential() {
		label := LabelConfidential
		for _, l := range ConfidentialLabels {
			if labels.Has(l) {
				label = l
				break
			}
		}
		for _, h := range shingles(text, maxShinglesPerEvent) {
			out = append(out, Fingerprint{Hash: h, Label: label, OriginEventID: originEventID})
		}
	}
	return out
}

// MatchFingerprints finds which remembered fingerprints appear in text. The
// result is grouped by origin event and label.
func MatchFingerprints(text string, remembered []Fingerprint) []LineageHit {
	if text == "" || len(remembered) == 0 {
		return nil
	}
	index := make(map[uint64][]int, len(remembered))
	for i, fp := range remembered {
		index[fp.Hash] = append(index[fp.Hash], i)
	}

	type key struct {
		origin string
		label  Label
	}
	hits := map[key]*LineageHit{}
	matched := map[int]bool{}
	record := func(i int, exact bool) {
		if matched[i] {
			return
		}
		matched[i] = true
		fp := remembered[i]
		k := key{fp.OriginEventID, fp.Label}
		h, ok := hits[k]
		if !ok {
			h = &LineageHit{OriginEventID: fp.OriginEventID, Label: fp.Label}
			hits[k] = h
		}
		h.Count++
		h.Exact = h.Exact || exact
	}

	class := Classify(text)
	for _, ids := range class.Identifiers {
		for _, id := range ids {
			for _, i := range index[hashExact(id)] {
				if remembered[i].Exact {
					record(i, true)
				}
			}
		}
	}
	for _, h := range shingles(text, 0) {
		for _, i := range index[h] {
			if !remembered[i].Exact {
				record(i, false)
			}
		}
	}

	out := make([]LineageHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, *h)
	}
	sortLineage(out)
	return out
}

func sortLineage(hits []LineageHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0; j-- {
			a, b := hits[j-1], hits[j]
			if a.OriginEventID < b.OriginEventID || (a.OriginEventID == b.OriginEventID && a.Label <= b.Label) {
				break
			}
			hits[j-1], hits[j] = hits[j], hits[j-1]
		}
	}
}

func hashExact(s string) uint64 {
	sum := sha256.Sum256([]byte("x:" + strings.ToLower(strings.TrimSpace(s))))
	return binary.LittleEndian.Uint64(sum[:8])
}

func hashShingle(words []string) uint64 {
	sum := sha256.Sum256([]byte("s:" + strings.Join(words, " ")))
	return binary.LittleEndian.Uint64(sum[:8])
}

// shingles hashes every run of shingleWords normalized words. limit 0 means
// no limit.
func shingles(text string, limit int) []uint64 {
	words := normalizeWords(text)
	if len(words) < minShingleTextWords {
		return nil
	}
	n := len(words) - shingleWords + 1
	if limit > 0 && n > limit {
		n = limit
	}
	out := make([]uint64, 0, n)
	seen := make(map[uint64]bool, n)
	for i := 0; i < len(words)-shingleWords+1; i++ {
		h := hashShingle(words[i : i+shingleWords])
		if seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// normalizeWords lowercases, drops punctuation and splits on whitespace. Short
// stop-words are kept: dropping them makes shingles match more loosely than is
// safe for lineage.
func normalizeWords(text string) []string {
	var (
		words []string
		cur   strings.Builder
	)
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cur.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return words
}

func luhnValid(s string) bool {
	var digits []int
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits = append(digits, int(r-'0'))
		}
	}
	if len(digits) < 13 {
		return false
	}
	sum := 0
	double := false
	for _, d := range slices.Backward(digits) {
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

func digitCount(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

// FlattenArguments renders a tool's arguments (or any JSON-like value) as text
// for scanning. Keys are included so a policy or classifier can see field
// names.
func FlattenArguments(v any) string {
	var b strings.Builder
	flattenInto(&b, v, 0)
	return b.String()
}

func flattenInto(b *strings.Builder, v any, depth int) {
	if depth > 32 {
		return
	}
	switch t := v.(type) {
	case nil:
	case string:
		b.WriteString(t)
		b.WriteByte('\n')
	case map[string]any:
		for k, val := range t {
			b.WriteString(k)
			b.WriteString(": ")
			flattenInto(b, val, depth+1)
		}
	case []any:
		for _, val := range t {
			flattenInto(b, val, depth+1)
		}
	case bool:
		if t {
			b.WriteString("true\n")
		} else {
			b.WriteString("false\n")
		}
	default:
		fmt.Fprint(b, t)
		b.WriteByte('\n')
	}
}
