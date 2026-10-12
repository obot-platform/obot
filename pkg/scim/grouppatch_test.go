package scim

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/scim/adapter"
)

func TestPlanGroupPatch(t *testing.T) {
	tests := []struct {
		name   string
		ops    string
		want   gclient.SCIMGroupPatch
		wantOK bool
	}{
		{
			name: "members added, as Okta sends them",
			ops:  `[{"op":"add","path":"members","value":[{"value":"a","display":"A"},{"value":"b","display":"B"}]}]`,
			want: gclient.SCIMGroupPatch{
				MemberIDs: []string{"a", "b"},
			},
			wantOK: true,
		},
		{
			name: "a member removed by a filter, as Okta sends it",
			ops:  `[{"op":"remove","path":"members[value eq \"A\"]"}]`,
			want: gclient.SCIMGroupPatch{
				RemovedMemberIDs: []string{"a"},
			},
			wantOK: true,
		},
		{
			name: "members replaced, as Okta sends them",
			ops:  `[{"op":"replace","path":"members","value":[{"value":"a","display":"A"}]}]`,
			want: gclient.SCIMGroupPatch{
				ReplaceMembers: true,
				MemberIDs:      []string{"a"},
			},
			wantOK: true,
		},
		{
			name: "every member removed by an empty replacement",
			ops:  `[{"op":"replace","path":"members","value":[]}]`,
			want: gclient.SCIMGroupPatch{
				ReplaceMembers: true,
				MemberIDs:      []string{},
			},
			wantOK: true,
		},
		{
			name: "a rename without a path that echoes the id, as Okta sends it",
			ops:  `[{"op":"replace","value":{"id":"g1","displayName":"Renamed"}}]`,
			want: gclient.SCIMGroupPatch{
				DisplayName: "Renamed",
			},
			wantOK: true,
		},
		{
			name: "a rename without a path that changes the id",
			ops:  `[{"op":"replace","value":{"id":"other","displayName":"Renamed"}}]`,
		},
		{
			name: "a rename with a path",
			ops:  `[{"op":"Replace","path":"DisplayName","value":"Renamed"}]`,
			want: gclient.SCIMGroupPatch{
				DisplayName: "Renamed",
			},
			wantOK: true,
		},
		{
			name: "a path with the schema URN",
			ops:  `[{"op":"add","path":"urn:ietf:params:scim:schemas:core:2.0:Group:members","value":[{"value":"a"}]}]`,
			want: gclient.SCIMGroupPatch{
				MemberIDs: []string{"a"},
			},
			wantOK: true,
		},
		{
			name: "the last operation that names a member decides",
			ops: `[{"op":"add","path":"members","value":[{"value":"a"},{"value":"b"}]},` +
				`{"op":"remove","path":"members[value eq \"A\"]"},` +
				`{"op":"remove","path":"members[value eq \"c\"]"},` +
				`{"op":"add","path":"members","value":[{"value":"c"}]}]`,
			want: gclient.SCIMGroupPatch{
				MemberIDs:        []string{"b", "c"},
				RemovedMemberIDs: []string{"a"},
			},
			wantOK: true,
		},
		{
			name: "operations after a replacement apply to its member set",
			ops: `[{"op":"add","path":"members","value":[{"value":"x"}]},` +
				`{"op":"replace","path":"members","value":[{"value":"a"},{"value":"b"}]},` +
				`{"op":"remove","path":"members[value eq \"a\"]"},` +
				`{"op":"add","path":"members","value":[{"value":"b"},{"value":"c"}]}]`,
			want: gclient.SCIMGroupPatch{
				ReplaceMembers: true,
				MemberIDs:      []string{"b", "c"},
			},
			wantOK: true,
		},
		{
			name: "a member of another type",
			ops:  `[{"op":"add","path":"members","value":[{"value":"a","type":"Group"}]}]`,
		},
		{
			name: "a member without a value",
			ops:  `[{"op":"add","path":"members","value":[{"display":"A"}]}]`,
		},
		{
			name: "every member removed without a filter",
			ops:  `[{"op":"remove","path":"members"}]`,
		},
		{
			name: "a filter on another sub-attribute",
			ops:  `[{"op":"remove","path":"members[display eq \"A\"]"}]`,
		},
		{
			name: "a filter that is not a single equality",
			ops:  `[{"op":"remove","path":"members[value eq \"a\" or value eq \"b\"]"}]`,
		},
		{
			name: "members added without a path",
			ops:  `[{"op":"add","value":{"members":[{"value":"a"}]}}]`,
		},
		{
			name: "a pathless replacement of another attribute",
			ops:  `[{"op":"replace","value":{"displayName":"Renamed","externalId":"x"}}]`,
		},
		{
			name: "an empty name",
			ops:  `[{"op":"replace","path":"displayName","value":" "}]`,
		},
		{
			name: "a name that is not a string",
			ops:  `[{"op":"replace","value":{"displayName":7}}]`,
		},
		{
			name: "an add of a name",
			ops:  `[{"op":"add","path":"displayName","value":"Renamed"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := planGroupPatch("g1", decodeTestOperations(t, tt.ops))
			if ok != tt.wantOK {
				t.Fatalf("planGroupPatch() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("planGroupPatch() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestGroupPatchMatchesTheWholeGroupPatch applies each change to two groups with the same members, one as a plan
// and one with applyPatch against the whole group, and expects the same members afterwards.
func TestGroupPatchMatchesTheWholeGroupPatch(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	ids := map[string]string{}
	for _, name := range []string{"alice", "bob", "carol", "dave"} {
		ids[name] = s.do(http.MethodPost, "Users", scimUser(name+"@example.com", "00u-"+name)).expect(t, http.StatusCreated).id()
	}
	ops := func(template string) string {
		for name, id := range ids {
			template = strings.ReplaceAll(template, "{"+name+"}", id)
			template = strings.ReplaceAll(template, "{"+strings.ToUpper(name)+"}", strings.ToUpper(id))
		}
		return template
	}

	tests := []struct {
		name    string
		ops     string
		wantErr bool
	}{
		{
			name: "add a member",
			ops:  `[{"op":"add","path":"members","value":[{"value":"{carol}","display":"carol"}]}]`,
		},
		{
			name: "add a member again",
			ops:  `[{"op":"add","path":"members","value":[{"value":"{alice}"}]}]`,
		},
		{
			name: "remove a member",
			ops:  `[{"op":"remove","path":"members[value eq \"{bob}\"]"}]`,
		},
		{
			name: "remove a member by its uppercased value",
			ops:  `[{"op":"remove","path":"members[value eq \"{BOB}\"]"}]`,
		},
		{
			name: "add a member by its uppercased value",
			ops:  `[{"op":"add","path":"members","value":[{"value":"{CAROL}"}]}]`,
		},
		{
			name: "replace the members by their uppercased values",
			ops:  `[{"op":"replace","path":"members","value":[{"value":"{BOB}"},{"value":"{carol}"}]}]`,
		},
		{
			name: "remove a member that is not a user",
			ops:  `[{"op":"remove","path":"members[value eq \"00000000-0000-0000-0000-000000000000\"]"}]`,
		},
		{
			name: "remove a user that is not a member",
			ops:  `[{"op":"remove","path":"members[value eq \"{dave}\"]"}]`,
		},
		{
			name: "add and remove members",
			ops: `[{"op":"add","path":"members","value":[{"value":"{carol}"},{"value":"{dave}"}]},` +
				`{"op":"remove","path":"members[value eq \"{alice}\"]"},` +
				`{"op":"remove","path":"members[value eq \"{dave}\"]"}]`,
		},
		{
			name: "remove a member and add it back",
			ops: `[{"op":"remove","path":"members[value eq \"{alice}\"]"},` +
				`{"op":"add","path":"members","value":[{"value":"{alice}"}]}]`,
		},
		{
			name: "replace the members",
			ops:  `[{"op":"replace","path":"members","value":[{"value":"{bob}"},{"value":"{carol}"}]}]`,
		},
		{
			name: "remove every member",
			ops:  `[{"op":"replace","path":"members","value":[]}]`,
		},
		{
			name: "change the replaced members",
			ops: `[{"op":"replace","path":"members","value":[{"value":"{carol}"},{"value":"{alice}"}]},` +
				`{"op":"add","path":"members","value":[{"value":"{dave}"},{"value":"{alice}"}]},` +
				`{"op":"remove","path":"members[value eq \"{carol}\"]"}]`,
		},
		{
			name:    "add a member that is not a user",
			ops:     `[{"op":"add","path":"members","value":[{"value":"00000000-0000-0000-0000-000000000000"}]}]`,
			wantErr: true,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded := decodeTestOperations(t, ops(tt.ops))
			if _, ok := planGroupPatch("", decoded); !ok {
				t.Fatal("the change does not apply as a plan")
			}
			var operations []any
			if err := json.Unmarshal([]byte(ops(tt.ops)), &operations); err != nil {
				t.Fatal(err)
			}

			planned := s.do(http.MethodPost, "Groups", scimGroup(fmt.Sprintf("planned %d", i), ids["alice"], ids["bob"])).expect(t, http.StatusCreated)
			whole := s.do(http.MethodPost, "Groups", scimGroup(fmt.Sprintf("whole %d", i), ids["alice"], ids["bob"])).expect(t, http.StatusCreated)

			resp := s.do(http.MethodPatch, "Groups/"+planned.id(), map[string]any{
				"schemas":    []string{patchOpSchema},
				"Operations": operations,
			})
			err := s.gateway.UpdateSCIMGroup(t.Context(), s.conn, whole.id(), func(current gclient.SCIMGroup) (gclient.SCIMGroupInput, error) {
				resource := groupResource(&current, testServerURL)
				if err := applyPatch(groupResourceSchema, resource, decoded, adapter.PatchRules{}); err != nil {
					return gclient.SCIMGroupInput{}, err
				}
				return groupInputFromResource(resource)
			})

			if tt.wantErr {
				resp.expect(t, http.StatusBadRequest)
				if err == nil {
					t.Fatal("the whole group patch succeeded")
				}
			} else {
				resp.expect(t, http.StatusNoContent)
				if err != nil {
					t.Fatalf("the whole group patch failed: %v", err)
				}
			}
			if got, want := s.memberships("okta/"+planned.id()), s.memberships("okta/"+whole.id()); !slices.Equal(got, want) {
				t.Fatalf("members = %v, want %v", got, want)
			}
		})
	}
}

func TestGroupPatchChangesOnlyTheMembersItNames(t *testing.T) {
	s := newSCIMTest(t)
	alice := s.seedUser("00u-alice", "alice@example.com", types2.RoleBasic)
	bob := s.seedUser("00u-bob", "bob@example.com", types2.RoleBasic)
	carol := s.seedUser("00u-carol", "carol@example.com", types2.RoleBasic)
	s.enable()

	aliceID := s.do(http.MethodPost, "Users", scimUser("alice@example.com", "00u-alice")).expect(t, http.StatusCreated).id()
	bobID := s.do(http.MethodPost, "Users", scimUser("bob@example.com", "00u-bob")).expect(t, http.StatusCreated).id()
	carolID := s.do(http.MethodPost, "Users", scimUser("carol@example.com", "00u-carol")).expect(t, http.StatusCreated).id()
	group := s.do(http.MethodPost, "Groups", scimGroup("Team", aliceID, bobID)).expect(t, http.StatusCreated)

	revision := func() int64 {
		t.Helper()
		var binding types.SCIMGroupBinding
		if err := s.gorm().Where("id = ?", group.id()).Take(&binding).Error; err != nil {
			t.Fatal(err)
		}
		return binding.Revision
	}
	eventCounts := func() map[uint]int {
		return map[uint]int{
			alice.ID: len(s.events(alice.ID)),
			bob.ID:   len(s.events(bob.ID)),
			carol.ID: len(s.events(carol.ID)),
		}
	}
	change := patchOp(map[string]any{
		"op":   "add",
		"path": "members",
		"value": []any{
			map[string]any{
				"value": carolID,
			},
		},
	}, map[string]any{
		"op":   "remove",
		"path": `members[value eq "` + bobID + `"]`,
	})

	before, beforeRevision := eventCounts(), revision()
	s.do(http.MethodPatch, "Groups/"+group.id(), change).expect(t, http.StatusNoContent)
	if got := s.memberships("okta/" + group.id()); !slices.Equal(got, []uint{alice.ID, carol.ID}) {
		t.Fatalf("memberships = %v", got)
	}
	after := eventCounts()
	if after[alice.ID] != before[alice.ID] || after[bob.ID] != before[bob.ID]+1 || after[carol.ID] != before[carol.ID]+1 {
		t.Fatalf("events before %v, after %v; want one each for the removed and the added member", before, after)
	}
	if !s.events(bob.ID)[after[bob.ID]-1].GroupsRemoved || s.events(carol.ID)[after[carol.ID]-1].GroupsRemoved {
		t.Fatal("the events do not record who left the group")
	}
	if got := revision(); got != beforeRevision+1 {
		t.Fatalf("revision = %d, want %d", got, beforeRevision+1)
	}

	// Repeating the change changes nothing and emits nothing.
	s.do(http.MethodPatch, "Groups/"+group.id(), change).expect(t, http.StatusNoContent)
	if got := eventCounts(); !maps.Equal(got, after) || revision() != beforeRevision+1 {
		t.Fatalf("a repeated change had an effect: events %v, revision %d", got, revision())
	}
}
