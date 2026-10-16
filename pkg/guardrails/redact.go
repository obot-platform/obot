package guardrails

import (
	"sort"
	"strings"
)

// RedactionText is what replaces redacted content.
func RedactionText(label string) string {
	if label == "" {
		label = "SENSITIVE"
	}
	return "[REDACTED:" + strings.ToUpper(strings.ReplaceAll(label, " ", "_")) + "]"
}

// ApplySpans returns the segments with every span replaced by a redaction marker.
// Overlapping spans are merged; spans outside a segment's text are ignored.
func ApplySpans(segments []Segment, spans []Span) []Segment {
	if len(spans) == 0 {
		return segments
	}
	bySegment := map[int][]Span{}
	for _, s := range spans {
		if s.Segment < 0 || s.Segment >= len(segments) || s.Start < 0 || s.End <= s.Start || s.End > len(segments[s.Segment].Text) {
			continue
		}
		bySegment[s.Segment] = append(bySegment[s.Segment], s)
	}
	out := make([]Segment, len(segments))
	copy(out, segments)
	for i, ss := range bySegment {
		out[i].Text = redactText(segments[i].Text, ss)
	}
	return out
}

func redactText(text string, spans []Span) string {
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	merged := []Span{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.Start <= last.End {
			if s.End > last.End {
				last.End = s.End
			}
			continue
		}
		merged = append(merged, s)
	}
	var b strings.Builder
	pos := 0
	for _, s := range merged {
		b.WriteString(text[pos:s.Start])
		b.WriteString(RedactionText(s.Label))
		pos = s.End
	}
	b.WriteString(text[pos:])
	return b.String()
}
