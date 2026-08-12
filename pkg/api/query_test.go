package api

import (
	"slices"
	"testing"
)

func TestParseUintList(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []uint
	}{
		{name: "no values"},
		{name: "repeated values", values: []string{"42", "7", "9"}, want: []uint{42, 7, 9}},
		{name: "comma-separated values", values: []string{"42,7,9"}, want: []uint{42, 7, 9}},
		{name: "trims whitespace", values: []string{" 42, 7 ", " 9 "}, want: []uint{42, 7, 9}},
		{name: "ignores blank values", values: []string{"", "  ", ",", "42,,7"}, want: []uint{42, 7}},
		{name: "ignores invalid and zero values", values: []string{"invalid", "-1", "0", "42"}, want: []uint{42}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseUintList(tt.values); !slices.Equal(got, tt.want) {
				t.Fatalf("ParseUintList(%v) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}
