package devicescan

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	skillNameMaxRunes        = 100
	skillDescriptionMaxRunes = 1024
)

// parseFrontmatter extracts (name, description) from a SKILL.md byte
// stream. If the document has no leading "---" YAML frontmatter, or the
// block is malformed, both return values are empty strings — the caller is
// expected to fall back (e.g. directory name for the skill name).
func parseFrontmatter(data []byte) (name, description string) {
	s := strings.TrimLeft(string(data), " \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return "", ""
	}
	s = strings.TrimLeft(s[3:], "\r\n")
	end := strings.Index(s, "\n---")
	if end < 0 {
		return "", ""
	}
	block := s[:end]

	var fm map[string]any
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return "", ""
	}
	return clipRunes(asString(fm["name"]), skillNameMaxRunes),
		clipRunes(asString(fm["description"]), skillDescriptionMaxRunes)
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
