package scim

import (
	"strings"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
)

// memberChange is the membership that a group PATCH leaves a member with, and the member ID as the PATCH sent it.
type memberChange struct {
	id     string
	member bool
}

// planGroupPatch turns a PATCH of the group with SCIM ID groupID into a gclient.SCIMGroupPatch when every operation is
// one that Okta sends: a rename, or an add, a filtered remove, or a replacement of members. Applying such a patch reads
// and writes only the members it names, where applyPatch needs every member of the group. It reports false for any
// other operation, and for any value that fails validation, so that applyPatch handles the request and reports the
// failure.
func planGroupPatch(groupID string, ops []patchOperation) (gclient.SCIMGroupPatch, bool) {
	var (
		patch    gclient.SCIMGroupPatch
		replaced []string
		// changes maps the lowercased ID of each member that an operation after the last replacement names to the
		// ID as sent and whether it is a member afterwards. A filter matches member values case-insensitively.
		changes = map[string]memberChange{}
		order   []string
	)
	change := func(id string, member bool) {
		key := strings.ToLower(id)
		if _, ok := changes[key]; !ok {
			order = append(order, key)
		}
		changes[key] = memberChange{
			id:     id,
			member: member,
		}
	}

	for _, op := range ops {
		if op.Path == "" {
			name, ok := pathlessGroupRename(groupID, op)
			if !ok {
				return gclient.SCIMGroupPatch{}, false
			}
			patch.DisplayName = name
			continue
		}

		path, err := parsePatchPath(op.Path)
		if err != nil || !path.Attr.inSchema(groupResourceSchema) || path.Attr.Sub != "" || path.Sub != "" {
			return gclient.SCIMGroupPatch{}, false
		}
		switch {
		case strings.EqualFold(path.Attr.Name, "displayName") && path.Filter == nil && op.Op == patchReplace:
			name, ok := op.Value.(string)
			if !ok || strings.TrimSpace(name) == "" {
				return gclient.SCIMGroupPatch{}, false
			}
			patch.DisplayName = name
		case strings.EqualFold(path.Attr.Name, "members") && path.Filter == nil && op.Op == patchReplace:
			ids, ok := patchMemberIDs(op.Value)
			if !ok {
				return gclient.SCIMGroupPatch{}, false
			}
			patch.ReplaceMembers = true
			replaced = ids
			changes, order = map[string]memberChange{}, nil
		case strings.EqualFold(path.Attr.Name, "members") && path.Filter == nil && op.Op == patchAdd:
			ids, ok := patchMemberIDs(op.Value)
			if !ok {
				return gclient.SCIMGroupPatch{}, false
			}
			for _, id := range ids {
				change(id, true)
			}
		case strings.EqualFold(path.Attr.Name, "members") && path.Filter != nil && op.Op == patchRemove:
			id, ok := memberValueEquals(path.Filter)
			if !ok {
				return gclient.SCIMGroupPatch{}, false
			}
			change(id, false)
		default:
			return gclient.SCIMGroupPatch{}, false
		}
	}

	if patch.ReplaceMembers {
		patch.MemberIDs = make([]string, 0, len(replaced)+len(changes))
		inReplacement := make(map[string]struct{}, len(replaced))
		for _, id := range replaced {
			key := strings.ToLower(id)
			inReplacement[key] = struct{}{}
			if c, ok := changes[key]; !ok || c.member {
				patch.MemberIDs = append(patch.MemberIDs, id)
			}
		}
		for _, key := range order {
			if _, ok := inReplacement[key]; ok {
				continue
			}
			if c := changes[key]; c.member {
				patch.MemberIDs = append(patch.MemberIDs, c.id)
			}
		}
		return patch, true
	}

	for _, key := range order {
		if c := changes[key]; c.member {
			patch.MemberIDs = append(patch.MemberIDs, c.id)
		} else {
			patch.RemovedMemberIDs = append(patch.RemovedMemberIDs, key)
		}
	}
	return patch, true
}

// pathlessGroupRename returns the name that a replacement without a path sets, as Okta sends it: the group's
// displayName, and its id, which is read-only and left as it is.
func pathlessGroupRename(groupID string, op patchOperation) (string, bool) {
	values, ok := op.Value.(map[string]any)
	if op.Op != patchReplace || !ok {
		return "", false
	}

	var name string
	for key, value := range values {
		switch {
		case strings.EqualFold(key, "id"):
			if value != groupID {
				return "", false
			}
		case strings.EqualFold(key, "displayName"):
			s, ok := value.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return "", false
			}
			name = s
		default:
			return "", false
		}
	}
	return name, name != ""
}

// patchMemberIDs returns the SCIM user IDs of a members value, or false if the value is not valid.
func patchMemberIDs(value any) ([]string, bool) {
	if value == nil {
		return nil, false
	}
	members, err := canonicalValue(groupResourceSchema.attribute("members"), value)
	if err != nil {
		return nil, false
	}
	ids, err := groupMemberIDs(members)
	return ids, err == nil
}

// memberValueEquals returns the member value of a filter that selects one member by it, as members[value eq "id"]
// does.
func memberValueEquals(f filter) (string, bool) {
	c, ok := f.(comparison)
	if !ok || c.Operator != "eq" || c.Path.URN != "" || c.Path.Sub != "" || !strings.EqualFold(c.Path.Name, "value") {
		return "", false
	}
	s, ok := c.Value.(string)
	return s, ok && s != ""
}
