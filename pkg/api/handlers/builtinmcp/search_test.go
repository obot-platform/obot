package builtinmcp

import (
	"strings"
	"testing"
)

func TestMatchScore(t *testing.T) {
	score := func(query, name string, descriptions ...string) int {
		q := normalize(query)
		return matchScore(q, strings.Fields(q), name, descriptions...)
	}
	tests := []struct {
		name  string
		got   int
		want  int
		check func(int) bool
	}{
		{name: "exact name", got: score("exa", "Exa"), want: 100},
		{name: "name prefix", got: score("exa", "Exa Search"), want: 80},
		{name: "case and punctuation", got: score("EXA-search", "Exa Search"), want: 100},
		{name: "every word in the name", got: score("search exa", "Exa Search"), want: 60},
		{name: "description only", got: score("search", "Tavily", "Web search for agents"), want: 10},
		{name: "substring inside a word is no match", got: score("exa", "Monday.com", "Boards for Texas teams"), want: 0},
		{name: "word prefix in the description ranks low", got: score("exa", "Monday.com", "For example boards"), want: 10},
		{name: "missing word is no match", got: score("exa slack", "Exa Search"), want: 0},
		{name: "empty query matches weakly", got: score("", "Anything"), want: 1},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: score = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
	if score("exa", "Exa Search") <= score("exa", "Tavily", "Exa-compatible search") {
		t.Error("a name match must outrank a description match")
	}
}
