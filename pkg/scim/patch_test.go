package scim

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/scim/adapter"
)

func TestApplyPatchUser(t *testing.T) {
	entra := adapter.PatchRules{
		ReplaceAddsUnmatched: true,
	}
	tests := []struct {
		name         string
		ops          string
		rules        adapter.PatchRules
		want         map[string]any
		wantScimType string
	}{
		{
			name: "pathless deactivation, as the Okta catalog app sends it",
			ops:  `[{"op":"replace","value":{"active":false}}]`,
			want: map[string]any{
				"active": false,
			},
		},
		{
			name: "active as a string, as some clients send it",
			ops:  `[{"op":"Replace","path":"active","value":"False"}]`,
			want: map[string]any{
				"active": false,
			},
		},
		{
			name: "sub-attribute path",
			ops:  `[{"op":"replace","path":"name.familyName","value":"Updated"}]`,
			want: map[string]any{
				"name": map[string]any{
					"givenName":  "Given",
					"familyName": "Updated",
				},
			},
		},
		{
			name: "pathless dotted key",
			ops:  `[{"op":"replace","value":{"name.givenName":"New"}}]`,
			want: map[string]any{
				"name": map[string]any{
					"givenName":  "New",
					"familyName": "Family",
				},
			},
		},
		{
			name: "filtered sub-attribute replaces the matching email",
			ops:  `[{"op":"replace","path":"emails[type eq \"work\"].value","value":"new@example.com"}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "new@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name: "filtered sub-attribute adds an email when none matches",
			ops:  `[{"op":"add","path":"emails[type eq \"home\"].value","value":"home@example.com"}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
					map[string]any{
						"value": "home@example.com",
						"type":  "home",
					},
				},
			},
		},
		{
			name: "filtered primary sub-attribute makes its email the only primary one",
			ops: `[{"op":"add","path":"emails","value":[{"value":"home@example.com","type":"home"}]},` +
				`{"op":"replace","path":"emails[type eq \"home\"].primary","value":true}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value": "user@example.com",
						"type":  "work",
					},
					map[string]any{
						"value":   "home@example.com",
						"type":    "home",
						"primary": true,
					},
				},
			},
		},
		{
			name: "filtered replacement with a primary value makes it the only primary one",
			ops: `[{"op":"add","path":"emails","value":[{"value":"home@example.com","type":"home"}]},` +
				`{"op":"replace","path":"emails[type eq \"home\"]","value":{"value":"new@example.com","type":"home","primary":true}}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value": "user@example.com",
						"type":  "work",
					},
					map[string]any{
						"value":   "new@example.com",
						"type":    "home",
						"primary": true,
					},
				},
			},
		},
		{
			name: "adding the primary email again, twice, changes nothing",
			ops:  `[{"op":"add","path":"emails","value":[{"value":"user@example.com","type":"work","primary":true}]},{"op":"add","path":"emails","value":[{"value":"user@example.com","type":"work","primary":true}]}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name: "adding a primary email makes it the only primary one",
			ops:  `[{"op":"add","path":"emails","value":[{"value":"home@example.com","type":"home","primary":true}]}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value": "user@example.com",
						"type":  "work",
					},
					map[string]any{
						"value":   "home@example.com",
						"type":    "home",
						"primary": true,
					},
				},
			},
		},
		{
			name: "adding an existing email as primary marks it primary without adding it again",
			ops:  `[{"op":"add","path":"emails","value":[{"value":"home@example.com","type":"home"}]},{"op":"add","path":"emails","value":[{"value":"home@example.com","type":"home","primary":true}]}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value": "user@example.com",
						"type":  "work",
					},
					map[string]any{
						"value":   "home@example.com",
						"type":    "home",
						"primary": true,
					},
				},
			},
		},
		{
			name: "attribute names are case-insensitive",
			ops:  `[{"op":"replace","path":"DISPLAYNAME","value":"Renamed"}]`,
			want: map[string]any{
				"displayName": "Renamed",
			},
		},
		{
			name: "pathless read-only attributes that are unchanged are ignored, as clients echo the id",
			ops:  `[{"op":"replace","value":{"id":"u1","schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"nickName":"Nick"}}]`,
			want: map[string]any{
				"id":       "u1",
				"nickName": "Nick",
			},
		},
		{
			name:         "a pathless change to a read-only attribute is refused",
			ops:          `[{"op":"replace","value":{"id":"other","nickName":"Nick"}}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name:         "a pathless unknown attribute is refused",
			ops:          `[{"op":"replace","value":{"shoeSize":"10","nickName":"Nick"}}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name:         "a pathless extension attribute is refused",
			ops:          `[{"op":"replace","value":{"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":{"department":"x"}}}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name: "pathless members under the schema URN",
			ops:  `[{"op":"replace","value":{"urn:ietf:params:scim:schemas:core:2.0:User":{"nickName":"Nick","name.givenName":"New"}}}]`,
			want: map[string]any{
				"nickName": "Nick",
				"name": map[string]any{
					"givenName":  "New",
					"familyName": "Family",
				},
			},
		},
		{
			name: "a password is ignored, with or without a path",
			ops:  `[{"op":"replace","value":{"password":"secret","nickName":"Nick"}},{"op":"replace","path":"password","value":"secret"}]`,
			want: map[string]any{
				"nickName": "Nick",
				"password": nil,
			},
		},
		{
			name: "a path to a read-only attribute that restates it is ignored",
			ops:  `[{"op":"replace","path":"id","value":"u1"}]`,
			want: map[string]any{
				"id": "u1",
			},
		},
		{
			name:         "a path to a read-only attribute is refused",
			ops:          `[{"op":"replace","path":"id","value":"other"}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name:         "removing a read-only attribute is refused",
			ops:          `[{"op":"remove","path":"id"}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name: "adding an email again, with empty sub-attributes and in another case, changes nothing",
			ops:  `[{"op":"add","path":"emails","value":[{"value":"USER@example.com","type":"Work","display":"","primary":false}]}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name: "a listed email is removed by its value alone, in any case",
			ops:  `[{"op":"remove","path":"emails","value":[{"value":"USER@example.com"}]}]`,
			want: map[string]any{
				"emails": []any{},
			},
		},
		{
			name: "a listed email of another type removes nothing",
			ops:  `[{"op":"remove","path":"emails","value":[{"value":"user@example.com","type":"home"}]}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name: "a listed email without a value removes nothing",
			ops:  `[{"op":"remove","path":"emails","value":[{"type":"work"}]}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name:         "an unknown path is refused",
			ops:          `[{"op":"replace","path":"shoeSize","value":"10"}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name:         "an extension path is refused",
			ops:          `[{"op":"replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department","value":"x"}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name:         "remove without a path",
			ops:          `[{"op":"remove"}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name:         "replace with a filter that matches nothing",
			ops:          `[{"op":"replace","path":"emails[type eq \"home\"]","value":{"value":"x"}}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name:         "add of a sub-attribute that the filter selecting its value would no longer select",
			ops:          `[{"op":"add","path":"emails[value eq \"home@example.com\"].value","value":"other@example.com"}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name:         "replace of a sub-attribute with a filter that matches nothing",
			ops:          `[{"op":"replace","path":"emails[type eq \"home\"].value","value":"home@example.com"}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name:  "replace with a filter that matches nothing adds the value under rules that allow it",
			ops:   `[{"op":"replace","path":"emails[type eq \"home\"]","value":{"value":"home@example.com"}}]`,
			rules: entra,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
					map[string]any{
						"value": "home@example.com",
						"type":  "home",
					},
				},
			},
		},
		{
			name:  "replace of a sub-attribute with a filter that matches nothing adds the value under rules that allow it, as Entra sends it",
			ops:   `[{"op":"replace","path":"emails[type eq \"home\"].value","value":"home@example.com"}]`,
			rules: entra,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
					map[string]any{
						"value": "home@example.com",
						"type":  "home",
					},
				},
			},
		},
		{
			name:  "pathless replace with a filter that matches nothing adds the value under rules that allow it, as Entra sends it",
			ops:   `[{"op":"replace","value":{"emails[type eq \"home\"].value":"home@example.com"}}]`,
			rules: entra,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
					map[string]any{
						"value": "home@example.com",
						"type":  "home",
					},
				},
			},
		},
		{
			name: "filtered add that matches nothing adds the value with the filter's sub-attributes",
			ops:  `[{"op":"add","path":"emails[type eq \"home\"]","value":{"value":"home@example.com"}}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
					map[string]any{
						"value": "home@example.com",
						"type":  "home",
					},
				},
			},
		},
		{
			name:         "filtered add that matches nothing, of a value the filter would not select",
			ops:          `[{"op":"add","path":"emails[type eq \"home\"]","value":{"value":"home@example.com","type":"work"}}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name:         "wrong value type",
			ops:          `[{"op":"replace","path":"active","value":"maybe"}]`,
			wantScimType: scimTypeInvalidValue,
		},
		{
			name:         "invalid filter in path",
			ops:          `[{"op":"remove","path":"emails[type eq]"}]`,
			wantScimType: scimTypeInvalidFilter,
		},
		{
			name:         "removing a required attribute is refused",
			ops:          `[{"op":"remove","path":"userName"}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name:         "removing active is refused",
			ops:          `[{"op":"remove","path":"active"}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name:         "removing active with a pathless null is refused",
			ops:          `[{"op":"replace","value":{"active":null}}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name: "removing attributes that are not required",
			ops:  `[{"op":"replace","path":"nickName","value":"Nick"},{"op":"remove","path":"nickName"},{"op":"replace","value":{"name":null}}]`,
			want: map[string]any{
				"nickName": nil,
				"name":     nil,
			},
		},
		{
			name: "a null value removes what it is assigned to, with or without a path",
			ops: `[{"op":"replace","path":"title","value":"Engineer"},{"op":"replace","path":"title","value":null},` +
				`{"op":"add","path":"nickName","value":"Nick"},{"op":"add","path":"nickName","value":null},` +
				`{"op":"replace","path":"name.givenName","value":null},` +
				`{"op":"replace","value":{"emails[type eq \"work\"]":null}}]`,
			want: map[string]any{
				"title":    nil,
				"nickName": nil,
				"name": map[string]any{
					"familyName": "Family",
				},
				"emails": []any{},
			},
		},
		{
			name: "adding null to a multi-valued attribute adds nothing, with or without a filter or a path",
			ops: `[{"op":"add","path":"emails","value":null},` +
				`{"op":"add","path":"emails[type eq \"work\"]","value":null},` +
				`{"op":"add","value":{"emails":null}}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name:         "replacing values that a filter selects with null needs a value it selects",
			ops:          `[{"op":"replace","path":"emails[type eq \"home\"]","value":null}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name:         "replacing a sub-attribute of values that a filter selects with null needs a value it selects",
			ops:          `[{"op":"replace","path":"emails[type eq \"home\"].value","value":null}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name: "replacing values that a filter selects with null removes them",
			ops: `[{"op":"add","path":"emails","value":[{"value":"home@example.com","type":"home"}]},` +
				`{"op":"replace","path":"emails[type eq \"home\"]","value":null}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name:  "replacing values that a filter selects with null changes nothing when it selects none and the client's rules add unmatched values",
			ops:   `[{"op":"replace","path":"emails[type eq \"home\"]","value":null}]`,
			rules: entra,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value":   "user@example.com",
						"type":    "work",
						"primary": true,
					},
				},
			},
		},
		{
			name:         "replacing an unknown sub-attribute of values that a filter selects with null is an invalid path",
			ops:          `[{"op":"replace","path":"emails[type eq \"home\"].bogus","value":null}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name: "adding null to a read-only multi-valued attribute adds nothing",
			ops:  `[{"op":"add","path":"groups","value":null}]`,
			want: map[string]any{
				"groups": []any{},
			},
		},
		{
			name: "adding null to a sub-attribute of a filtered value removes it",
			ops:  `[{"op":"add","path":"emails[type eq \"work\"].primary","value":null}]`,
			want: map[string]any{
				"emails": []any{
					map[string]any{
						"value": "user@example.com",
						"type":  "work",
					},
				},
			},
		},
		{
			name:         "removing active with a null value is refused",
			ops:          `[{"op":"replace","path":"active","value":null}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name:         "a replace without a value is refused",
			ops:          `[{"op":"replace","path":"title"}]`,
			wantScimType: scimTypeInvalidValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := testUserResource()
			err := applyPatch(userResourceSchema, resource, decodeTestOperations(t, tt.ops), tt.rules)
			if tt.wantScimType != "" {
				var scimErr *Error
				if !errors.As(err, &scimErr) || scimErr.ScimType != tt.wantScimType {
					t.Fatalf("applyPatch() error = %v, want scimType %q", err, tt.wantScimType)
				}
				return
			}
			if err != nil {
				t.Fatalf("applyPatch() error = %v", err)
			}

			for key, want := range tt.want {
				if got := resource[key]; !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %#v, want %#v", key, got, want)
				}
			}
		})
	}
}

func TestApplyPatchGroupMembers(t *testing.T) {
	tests := []struct {
		name        string
		ops         string
		wantName    string
		wantMembers []string
		// wantSCIMType is the scimType of the error the operations must fail with, if any.
		wantSCIMType string
	}{
		{
			name:        "pathless rename that echoes the id, as Okta sends it",
			ops:         `[{"op":"replace","value":{"displayName":"renamed","id":"g1"}}]`,
			wantName:    "renamed",
			wantMembers: []string{"u1", "u2"},
		},
		{
			name:        "add is idempotent",
			ops:         `[{"op":"add","path":"members","value":[{"value":"u1"},{"value":"u2"},{"value":"u3","display":"u3@example.com"}]}]`,
			wantName:    "group",
			wantMembers: []string{"u1", "u2", "u3"},
		},
		{
			name:        "filtered remove",
			ops:         `[{"op":"remove","path":"members[value eq \"u1\"]"}]`,
			wantName:    "group",
			wantMembers: []string{"u2"},
		},
		{
			name:        "filtered remove of an absent member changes nothing",
			ops:         `[{"op":"remove","path":"members[value eq \"u9\"]"}]`,
			wantName:    "group",
			wantMembers: []string{"u1", "u2"},
		},
		{
			name:        "adding null members changes nothing",
			ops:         `[{"op":"add","path":"members","value":null}]`,
			wantName:    "group",
			wantMembers: []string{"u1", "u2"},
		},
		{
			name:        "replacing the members with null removes them all",
			ops:         `[{"op":"replace","path":"members","value":null}]`,
			wantName:    "group",
			wantMembers: []string{},
		},
		{
			name:        "remove all",
			ops:         `[{"op":"remove","path":"members"}]`,
			wantName:    "group",
			wantMembers: []string{},
		},
		{
			name:        "remove listed values",
			ops:         `[{"op":"remove","path":"members","value":[{"value":"u2"}]}]`,
			wantName:    "group",
			wantMembers: []string{"u1"},
		},
		{
			name:        "listed values match in any case, as a filter does",
			ops:         `[{"op":"remove","path":"members","value":[{"value":"U2"}]},{"op":"add","path":"members","value":[{"value":"U1"}]}]`,
			wantName:    "group",
			wantMembers: []string{"u1"},
		},
		{
			name:         "removing the display name is refused",
			ops:          `[{"op":"remove","path":"displayName"}]`,
			wantSCIMType: scimTypeMutability,
		},
		{
			name:         "a pathless rename that changes the id is refused",
			ops:          `[{"op":"replace","value":{"displayName":"renamed","id":"g2"}}]`,
			wantSCIMType: scimTypeMutability,
		},
		{
			name:        "replace is the complete set",
			ops:         `[{"op":"replace","path":"members","value":[{"value":"u3"}]}]`,
			wantName:    "group",
			wantMembers: []string{"u3"},
		},
		{
			name:        "replace with an empty list clears the members",
			ops:         `[{"op":"replace","path":"members","value":[]}]`,
			wantName:    "group",
			wantMembers: []string{},
		},
		{
			name:        "filtered add merges into the selected member",
			ops:         `[{"op":"add","path":"members[value eq \"u1\"]","value":{"display":"one"}}]`,
			wantName:    "group",
			wantMembers: []string{"u1", "u2"},
		},
		{
			name:        "filtered add that matches nothing adds the member the filter selects",
			ops:         `[{"op":"add","path":"members[value eq \"u3\"]","value":{"display":"three"}}]`,
			wantName:    "group",
			wantMembers: []string{"u1", "u2", "u3"},
		},
		{
			name:         "filtered add that matches nothing, of a member the filter would not select",
			ops:          `[{"op":"add","path":"members[value eq \"u9\"]","value":{"value":"u3"}}]`,
			wantSCIMType: scimTypeNoTarget,
		},
		{
			name:         "filtered replace cannot change a member's value",
			ops:          `[{"op":"replace","path":"members[value eq \"u1\"]","value":{"value":"u3"}}]`,
			wantSCIMType: scimTypeMutability,
		},
		{
			name:         "filtered replace cannot change a member's value sub-attribute",
			ops:          `[{"op":"replace","path":"members[value eq \"u1\"].value","value":"u2"}]`,
			wantSCIMType: scimTypeMutability,
		},
		{
			name:         "filtered replace cannot drop a member's immutable sub-attributes",
			ops:          `[{"op":"replace","path":"members[value eq \"u1\"]","value":{"value":"u1"}}]`,
			wantSCIMType: scimTypeMutability,
		},
		{
			name:         "filtered remove cannot remove a member's value sub-attribute",
			ops:          `[{"op":"remove","path":"members[value eq \"u1\"].value"}]`,
			wantSCIMType: scimTypeMutability,
		},
		{
			name:        "restating a member's value changes nothing",
			ops:         `[{"op":"add","path":"members[value eq \"u1\"]","value":{"value":"u1"}},{"op":"replace","path":"members[value eq \"u2\"].value","value":"u2"}]`,
			wantName:    "group",
			wantMembers: []string{"u1", "u2"},
		},
		{
			name:        "operations apply in order",
			ops:         `[{"op":"replace","value":{"displayName":"group"}},{"op":"add","path":"members","value":[{"value":"u3"}]},{"op":"remove","path":"members[value eq \"u1\"]"}]`,
			wantName:    "group",
			wantMembers: []string{"u2", "u3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := map[string]any{
				"schemas":     []any{groupSchema},
				"id":          "g1",
				"displayName": "group",
				"members": []any{
					map[string]any{
						"value": "u1",
						"$ref":  "https://obot.example.com/scim/v2/Users/u1",
						"type":  "User",
					},
					map[string]any{
						"value": "u2",
						"$ref":  "https://obot.example.com/scim/v2/Users/u2",
						"type":  "User",
					},
				},
			}
			err := applyPatch(groupResourceSchema, resource, decodeTestOperations(t, tt.ops), adapter.PatchRules{})
			if tt.wantSCIMType != "" {
				if e, ok := err.(*Error); !ok || e.ScimType != tt.wantSCIMType {
					t.Fatalf("applyPatch() error = %v, want scimType %s", err, tt.wantSCIMType)
				}
				return
			}
			if err != nil {
				t.Fatalf("applyPatch() error = %v", err)
			}

			input, err := groupInputFromResource(resource)
			if err != nil {
				t.Fatalf("groupInputFromResource() error = %v", err)
			}
			members := input.MemberIDs
			if members == nil {
				members = []string{}
			}
			if input.DisplayName != tt.wantName || !reflect.DeepEqual(members, tt.wantMembers) {
				t.Fatalf("got %q %v, want %q %v", input.DisplayName, members, tt.wantName, tt.wantMembers)
			}
		})
	}
}

// TestApplyPatchPathlessOrder applies members of a pathless value that change the same attribute, which a client can
// write in any order, and expects each attribute to apply before the members that select a part of it.
func TestApplyPatchPathlessOrder(t *testing.T) {
	ops := decodeTestOperations(t, `[{"op":"replace","value":{`+
		`"emails[type eq \"work\"].value":"new@example.com",`+
		`"emails":[{"value":"other@example.com","type":"work"}],`+
		`"name.givenName":"New",`+
		`"name":{"givenName":"Old","familyName":"Replaced"}}}]`)
	want := testUserResource()
	want["emails"] = []any{
		map[string]any{
			"value": "new@example.com",
			"type":  "work",
		},
	}
	want["name"] = map[string]any{
		"givenName":  "New",
		"familyName": "Replaced",
	}

	// Go randomizes map iteration, so each run sees the members in a different order.
	for range 100 {
		resource := testUserResource()
		if err := applyPatch(userResourceSchema, resource, ops, adapter.PatchRules{}); err != nil {
			t.Fatalf("applyPatch() error = %v", err)
		}
		if !reflect.DeepEqual(resource, want) {
			t.Fatalf("resource = %#v, want %#v", resource, want)
		}
	}
}

func TestDecodePatchOperations(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantOps      int
		wantScimType string
	}{
		{
			name:    "Okta request",
			body:    `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","value":{"active":false}}]}`,
			wantOps: 1,
		},
		{
			name:    "member names are case-insensitive",
			body:    `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"operations":[{"OP":"Add","PATH":"members","VALUE":[]}]}`,
			wantOps: 1,
		},
		{
			name:         "wrong message schema",
			body:         `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"Operations":[{"op":"add","path":"x","value":1}]}`,
			wantScimType: scimTypeInvalidSyntax,
		},
		{
			name:         "no operations",
			body:         `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[]}`,
			wantScimType: scimTypeInvalidSyntax,
		},
		{
			name:         "too many operations",
			body:         `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[` + strings.TrimSuffix(strings.Repeat(`{"op":"add","path":"nickName","value":"x"},`, maxPatchOperations+1), ",") + `]}`,
			wantScimType: scimTypeInvalidSyntax,
		},
		{
			name:         "unknown operation",
			body:         `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"move","path":"x"}]}`,
			wantScimType: scimTypeInvalidSyntax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			decoder := json.NewDecoder(strings.NewReader(tt.body))
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil {
				t.Fatal(err)
			}

			ops, err := decodePatchOperations(body)
			if tt.wantScimType != "" {
				var scimErr *Error
				if !errors.As(err, &scimErr) || scimErr.ScimType != tt.wantScimType {
					t.Fatalf("decodePatchOperations() error = %v, want scimType %q", err, tt.wantScimType)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodePatchOperations() error = %v", err)
			}
			if len(ops) != tt.wantOps {
				t.Fatalf("got %d operations, want %d", len(ops), tt.wantOps)
			}
		})
	}
}

func testUserResource() map[string]any {
	return map[string]any{
		"schemas":  []any{userSchema},
		"id":       "u1",
		"userName": "user@example.com",
		"active":   true,
		"name": map[string]any{
			"givenName":  "Given",
			"familyName": "Family",
		},
		"emails": []any{
			map[string]any{
				"value":   "user@example.com",
				"type":    "work",
				"primary": true,
			},
		},
		"groups": []any{},
	}
}

func decodeTestOperations(t *testing.T, ops string) []patchOperation {
	t.Helper()

	var raw []any
	decoder := json.NewDecoder(strings.NewReader(ops))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		t.Fatalf("failed to decode operations: %v", err)
	}
	decoded, err := decodePatchOperations(map[string]any{
		"schemas":    []any{patchOpSchema},
		"Operations": raw,
	})
	if err != nil {
		t.Fatalf("failed to decode operations: %v", err)
	}
	return decoded
}
