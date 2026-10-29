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
			name: "pathless schemas that name the enterprise extension before its attributes apply are ignored",
			ops:  `[{"op":"replace","value":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User","urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"],"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":{"department":"Sales"}}}]`,
			want: map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Sales",
				},
			},
		},
		{
			name: "schemas in another order and case are ignored",
			ops:  `[{"op":"replace","path":"schemas","value":["URN:IETF:PARAMS:SCIM:SCHEMAS:EXTENSION:ENTERPRISE:2.0:USER","urn:ietf:params:scim:schemas:core:2.0:User"]}]`,
			want: map[string]any{
				"schemas": []any{userSchema},
			},
		},
		{
			name:         "schemas that name another schema are refused",
			ops:          `[{"op":"replace","value":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User","urn:example:custom"],"nickName":"Nick"}}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name:         "removing schemas is refused",
			ops:          `[{"op":"remove","path":"schemas"}]`,
			wantScimType: scimTypeMutability,
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
			name:         "a pathless attribute of an unknown extension is refused",
			ops:          `[{"op":"replace","value":{"urn:example:params:scim:schemas:extension:custom:2.0:User":{"department":"x"}}}]`,
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
			name:         "a path into an unknown extension is refused",
			ops:          `[{"op":"replace","path":"urn:example:params:scim:schemas:extension:custom:2.0:User:department","value":"x"}]`,
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
			name: "a null sub-attribute of a complex value removes it",
			ops:  `[{"op":"replace","path":"name","value":{"givenName":null,"familyName":"New"}}]`,
			want: map[string]any{
				"name": map[string]any{
					"familyName": "New",
				},
			},
		},
		{
			name: "a complex value whose sub-attributes are all null removes the attribute, without a path",
			ops:  `[{"op":"replace","value":{"name":{"givenName":null,"familyName":null}}}]`,
			want: map[string]any{
				"name": nil,
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

func TestApplyPatchUserAddressesAndEnterpriseExtension(t *testing.T) {
	entra := adapter.PatchRules{
		ReplaceAddsUnmatched: true,
	}
	// withChanges returns the test resource with the given attributes set, or removed when nil.
	withChanges := func(changes map[string]any) map[string]any {
		resource := testExtendedUserResource()
		for key, value := range changes {
			if value == nil {
				delete(resource, key)
				continue
			}
			resource[key] = value
		}
		return resource
	}
	tests := []struct {
		name         string
		ops          string
		rules        adapter.PatchRules
		want         map[string]any
		wantScimType string
	}{
		{
			name:  "adds to the work address, as Entra's provision on demand does",
			ops:   `[{"op":"Add","path":"addresses[type eq \"work\"].formatted","value":"Building 1"},{"op":"Add","path":"addresses[type eq \"work\"].locality","value":"Redmond"}]`,
			rules: entra,
			want: withChanges(map[string]any{
				"addresses": []any{
					map[string]any{
						"type":      "work",
						"locality":  "Redmond",
						"formatted": "Building 1",
					},
				},
			}),
		},
		{
			name:  "adding to an address that does not exist creates it",
			ops:   `[{"op":"Add","path":"addresses[type eq \"home\"].locality","value":"Kirkland"}]`,
			rules: entra,
			want: withChanges(map[string]any{
				"addresses": []any{
					map[string]any{
						"type":     "work",
						"locality": "Seattle",
					},
					map[string]any{
						"type":     "home",
						"locality": "Kirkland",
					},
				},
			}),
		},
		{
			name:  "replacing a sub-attribute of an address that does not exist creates it under Entra's rules",
			ops:   `[{"op":"Replace","path":"addresses[type eq \"home\"].formatted","value":"Home"}]`,
			rules: entra,
			want: withChanges(map[string]any{
				"addresses": []any{
					map[string]any{
						"type":     "work",
						"locality": "Seattle",
					},
					map[string]any{
						"type":      "home",
						"formatted": "Home",
					},
				},
			}),
		},
		{
			name:         "replacing a sub-attribute of an address that does not exist fails without Entra's rules",
			ops:          `[{"op":"Replace","path":"addresses[type eq \"home\"].formatted","value":"Home"}]`,
			wantScimType: scimTypeNoTarget,
		},
		{
			name: "replaces a sub-attribute of the work address",
			ops:  `[{"op":"Replace","path":"addresses[type eq \"work\"].locality","value":"Redmond"}]`,
			want: withChanges(map[string]any{
				"addresses": []any{
					map[string]any{
						"type":     "work",
						"locality": "Redmond",
					},
				},
			}),
		},
		{
			name: "removes a sub-attribute of the work address",
			ops:  `[{"op":"Remove","path":"addresses[type eq \"work\"].locality"}]`,
			want: withChanges(map[string]any{
				"addresses": []any{
					map[string]any{
						"type": "work",
					},
				},
			}),
		},
		{
			name: "removes the work address",
			ops:  `[{"op":"Remove","path":"addresses[type eq \"work\"]"}]`,
			want: withChanges(map[string]any{
				"addresses": []any{},
			}),
		},
		{
			name: "removes a listed address",
			ops:  `[{"op":"Remove","path":"addresses","value":[{"type":"work","locality":"Seattle"}]}]`,
			want: withChanges(map[string]any{
				"addresses": []any{},
			}),
		},
		{
			name: "removes nothing for a listed address that differs",
			ops:  `[{"op":"Remove","path":"addresses","value":[{"type":"work","locality":"Redmond"}]}]`,
			want: testExtendedUserResource(),
		},
		{
			name: "removes nothing for a listed address that sets nothing",
			ops:  `[{"op":"Remove","path":"addresses","value":[{"primary":false}]}]`,
			want: testExtendedUserResource(),
		},
		{
			name:  "adds extension attributes, as Entra does",
			ops:   `[{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:employeeNumber","value":"42"},{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department","value":"Sales"}]`,
			rules: entra,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"employeeNumber": "42",
					"department":     "Sales",
					"manager": map[string]any{
						"value": "manager-1",
					},
				},
			}),
		},
		{
			name: "extension URNs and attribute names are case-insensitive",
			ops:  `[{"op":"replace","path":"URN:IETF:PARAMS:SCIM:SCHEMAS:EXTENSION:ENTERPRISE:2.0:USER:DEPARTMENT","value":"Sales"}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Sales",
					"manager": map[string]any{
						"value": "manager-1",
					},
				},
			}),
		},
		{
			name: "replaces the manager with a string",
			ops:  `[{"op":"Replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager","value":"manager-2"}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
					"manager": map[string]any{
						"value": "manager-2",
					},
				},
			}),
		},
		{
			name: "replaces the manager with an object",
			ops:  `[{"op":"Replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager","value":{"value":"manager-3"}}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
					"manager": map[string]any{
						"value": "manager-3",
					},
				},
			}),
		},
		{
			name: "a null manager ID removes the manager",
			ops:  `[{"op":"Replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager","value":{"value":null}}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
				},
			}),
		},
		{
			name: "a null manager ID removes the manager, without a path",
			ops:  `[{"op":"Replace","value":{"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":{"manager":{"value":null}}}}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
				},
			}),
		},
		{
			name: "pathless schemas without the extension, which the user has, are ignored",
			ops:  `[{"op":"replace","value":{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"title":"Engineer"}}]`,
			want: withChanges(map[string]any{
				"title": "Engineer",
			}),
		},
		{
			name:  "replaces the manager with a list of one object, as Microsoft's SCIM documentation shows Entra sending it",
			ops:   `[{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager","value":[{"$ref":"https://example.com/scim/Users/manager-5","value":"manager-5"}]}]`,
			rules: entra,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
					"manager": map[string]any{
						"value": "manager-5",
						"$ref":  "https://example.com/scim/Users/manager-5",
					},
				},
			}),
		},
		{
			name:         "a manager list of more than one is refused",
			ops:          `[{"op":"Replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager","value":[{"value":"manager-5"},{"value":"manager-6"}]}]`,
			wantScimType: scimTypeInvalidValue,
		},
		{
			name: "replaces a sub-attribute of the manager",
			ops:  `[{"op":"Replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager.value","value":"manager-4"}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
					"manager": map[string]any{
						"value": "manager-4",
					},
				},
			}),
		},
		{
			name:         "the manager's displayName is read-only",
			ops:          `[{"op":"Replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager.displayName","value":"Boss"}]`,
			wantScimType: scimTypeMutability,
		},
		{
			name: "pathless extension object",
			ops:  `[{"op":"Replace","value":{"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":{"department":"Sales","costCenter":"CC1"},"title":"Engineer"}}]`,
			want: withChanges(map[string]any{
				"title": "Engineer",
				enterpriseUserSchema: map[string]any{
					"department": "Sales",
					"costCenter": "CC1",
					"manager": map[string]any{
						"value": "manager-1",
					},
				},
			}),
		},
		{
			name: "pathless extension attribute",
			ops:  `[{"op":"Replace","value":{"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:division":"Cloud"}}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
					"division":   "Cloud",
					"manager": map[string]any{
						"value": "manager-1",
					},
				},
			}),
		},
		{
			name:         "a pathless extension value that is not an object is refused",
			ops:          `[{"op":"Replace","value":{"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":"Sales"}}]`,
			wantScimType: scimTypeInvalidValue,
		},
		{
			name: "removes an extension attribute",
			ops:  `[{"op":"Remove","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department"}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"manager": map[string]any{
						"value": "manager-1",
					},
				},
			}),
		},
		{
			name: "removing every extension attribute removes the extension object",
			ops:  `[{"op":"Remove","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department"},{"op":"Remove","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager"}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: nil,
			}),
		},
		{
			name: "removing the manager's value removes the manager",
			ops:  `[{"op":"Remove","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager.value"}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"department": "Engineering",
				},
			}),
		},
		{
			name: "replacing an extension attribute with null removes it",
			ops:  `[{"op":"Replace","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department","value":null}]`,
			want: withChanges(map[string]any{
				enterpriseUserSchema: map[string]any{
					"manager": map[string]any{
						"value": "manager-1",
					},
				},
			}),
		},
		{
			name:         "an unknown extension attribute is refused",
			ops:          `[{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:badge","value":"1"}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name:         "an extension's attributes are not core attributes",
			ops:          `[{"op":"Add","path":"department","value":"Sales"}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name:         "core attributes are not an extension's attributes",
			ops:          `[{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:externalId","value":"x"}]`,
			wantScimType: scimTypeInvalidPath,
		},
		{
			name:         "an unknown URN is refused",
			ops:          `[{"op":"Add","path":"urn:example:params:scim:schemas:extension:custom:2.0:User:department","value":"Sales"}]`,
			wantScimType: scimTypeInvalidPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := testExtendedUserResource()
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
			if !reflect.DeepEqual(resource, tt.want) {
				t.Fatalf("resource = %#v, want %#v", resource, tt.want)
			}
		})
	}
}

func TestApplyPatchAddsExtensionToAUserWithoutOne(t *testing.T) {
	resource := testUserResource()
	ops := decodeTestOperations(t, `[{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department","value":"Sales"}]`)
	if err := applyPatch(userResourceSchema, resource, ops, adapter.PatchRules{}); err != nil {
		t.Fatalf("applyPatch() error = %v", err)
	}
	want := map[string]any{
		"department": "Sales",
	}
	if got := resource[enterpriseUserSchema]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", enterpriseUserSchema, got, want)
	}

	// Removing what was never there leaves no empty extension object.
	resource = testUserResource()
	ops = decodeTestOperations(t, `[{"op":"Remove","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department"}]`)
	if err := applyPatch(userResourceSchema, resource, ops, adapter.PatchRules{}); err != nil {
		t.Fatalf("applyPatch() error = %v", err)
	}
	if got, ok := resource[enterpriseUserSchema]; ok {
		t.Fatalf("%s = %#v, want none", enterpriseUserSchema, got)
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

// testExtendedUserResource returns the test user with a work address and enterprise extension attributes.
func testExtendedUserResource() map[string]any {
	resource := testUserResource()
	resource["schemas"] = []any{userSchema, enterpriseUserSchema}
	resource["addresses"] = []any{
		map[string]any{
			"type":     "work",
			"locality": "Seattle",
		},
	}
	resource[enterpriseUserSchema] = map[string]any{
		"department": "Engineering",
		"manager": map[string]any{
			"value": "manager-1",
		},
	}
	return resource
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

func TestApplyPatchManagerRef(t *testing.T) {
	const (
		manager = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager"
		ref1    = "https://example.com/scim/Users/manager-1"
	)
	tests := []struct {
		name string
		ops  string
		want map[string]any
	}{
		{
			name: "a new manager ID alone drops the old manager's $ref",
			ops:  `[{"op":"Replace","path":"` + manager + `","value":["manager-6"]}]`,
			want: map[string]any{
				"value": "manager-6",
			},
		},
		{
			name: "a new manager.value drops the old manager's $ref",
			ops:  `[{"op":"Replace","path":"` + manager + `.value","value":"manager-7"}]`,
			want: map[string]any{
				"value": "manager-7",
			},
		},
		{
			name: "a $ref set in the same request is kept",
			ops:  `[{"op":"Replace","path":"` + manager + `.$ref","value":"https://example.com/scim/Users/manager-8"},{"op":"Replace","path":"` + manager + `.value","value":"manager-8"}]`,
			want: map[string]any{
				"value": "manager-8",
				"$ref":  "https://example.com/scim/Users/manager-8",
			},
		},
		{
			name: "restating the manager's ID keeps its $ref",
			ops:  `[{"op":"Replace","path":"` + manager + `","value":"manager-1"}]`,
			want: map[string]any{
				"value": "manager-1",
				"$ref":  ref1,
			},
		},
		{
			name: "a manager ID that differs only in case keeps its $ref",
			ops:  `[{"op":"Replace","path":"` + manager + `","value":"MANAGER-1"}]`,
			want: map[string]any{
				"value": "MANAGER-1",
				"$ref":  ref1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := testExtendedUserResource()
			resource[enterpriseUserSchema] = map[string]any{
				"manager": map[string]any{
					"value": "manager-1",
					"$ref":  ref1,
				},
			}
			if err := applyPatch(userResourceSchema, resource, decodeTestOperations(t, tt.ops), adapter.PatchRules{}); err != nil {
				t.Fatalf("applyPatch() error = %v", err)
			}
			got, _ := resource[enterpriseUserSchema].(map[string]any)
			if !reflect.DeepEqual(got["manager"], tt.want) {
				t.Fatalf("manager = %#v, want %#v", got["manager"], tt.want)
			}
		})
	}

	// Removing the manager's ID, or setting it to null, removes its $ref, and so the manager.
	for _, ops := range []string{
		`[{"op":"Remove","path":"` + manager + `.value"}]`,
		`[{"op":"Replace","path":"` + manager + `","value":{"value":null}}]`,
	} {
		resource := testExtendedUserResource()
		resource[enterpriseUserSchema] = map[string]any{
			"department": "Engineering",
			"manager": map[string]any{
				"value": "manager-1",
				"$ref":  ref1,
			},
		}
		if err := applyPatch(userResourceSchema, resource, decodeTestOperations(t, ops), adapter.PatchRules{}); err != nil {
			t.Fatalf("applyPatch(%s) error = %v", ops, err)
		}
		if want := map[string]any{"department": "Engineering"}; !reflect.DeepEqual(resource[enterpriseUserSchema], want) {
			t.Fatalf("applyPatch(%s): %s = %#v, want %#v", ops, enterpriseUserSchema, resource[enterpriseUserSchema], want)
		}
	}
}
