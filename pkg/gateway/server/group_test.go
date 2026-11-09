package server

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestParseGroupListParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantCursor string
	}{
		{
			name:      "defaults when absent",
			query:     "",
			wantLimit: client.DefaultGroupPageSize,
		},
		{
			name:      "explicit limit",
			query:     "limit=25",
			wantLimit: 25,
		},
		{
			name:      "limit capped",
			query:     "limit=100000",
			wantLimit: client.MaxGroupPageSize,
		},
		{
			name:      "zero limit falls back to default",
			query:     "limit=0",
			wantLimit: client.DefaultGroupPageSize,
		},
		{
			name:      "negative limit falls back to default",
			query:     "limit=-1",
			wantLimit: client.DefaultGroupPageSize,
		},
		{
			name:      "unparseable limit falls back to default",
			query:     "limit=many",
			wantLimit: client.DefaultGroupPageSize,
		},
		{
			name:       "cursor is carried through opaquely",
			query:      "limit=10&cursor=eyJ2IjoxfQ",
			wantLimit:  10,
			wantCursor: "eyJ2IjoxfQ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("bad test query: %v", err)
			}

			limit, cursor := parseGroupListParams(query)
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
			if cursor != tt.wantCursor {
				t.Errorf("cursor = %q, want %q", cursor, tt.wantCursor)
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
			// Deduplication keeps the first occurrence, so the result follows input
			// order rather than sorted order.
			name: "keeps input order when deduplicating",
			raw:  "entra/b,entra/a,entra/b",
			want: []string{"entra/b", "entra/a"},
		},
		{
			name: "all blank",
			raw:  ",,,",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitGroupIDs(tt.raw)
			if err != nil {
				t.Fatalf("splitGroupIDs() error = %v", err)
			}
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

func TestGroupIDsFromQuery(t *testing.T) {
	tests := []struct {
		name   string
		query  url.Values
		want   []string
		wantOK bool
	}{
		{
			name:   "neither parameter",
			query:  url.Values{"name": {"eng"}},
			wantOK: false,
		},
		{
			name: "repeated id keeps commas inside an ID",
			query: url.Values{"id": {
				"pingfederate/cn=Admins,ou=Groups,dc=example,dc=com",
				"pingfederate/cn=Devs,ou=Groups,dc=example,dc=com",
			}},
			want: []string{
				"pingfederate/cn=Admins,ou=Groups,dc=example,dc=com",
				"pingfederate/cn=Devs,ou=Groups,dc=example,dc=com",
			},
			wantOK: true,
		},
		{
			name:   "repeated id trims and drops blanks and duplicates",
			query:  url.Values{"id": {" entra/a ", "", "entra/b", "entra/a"}},
			want:   []string{"entra/a", "entra/b"},
			wantOK: true,
		},
		{
			name:   "ids is split at commas",
			query:  url.Values{"ids": {"entra/a,entra/b"}},
			want:   []string{"entra/a", "entra/b"},
			wantOK: true,
		},
		{
			name:   "blank id values count as absent",
			query:  url.Values{"id": {"", " "}, "ids": {"entra/a,entra/b"}},
			want:   []string{"entra/a", "entra/b"},
			wantOK: true,
		},
		{
			name:   "only blank id values and no ids",
			query:  url.Values{"id": {""}},
			wantOK: false,
		},
		{
			name:   "id is preferred over ids",
			query:  url.Values{"id": {"entra/a,b"}, "ids": {"entra/a,b"}},
			want:   []string{"entra/a,b"},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := groupIDsFromQuery(tt.query)
			if err != nil {
				t.Fatalf("groupIDsFromQuery() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("ids = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupIDsFromQueryRejectsAnOversizedBatch(t *testing.T) {
	query := url.Values{}
	for i := range maxGroupIDsPerRequest + 1 {
		query.Add("id", fmt.Sprintf("entra/%d", i))
	}
	if _, _, err := groupIDsFromQuery(query); err == nil {
		t.Error("expected an error for a batch over the limit")
	}
}

func TestSplitGroupIDsRejectsAnOversizedBatch(t *testing.T) {
	ids := make([]string, 0, maxGroupIDsPerRequest+1)
	for i := range maxGroupIDsPerRequest + 1 {
		ids = append(ids, fmt.Sprintf("entra/%d", i))
	}

	// Silently answering for the first N would leave the caller rendering raw IDs for the rest with
	// nothing to say why, so the caller has to chunk instead.
	if _, err := splitGroupIDs(strings.Join(ids, ",")); err == nil {
		t.Error("expected an error for a batch over the limit")
	}
}

func TestSplitGroupIDsAcceptsAFullBatch(t *testing.T) {
	ids := make([]string, 0, maxGroupIDsPerRequest)
	for i := range maxGroupIDsPerRequest {
		ids = append(ids, fmt.Sprintf("entra/%d", i))
	}

	got, err := splitGroupIDs(strings.Join(ids, ","))
	if err != nil {
		t.Fatalf("splitGroupIDs() error = %v", err)
	}
	if len(got) != maxGroupIDsPerRequest {
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
		got := trimGroupsForUser(&user.DefaultInfo{Groups: []string{types2.GroupBasic}}, groups)
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
		got := trimGroupsForUser(&user.DefaultInfo{Groups: []string{types2.GroupAdmin}}, groups)
		if got[0].AuthProviderName != "entra-auth-provider" {
			t.Errorf("AuthProviderName = %q, want entra-auth-provider", got[0].AuthProviderName)
		}
	})

	// userIsBasicOrPower counts power-user-plus as privileged, so it sees the untrimmed group.
	t.Run("power users plus see everything", func(t *testing.T) {
		got := trimGroupsForUser(&user.DefaultInfo{Groups: []string{types2.GroupPowerUserPlus}}, groups)
		if got[0].AuthProviderName != "entra-auth-provider" {
			t.Errorf("AuthProviderName = %q, want entra-auth-provider", got[0].AuthProviderName)
		}
	})
}
