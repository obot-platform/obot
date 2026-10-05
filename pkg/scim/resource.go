package scim

import (
	"strings"
	"time"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

// userResource returns the SCIM representation of a user, with canonical attribute names.
func userResource(u *gclient.SCIMUser, baseURL string) map[string]any {
	groups := make([]any, 0, len(u.Groups))
	for _, g := range u.Groups {
		groups = append(groups, map[string]any{
			"value":   g.ID,
			"$ref":    baseURL + "/Groups/" + g.ID,
			"display": g.DisplayName,
			"type":    "direct",
		})
	}

	m := map[string]any{
		"schemas":  []any{userSchema},
		"id":       u.ID,
		"userName": u.UserName,
		"active":   u.Active,
		"groups":   groups,
		"meta":     meta("User", u.CreatedAt, u.UpdatedAt, baseURL+"/Users/"+u.ID),
	}
	setString(m, "externalId", u.ExternalID)

	p := u.Profile
	if p.Name != nil {
		name := map[string]any{}
		setString(name, "formatted", p.Name.Formatted)
		setString(name, "familyName", p.Name.FamilyName)
		setString(name, "givenName", p.Name.GivenName)
		setString(name, "middleName", p.Name.MiddleName)
		setString(name, "honorificPrefix", p.Name.HonorificPrefix)
		setString(name, "honorificSuffix", p.Name.HonorificSuffix)
		if len(name) > 0 {
			m["name"] = name
		}
	}
	setString(m, "displayName", p.DisplayName)
	setString(m, "nickName", p.NickName)
	setString(m, "profileUrl", p.ProfileURL)
	setString(m, "title", p.Title)
	setString(m, "userType", p.UserType)
	setString(m, "preferredLanguage", p.PreferredLanguage)
	setString(m, "locale", p.Locale)
	setString(m, "timezone", p.Timezone)
	if emails := multiValues(p.Emails); len(emails) > 0 {
		m["emails"] = emails
	}
	if phoneNumbers := multiValues(p.PhoneNumbers); len(phoneNumbers) > 0 {
		m["phoneNumbers"] = phoneNumbers
	}

	return m
}

// groupResource returns the SCIM representation of a group, with canonical attribute names. Members are
// included only when the group was loaded with them.
func groupResource(g *gclient.SCIMGroup, baseURL string) map[string]any {
	m := map[string]any{
		"schemas":     []any{groupSchema},
		"id":          g.ID,
		"displayName": g.DisplayName,
		"meta":        meta("Group", g.CreatedAt, g.UpdatedAt, baseURL+"/Groups/"+g.ID),
	}

	if g.Members != nil {
		members := make([]any, 0, len(g.Members))
		for _, member := range g.Members {
			members = append(members, map[string]any{
				"value":   member.ID,
				"$ref":    baseURL + "/Users/" + member.ID,
				"display": member.UserName,
				"type":    "User",
			})
		}
		m["members"] = members
	}

	return m
}

// userInputFromResource converts a user representation, from a request body or a patched resource, into the
// attributes a write stores. Read-only and unsupported attributes, such as id, meta, groups, and password, are
// ignored.
func userInputFromResource(body map[string]any) (gclient.SCIMUserInput, error) {
	m, err := canonicalResource(userResourceSchema, body)
	if err != nil {
		return gclient.SCIMUserInput{}, err
	}

	input := gclient.SCIMUserInput{
		UserName:   stringValue(m, "userName"),
		ExternalID: stringValue(m, "externalId"),
		Profile: types.SCIMUserProfile{
			DisplayName:       stringValue(m, "displayName"),
			NickName:          stringValue(m, "nickName"),
			ProfileURL:        stringValue(m, "profileUrl"),
			Title:             stringValue(m, "title"),
			UserType:          stringValue(m, "userType"),
			PreferredLanguage: stringValue(m, "preferredLanguage"),
			Locale:            stringValue(m, "locale"),
			Timezone:          stringValue(m, "timezone"),
			Emails:            multiValuesFrom(m["emails"]),
			PhoneNumbers:      multiValuesFrom(m["phoneNumbers"]),
		},
	}
	if active, ok := m["active"].(bool); ok {
		input.Active = &active
	}
	if name, ok := m["name"].(map[string]any); ok {
		input.Profile.Name = &types.SCIMName{
			Formatted:       stringValue(name, "formatted"),
			FamilyName:      stringValue(name, "familyName"),
			GivenName:       stringValue(name, "givenName"),
			MiddleName:      stringValue(name, "middleName"),
			HonorificPrefix: stringValue(name, "honorificPrefix"),
			HonorificSuffix: stringValue(name, "honorificSuffix"),
		}
	}

	if strings.TrimSpace(input.UserName) == "" {
		return gclient.SCIMUserInput{}, badRequest(scimTypeInvalidValue, "userName is required")
	}
	// RFC 7643 section 2.4 allows one primary value at most.
	for _, attr := range []string{"emails", "phoneNumbers"} {
		if primaryValues(m[attr]) > 1 {
			return gclient.SCIMUserInput{}, badRequest(scimTypeInvalidValue, "%s can have at most one primary value", attr)
		}
	}
	return input, nil
}

// primaryValues returns the number of values of a multi-valued attribute that are primary.
func primaryValues(value any) int {
	var n int
	for _, item := range listValue(value) {
		if m, ok := item.(map[string]any); ok && m["primary"] == true {
			n++
		}
	}
	return n
}

// groupInputFromResource converts a group representation, from a request body or a patched resource, into the
// attributes a write stores. Members are the complete member set.
func groupInputFromResource(body map[string]any) (gclient.SCIMGroupInput, error) {
	m, err := canonicalResource(groupResourceSchema, body)
	if err != nil {
		return gclient.SCIMGroupInput{}, err
	}

	memberIDs, err := groupMemberIDs(m["members"])
	if err != nil {
		return gclient.SCIMGroupInput{}, err
	}
	input := gclient.SCIMGroupInput{
		DisplayName: stringValue(m, "displayName"),
		MemberIDs:   memberIDs,
	}

	if strings.TrimSpace(input.DisplayName) == "" {
		return gclient.SCIMGroupInput{}, badRequest(scimTypeInvalidValue, "displayName is required")
	}
	return input, nil
}

// canonicalResource returns the resource's writable attributes under their canonical names, checking their
// types. Attribute names are case-insensitive. Unknown, extension, and read-only attributes are dropped.
func canonicalResource(schema *resourceSchema, body map[string]any) (map[string]any, error) {
	m := make(map[string]any, len(body))
	for key, value := range body {
		attr := schema.attribute(key)
		if attr == nil || attr.Mutability == mutabilityReadOnly || value == nil {
			continue
		}

		v, err := canonicalValue(attr, value)
		if err != nil {
			return nil, err
		}
		m[attr.Name] = v
	}
	return m, nil
}

// canonicalValue checks a value against its attribute and returns it with canonical sub-attribute names.
func canonicalValue(attr *attribute, value any) (any, error) {
	if attr.MultiValued {
		return canonicalList(attr, value)
	}
	return canonicalSingle(attr, value)
}

// canonicalList returns the values of a multi-valued attribute. A single value is accepted as a list of one.
func canonicalList(attr *attribute, value any) ([]any, error) {
	items, ok := value.([]any)
	if !ok {
		items = []any{value}
	}

	out := make([]any, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		v, err := canonicalSingle(attr, item)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func canonicalSingle(attr *attribute, value any) (any, error) {
	switch attr.Type {
	case typeComplex:
		m, ok := value.(map[string]any)
		if !ok {
			return nil, badRequest(scimTypeInvalidValue, "%q must be an object", attr.Name)
		}
		out := make(map[string]any, len(m))
		for key, subValue := range m {
			sub := attr.subAttribute(key)
			if sub == nil || subValue == nil {
				continue
			}
			v, err := canonicalSingle(sub, subValue)
			if err != nil {
				return nil, err
			}
			out[sub.Name] = v
		}
		return out, nil
	case typeBoolean:
		switch v := value.(type) {
		case bool:
			return v, nil
		case string:
			// Some clients send booleans as strings, such as "False".
			if strings.EqualFold(v, "true") {
				return true, nil
			} else if strings.EqualFold(v, "false") {
				return false, nil
			}
		}
		return nil, badRequest(scimTypeInvalidValue, "%q must be a boolean", attr.Name)
	default:
		s, ok := value.(string)
		if !ok {
			return nil, badRequest(scimTypeInvalidValue, "%q must be a string", attr.Name)
		}
		return s, nil
	}
}

func meta(resourceType string, created, lastModified time.Time, location string) map[string]any {
	return map[string]any{
		"resourceType": resourceType,
		"created":      created.UTC().Format(time.RFC3339),
		"lastModified": lastModified.UTC().Format(time.RFC3339),
		"location":     location,
	}
}

func multiValues(values []types.SCIMMultiValue) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		m := map[string]any{}
		setString(m, "value", v.Value)
		setString(m, "display", v.Display)
		setString(m, "type", v.Type)
		if v.Primary {
			m["primary"] = true
		}
		out = append(out, m)
	}
	return out
}

func multiValuesFrom(value any) []types.SCIMMultiValue {
	items := listValue(value)
	out := make([]types.SCIMMultiValue, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]any)
		v := types.SCIMMultiValue{
			Value:   stringValue(m, "value"),
			Display: stringValue(m, "display"),
			Type:    stringValue(m, "type"),
		}
		v.Primary, _ = m["primary"].(bool)
		if v != (types.SCIMMultiValue{}) {
			out = append(out, v)
		}
	}
	return out
}

func setString(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

// groupMemberIDs returns the SCIM user IDs of canonical members values.
func groupMemberIDs(members any) ([]string, error) {
	values := listValue(members)
	ids := make([]string, 0, len(values))
	for _, v := range values {
		member, _ := v.(map[string]any)
		id := stringValue(member, "value")
		if id == "" {
			return nil, badRequest(scimTypeInvalidValue, "every member requires a value")
		}
		if t := stringValue(member, "type"); t != "" && !strings.EqualFold(t, "User") {
			return nil, badRequest(scimTypeInvalidValue, "member %q has type %q; only users can be members", id, t)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func stringValue(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
