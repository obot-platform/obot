package scim

import (
	"errors"
	"testing"
)

func TestParseListFilter(t *testing.T) {
	tests := []struct {
		name          string
		filter        string
		wantAttribute string
		wantValue     string
		wantScimType  string
	}{
		{
			name:          "Okta userName lookup",
			filter:        `userName eq "user@example.com"`,
			wantAttribute: "userName",
			wantValue:     "user@example.com",
		},
		{
			name:          "attribute names and operators are case-insensitive",
			filter:        `USERNAME EQ "user@example.com"`,
			wantAttribute: "userName",
			wantValue:     "user@example.com",
		},
		{
			name:          "escaped quotes and backslashes",
			filter:        `userName eq "a\"b\\c"`,
			wantAttribute: "userName",
			wantValue:     `a"b\c`,
		},
		{
			name:          "core schema URN prefix",
			filter:        `urn:ietf:params:scim:schemas:core:2.0:User:userName eq "user@example.com"`,
			wantAttribute: "userName",
			wantValue:     "user@example.com",
		},
		{
			name:          "externalId lookup, as Entra documents it",
			filter:        `externalId eq "6d1a5c1e-2f43-4f4e-9b1a-0c2d3e4f5a6b"`,
			wantAttribute: "externalId",
			wantValue:     "6d1a5c1e-2f43-4f4e-9b1a-0c2d3e4f5a6b",
		},
		{
			name:         "unsupported attribute",
			filter:       `emails eq "user@example.com"`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:         "unsupported operator",
			filter:       `userName sw "user"`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:         "logical expressions are not supported for lists",
			filter:       `userName eq "a" or userName eq "b"`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:         "non-string value",
			filter:       `userName eq true`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:         "extension schema",
			filter:       `urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:employeeNumber eq "1"`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:         "unterminated string",
			filter:       `userName eq "user`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:         "missing value",
			filter:       `userName eq`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:          "SQL is only ever a value",
			filter:        `userName eq "x' OR '1'='1"`,
			wantAttribute: "userName",
			wantValue:     `x' OR '1'='1`,
		},
		{
			name:         "trailing tokens",
			filter:       `userName eq "a" "b"`,
			wantScimType: scimTypeInvalidFilter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parseListFilter(userResourceSchema, tt.filter, "userName", "id", "externalId")
			if tt.wantScimType != "" {
				var scimErr *Error
				if !errors.As(err, &scimErr) || scimErr.ScimType != tt.wantScimType {
					t.Fatalf("parseListFilter() error = %v, want scimType %q", err, tt.wantScimType)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseListFilter() error = %v", err)
			}
			if f.Attribute != tt.wantAttribute || f.Value != tt.wantValue {
				t.Fatalf("parseListFilter() = %+v, want %s %q", f, tt.wantAttribute, tt.wantValue)
			}
		})
	}
}

func TestFilterMatches(t *testing.T) {
	member := map[string]any{
		"value":   "0f8c",
		"display": "Alice@Example.com",
		"type":    "User",
	}

	tests := []struct {
		name   string
		filter string
		want   bool
	}{
		{
			name:   "value equality",
			filter: `value eq "0f8c"`,
			want:   true,
		},
		{
			name:   "case-insensitive values",
			filter: `display eq "alice@example.com"`,
			want:   true,
		},
		{
			name:   "and",
			filter: `value eq "0f8c" and type eq "User"`,
			want:   true,
		},
		{
			name:   "or",
			filter: `value eq "nope" or type eq "user"`,
			want:   true,
		},
		{
			name:   "not",
			filter: `not (value eq "0f8c")`,
			want:   false,
		},
		{
			name:   "present",
			filter: `display pr`,
			want:   true,
		},
		{
			name:   "absent",
			filter: `$ref pr`,
			want:   false,
		},
		{
			name:   "starts with",
			filter: `display sw "alice"`,
			want:   true,
		},
		{
			name:   "grouping",
			filter: `(value eq "x" or value eq "0f8c") and type ne "Group"`,
			want:   true,
		},
	}

	members := groupResourceSchema.attribute("members")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parseFilter(tt.filter)
			if err != nil {
				t.Fatalf("parseFilter() error = %v", err)
			}
			if got := f.matches(members, member); got != tt.want {
				t.Fatalf("matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSupportedFilters(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{
			name:  "one",
			names: []string{"userName"},
			want:  `"userName eq"`,
		},
		{
			name:  "two",
			names: []string{"userName", "id"},
			want:  `"userName eq" and "id eq"`,
		},
		{
			name:  "three",
			names: []string{"userName", "id", "externalId"},
			want:  `"userName eq", "id eq", and "externalId eq"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportedFilters(tt.names); got != tt.want {
				t.Fatalf("supportedFilters() = %s, want %s", got, tt.want)
			}
		})
	}
}
