package types

import (
	"testing"
)

func TestSCIMUserNameKey(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		want     string
		wantErr  bool
	}{
		{
			name:     "case is mapped",
			userName: "BJensen@Example.com",
			want:     "bjensen@example.com",
		},
		{
			name:     "a decomposed character is composed",
			userName: "josé@example.com",
			want:     "josé@example.com",
		},
		{
			name:     "full-width characters are mapped",
			userName: "ｂｊｅｎｓｅｎ@example.com",
			want:     "bjensen@example.com",
		},
		{
			name:     "whitespace around and between parts is one space",
			userName: "  Barbara \t Jensen  ",
			want:     "barbara jensen",
		},
		{
			name:     "a control character is not allowed",
			userName: "bjensen\u0007@example.com",
			wantErr:  true,
		},
		{
			name:     "only whitespace",
			userName: " \t ",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SCIMUserNameKey(tt.userName)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SCIMUserNameKey(%q) = %q, want an error", tt.userName, got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("SCIMUserNameKey(%q) = %q, %v, want %q", tt.userName, got, err, tt.want)
			}
		})
	}
}
