package server

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestParseGroupPageParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "defaults when absent",
			query:      "",
			wantLimit:  defaultGroupPageSize,
			wantOffset: 0,
		},
		{
			name:       "explicit values",
			query:      "limit=25&offset=100",
			wantLimit:  25,
			wantOffset: 100,
		},
		{
			name:       "limit capped",
			query:      "limit=100000",
			wantLimit:  maxGroupPageSize,
			wantOffset: 0,
		},
		{
			name:      "zero limit falls back to default",
			query:     "limit=0",
			wantLimit: defaultGroupPageSize,
		},
		{
			name:      "negative limit falls back to default",
			query:     "limit=-1",
			wantLimit: defaultGroupPageSize,
		},
		{
			name:      "unparseable limit falls back to default",
			query:     "limit=many",
			wantLimit: defaultGroupPageSize,
		},
		{
			name:       "negative offset clamps",
			query:      "limit=10&offset=-5",
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name:       "unparseable offset clamps",
			query:      "limit=10&offset=abc",
			wantLimit:  10,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("bad test query: %v", err)
			}

			limit, offset := parseGroupPageParams(query)
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
			if offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}

func TestSplitGroupIDs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "single",
			raw:  "entra/a",
			want: []string{"entra/a"},
		},
		{
			name: "multiple",
			raw:  "entra/a,entra/b",
			want: []string{"entra/a", "entra/b"},
		},
		{
			name: "trims whitespace",
			raw:  " entra/a , entra/b ",
			want: []string{"entra/a", "entra/b"},
		},
		{
			name: "drops blanks",
			raw:  "entra/a,,entra/b,",
			want: []string{"entra/a", "entra/b"},
		},
		{
			name: "drops duplicates",
			raw:  "entra/a,entra/b,entra/a",
			want: []string{"entra/a", "entra/b"},
		},
		{
			name: "all blank",
			raw:  ",,,",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitGroupIDs(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("position %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSplitGroupIDsCapsBatchSize(t *testing.T) {
	ids := make([]string, 0, maxGroupIDsPerRequest*2)
	for i := range maxGroupIDsPerRequest * 2 {
		ids = append(ids, fmt.Sprintf("entra/%d", i))
	}

	if got := splitGroupIDs(strings.Join(ids, ",")); len(got) != maxGroupIDsPerRequest {
		t.Errorf("len = %d, want %d", len(got), maxGroupIDsPerRequest)
	}
}

func TestTrimGroupsForUser(t *testing.T) {
	groups := []types.Group{{
		ID:                    "entra/a",
		Name:                  "Engineering",
		AuthProviderName:      "entra-auth-provider",
		AuthProviderNamespace: "default",
	}}

	t.Run("basic users only see id and name", func(t *testing.T) {
		got := trimGroupsForUser(&user.DefaultInfo{Groups: []string{"basic"}}, groups)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].ID != "entra/a" || got[0].Name != "Engineering" {
			t.Errorf("id/name = %q/%q, want entra/a/Engineering", got[0].ID, got[0].Name)
		}
		if got[0].AuthProviderName != "" || got[0].AuthProviderNamespace != "" {
			t.Errorf("auth provider fields leaked: %+v", got[0])
		}
	})

	t.Run("admins see everything", func(t *testing.T) {
		got := trimGroupsForUser(&user.DefaultInfo{Groups: []string{"admin"}}, groups)
		if got[0].AuthProviderName != "entra-auth-provider" {
			t.Errorf("AuthProviderName = %q, want entra-auth-provider", got[0].AuthProviderName)
		}
	})
}
