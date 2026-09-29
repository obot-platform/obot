package scim

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestApplyPatchUser(t *testing.T) {
	tests := []struct {
		name         string
		ops          string
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
			name: "active with a path",
			ops:  `[{"op":"replace","path":"active","value":false}]`,
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
			name: "pathless read-only and unknown attributes are ignored",
			ops:  `[{"op":"replace","value":{"id":"other","groups":[],"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":{"department":"x"},"nickName":"Nick"}}]`,
			want: map[string]any{
				"nickName": "Nick",
			},
		},
		{
			name:         "a path to a read-only attribute is refused",
			ops:          `[{"op":"replace","path":"id","value":"other"}]`,
			wantScimType: scimTypeMutability,
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
			name:         "wrong value type",
			ops:          `[{"op":"replace","path":"active","value":"maybe"}]`,
			wantScimType: scimTypeInvalidValue,
		},
		{
			name:         "invalid filter in path",
			ops:          `[{"op":"remove","path":"emails[type eq]"}]`,
			wantScimType: scimTypeInvalidPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := testUserResource()
			err := applyPatch(userResourceSchema, resource, decodeTestOperations(t, tt.ops))
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
						"$ref":  "https://obot.example.com/scim/v2/c/Users/u1",
						"type":  "User",
					},
					map[string]any{
						"value": "u2",
						"$ref":  "https://obot.example.com/scim/v2/c/Users/u2",
						"type":  "User",
					},
				},
			}
			err := applyPatch(groupResourceSchema, resource, decodeTestOperations(t, tt.ops))
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
