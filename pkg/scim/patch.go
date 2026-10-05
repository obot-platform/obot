package scim

import (
	"cmp"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/obot-platform/obot/pkg/scim/adapter"
)

const (
	patchAdd     = "add"
	patchReplace = "replace"
	patchRemove  = "remove"

	// maxPatchOperations bounds the operations of one PATCH request. Okta sends at most a few.
	maxPatchOperations = 100
)

type patchOperation struct {
	Op    string
	Path  string
	Value any
	// HasValue is set when the operation has a value member, even a null one.
	HasValue bool
}

// patchPath is a PATCH target: an attribute, an optional value filter, and an optional sub-attribute.
type patchPath struct {
	Attr   attrPath
	Filter filter
	// Sub is the sub-attribute after a value filter, as in emails[type eq "work"].value. Without a filter, the
	// sub-attribute is Attr.Sub.
	Sub string
}

// pathlessMember is a member of the value of an operation without a path, which applies as though its key were the
// path.
type pathlessMember struct {
	key   string
	path  patchPath
	value any
}

// valueSet holds the values of a multi-valued complex attribute for membership tests. Values identified by a value
// sub-attribute, such as group members, match on it alone, in constant time. Other values, which are few, match the
// values they select, as selects says.
type valueSet struct {
	attr    *attribute
	byValue map[string]struct{}
	others  []map[string]any
}

// decodePatchOperations reads the operations of a PatchOp request. Member names are case-insensitive.
func decodePatchOperations(body map[string]any) ([]patchOperation, error) {
	if schemas, ok := lookupKey(body, "schemas").([]any); ok && !containsFold(schemas, patchOpSchema) {
		return nil, badRequest(scimTypeInvalidSyntax, "request is not a %s message", patchOpSchema)
	}

	raw, ok := lookupKey(body, "Operations").([]any)
	if !ok || len(raw) == 0 {
		return nil, badRequest(scimTypeInvalidSyntax, "Operations must be a non-empty array")
	}
	if len(raw) > maxPatchOperations {
		return nil, badRequest(scimTypeInvalidSyntax, "a request can have at most %d operations", maxPatchOperations)
	}

	ops := make([]patchOperation, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, badRequest(scimTypeInvalidSyntax, "each operation must be an object")
		}

		op, _ := lookupKey(m, "op").(string)
		op = strings.ToLower(op)
		if op != patchAdd && op != patchReplace && op != patchRemove {
			return nil, badRequest(scimTypeInvalidSyntax, "unsupported operation %q", op)
		}

		path, ok := lookupKey(m, "path").(string)
		if !ok && lookupKey(m, "path") != nil {
			return nil, badRequest(scimTypeInvalidPath, "path must be a string")
		}

		ops = append(ops, patchOperation{
			Op:       op,
			Path:     strings.TrimSpace(path),
			Value:    lookupKey(m, "value"),
			HasValue: hasKey(m, "value"),
		})
	}
	return ops, nil
}

// applyPatch applies operations, in order, to resource, which holds canonical attribute names, as RFC 7644 says
// unless rules, the client's, say otherwise. The caller validates the result and commits it once, so a failed
// operation leaves nothing applied.
func applyPatch(schema *resourceSchema, resource map[string]any, ops []patchOperation, rules adapter.PatchRules) error {
	for _, op := range ops {
		if err := applyOperation(schema, resource, op, rules); err != nil {
			return err
		}
	}
	return nil
}

func applyOperation(schema *resourceSchema, resource map[string]any, op patchOperation, rules adapter.PatchRules) error {
	if op.Path == "" {
		if op.Op == patchRemove {
			return badRequest(scimTypeNoTarget, "remove requires a path")
		}

		values, ok := op.Value.(map[string]any)
		if !ok {
			return badRequest(scimTypeInvalidValue, "an %s without a path requires an object value", op.Op)
		}
		members, err := pathlessMembers(schema, values)
		if err != nil {
			return err
		}

		// Each member of the value applies as though its key were the path.
		for _, m := range members {
			if err := applyToTarget(schema, resource, op.Op, m.key, m.path, m.value, rules); err != nil {
				return err
			}
		}
		return nil
	}

	path, err := parsePatchPath(op.Path)
	if err != nil {
		return err
	}
	if op.Op != patchRemove && !op.HasValue {
		return badRequest(scimTypeInvalidValue, "%s requires a value", op.Op)
	}
	return applyToTarget(schema, resource, op.Op, op.Path, path, op.Value, rules)
}

// pathlessMembers returns the members of the value of an operation without a path, in the order they apply. JSON
// objects have no order, so they apply by attribute, each attribute before the members that select a part of it,
// which refine it. The members of an object under the resource's own schema URN are members too.
func pathlessMembers(schema *resourceSchema, values map[string]any) ([]pathlessMember, error) {
	members := make([]pathlessMember, 0, len(values))
	add := func(key string, value any) error {
		path, err := parsePatchPath(key)
		if err != nil {
			return err
		}
		members = append(members, pathlessMember{
			key:   key,
			path:  path,
			value: value,
		})
		return nil
	}

	for key, value := range values {
		if !strings.EqualFold(key, schema.ID) {
			if err := add(key, value); err != nil {
				return nil, err
			}
			continue
		}

		nested, ok := value.(map[string]any)
		if !ok {
			return nil, badRequest(scimTypeInvalidValue, "%q must be an object", key)
		}
		for nestedKey, value := range nested {
			if err := add(key+":"+nestedKey, value); err != nil {
				return nil, err
			}
		}
	}

	part := func(p patchPath) int {
		if p.Filter == nil && p.Attr.Sub == "" {
			return 0
		}
		return 1
	}
	slices.SortFunc(members, func(a, b pathlessMember) int {
		return cmp.Or(
			strings.Compare(strings.ToLower(a.path.Attr.Name), strings.ToLower(b.path.Attr.Name)),
			cmp.Compare(part(a.path), part(b.path)),
			strings.Compare(a.key, b.key),
		)
	})
	return members, nil
}

// applyToTarget applies an operation to the attribute that path, which target spells, selects. An attribute that the
// schema ignores is left alone, and so is a read-only attribute that the operation leaves as it is, as when a client
// echoes the id. Any other change to a read-only attribute is refused.
func applyToTarget(schema *resourceSchema, resource map[string]any, op, target string, path patchPath, value any, rules adapter.PatchRules) error {
	if !path.Attr.inSchema(schema) {
		return badRequest(scimTypeInvalidPath, "attribute path %q is not in the %s schema", target, schema.Name)
	}
	if schema.ignores(path.Attr.Name) {
		return nil
	}
	attr := schema.attribute(path.Attr.Name)
	if attr == nil {
		return badRequest(scimTypeInvalidPath, "unknown attribute %q", path.Attr.Name)
	}
	// Adding to a multi-valued attribute only adds values, as RFC 7644 section 3.5.2.1 says, and null, like an empty
	// array, adds none. That changes nothing, even of a read-only attribute.
	if value == nil && op == patchAdd && attr.MultiValued && path.Attr.Sub == "" && path.Sub == "" {
		return nil
	}
	if attr.Mutability == mutabilityReadOnly {
		if op != patchRemove && path.Filter == nil && path.Attr.Sub == "" && unchanged(resource, attr, value) {
			return nil
		}
		// An add of values the attribute already holds changes nothing either.
		if op == patchAdd && attr.MultiValued && path.Filter == nil && path.Attr.Sub == "" && alreadyHeld(resource, attr, value) {
			return nil
		}
		return badRequest(scimTypeMutability, "attribute %q is read-only", attr.Name)
	}
	// Otherwise, a null value makes what it is assigned to unassigned, as RFC 7643 section 2.5 says, which removes it.
	// A replace through a filter still needs a target, as one with a value does, so applyFiltered handles it.
	if value == nil && (op != patchReplace || path.Filter == nil) {
		op = patchRemove
	}
	// Removing an attribute that must have a value is refused, as RFC 7644 section 3.5.2.2 says of required ones.
	if (attr.Required || attr.unremovable) && path.Filter == nil && path.Attr.Sub == "" && op == patchRemove {
		return badRequest(scimTypeMutability, "attribute %q cannot be removed", attr.Name)
	}
	return applyToPath(resource, attr, path, op, value, rules)
}

// alreadyHeld reports whether the multi-valued attribute already holds every value that an add of value would add, so
// that the add changes nothing.
func alreadyHeld(resource map[string]any, attr *attribute, value any) bool {
	added, err := canonicalList(attr, value)
	if err != nil {
		return false
	}
	existing := listValue(resource[attr.Name])
	set := newValueSet(attr, existing)
	for _, v := range added {
		if set.byValue != nil {
			if !set.contains(v) {
				return false
			}
			continue
		}
		if !slices.ContainsFunc(existing, func(e any) bool { return sameValue(attr, e, v) }) {
			return false
		}
	}
	return true
}

// unchanged reports whether value is the attribute's current value.
func unchanged(resource map[string]any, attr *attribute, value any) bool {
	current, ok := resource[attr.Name]
	if value == nil {
		return !ok
	}
	v, err := canonicalValue(attr, value)
	return err == nil && reflect.DeepEqual(v, current)
}

// parsePatchPath parses a PATCH path: attrPath, or attrPath "[" valFilter "]" with an optional "." subAttr.
func parsePatchPath(s string) (patchPath, error) {
	open := strings.IndexByte(s, '[')
	if open < 0 {
		attr, ok := parseAttrPath(s)
		if !ok {
			return patchPath{}, badRequest(scimTypeInvalidPath, "invalid path %q", s)
		}
		return patchPath{
			Attr: attr,
		}, nil
	}

	closing := strings.LastIndexByte(s, ']')
	if closing < open {
		return patchPath{}, badRequest(scimTypeInvalidPath, "invalid path %q", s)
	}

	attr, ok := parseAttrPath(s[:open])
	if !ok || attr.Sub != "" {
		return patchPath{}, badRequest(scimTypeInvalidPath, "invalid path %q", s)
	}

	f, err := parseFilter(s[open+1 : closing])
	if err != nil {
		detail := err.Error()
		if e, ok := errors.AsType[*Error](err); ok {
			detail = e.Detail
		}
		// RFC 7644 section 3.12 names invalidFilter for a malformed filter in a PATCH path.
		return patchPath{}, badRequest(scimTypeInvalidFilter, "invalid value filter in path %q: %s", s, detail)
	}

	var sub string
	if rest := s[closing+1:]; rest != "" {
		sub = strings.TrimPrefix(rest, ".")
		if sub == rest || !validAttributeName(sub) {
			return patchPath{}, badRequest(scimTypeInvalidPath, "invalid path %q", s)
		}
	}

	return patchPath{
		Attr:   attr,
		Filter: f,
		Sub:    sub,
	}, nil
}

func applyToPath(resource map[string]any, attr *attribute, path patchPath, op string, value any, rules adapter.PatchRules) error {
	switch {
	case path.Filter != nil:
		if !attr.MultiValued || attr.Type != typeComplex {
			return badRequest(scimTypeInvalidPath, "a value filter applies only to a multi-valued complex attribute, not %q", attr.Name)
		}
		return applyFiltered(resource, attr, path, op, value, rules)
	case path.Attr.Sub != "":
		if attr.MultiValued || attr.Type != typeComplex {
			return badRequest(scimTypeInvalidPath, "%q has no sub-attributes that can be selected without a value filter", attr.Name)
		}
		sub := attr.subAttribute(path.Attr.Sub)
		if sub == nil {
			return badRequest(scimTypeInvalidPath, "unknown attribute %q", path.Attr.String())
		}
		if sub.Mutability == mutabilityReadOnly {
			return badRequest(scimTypeMutability, "attribute %q is read-only", path.Attr.String())
		}
		return applyToSubAttribute(resource, attr, sub, op, value)
	default:
		return applyToAttribute(resource, attr, op, value)
	}
}

func applyToAttribute(resource map[string]any, attr *attribute, op string, value any) error {
	if op == patchRemove {
		if attr.MultiValued && value != nil {
			// Some clients remove specific values by listing them instead of filtering.
			return removeValues(resource, attr, value)
		}
		delete(resource, attr.Name)
		return nil
	}

	if value == nil {
		delete(resource, attr.Name)
		return nil
	}

	switch {
	case attr.MultiValued:
		values, err := canonicalList(attr, value)
		if err != nil {
			return err
		}
		if op == patchReplace {
			resource[attr.Name] = values
			return nil
		}
		resource[attr.Name] = addValues(attr, listValue(resource[attr.Name]), values)
	case attr.Type == typeComplex:
		v, err := canonicalValue(attr, value)
		if err != nil {
			return err
		}
		// Replacing or adding a complex attribute changes only the sub-attributes the value specifies.
		merged, _ := resource[attr.Name].(map[string]any)
		if merged == nil {
			merged = map[string]any{}
		}
		maps.Copy(merged, v.(map[string]any))
		resource[attr.Name] = merged
	default:
		v, err := canonicalValue(attr, value)
		if err != nil {
			return err
		}
		resource[attr.Name] = v
	}
	return nil
}

func applyToSubAttribute(resource map[string]any, attr, sub *attribute, op string, value any) error {
	obj, _ := resource[attr.Name].(map[string]any)
	if op == patchRemove || value == nil {
		if obj != nil {
			delete(obj, sub.Name)
			if len(obj) == 0 {
				delete(resource, attr.Name)
			}
		}
		return nil
	}

	v, err := canonicalValue(sub, value)
	if err != nil {
		return err
	}
	if obj == nil {
		obj = map[string]any{}
		resource[attr.Name] = obj
	}
	obj[sub.Name] = v
	return nil
}

func applyFiltered(resource map[string]any, attr *attribute, path patchPath, op string, value any, rules adapter.PatchRules) error {
	var sub *attribute
	if path.Sub != "" {
		if sub = attr.subAttribute(path.Sub); sub == nil {
			return badRequest(scimTypeInvalidPath, "unknown attribute %q", attr.Name+"."+path.Sub)
		}
		if sub.Mutability == mutabilityReadOnly {
			return badRequest(scimTypeMutability, "attribute %q is read-only", attr.Name+"."+sub.Name)
		}
	}

	values := listValue(resource[attr.Name])
	var matched []int
	for i, v := range values {
		if m, ok := v.(map[string]any); ok && path.Filter.matches(attr, m) {
			matched = append(matched, i)
		}
	}

	// A replace with null removes what the filter selects. Like any replace, it fails when the filter selects nothing,
	// unless the client's rules make it do what an add does, which with null adds nothing.
	if op == patchReplace && value == nil {
		if len(matched) == 0 && !rules.ReplaceAddsUnmatched {
			return badRequest(scimTypeNoTarget, "no value of %q matches the filter", attr.Name)
		}
		op = patchRemove
	}

	switch op {
	case patchRemove:
		// Removing a value that is already gone is not an error, so that repeated requests converge.
		if sub == nil {
			kept := make([]any, 0, len(values))
			for i, v := range values {
				if !slices.Contains(matched, i) {
					kept = append(kept, v)
				}
			}
			setList(resource, attr, kept)
			return nil
		}
		for _, i := range matched {
			existing := values[i].(map[string]any)
			if _, ok := existing[sub.Name]; ok && sub.Mutability == mutabilityImmutable {
				return immutableError(attr, sub)
			}
			delete(existing, sub.Name)
		}
		setList(resource, attr, values)
		return nil
	case patchAdd, patchReplace:
		var created bool
		if len(matched) == 0 {
			// An add whose filter selects no value creates one. A replace fails, as RFC 7644 section 3.5.2.3 says,
			// unless the client's rules make it do what an add does.
			if op == patchReplace && !rules.ReplaceAddsUnmatched {
				return badRequest(scimTypeNoTarget, "no value of %q matches the filter", attr.Name)
			}
			if sub == nil {
				return addForFilter(resource, attr, path.Filter, value)
			}
			// A sub-attribute of a value selected by equality creates the value, as clients do for
			// emails[type eq "work"].value.
			newValue, ok := valueFromEqualityFilter(attr, path.Filter)
			if !ok {
				return badRequest(scimTypeNoTarget, "no value of %q matches the filter", attr.Name)
			}
			values = append(values, newValue)
			matched = []int{len(values) - 1}
			created = true
		}

		for _, i := range matched {
			existing := values[i].(map[string]any)
			if sub != nil {
				v, err := canonicalValue(sub, value)
				if err != nil {
					return err
				}
				// An immutable sub-attribute is set when its value is created, and never changes afterwards.
				if old, ok := existing[sub.Name]; ok && sub.Mutability == mutabilityImmutable && !reflect.DeepEqual(old, v) {
					return immutableError(attr, sub)
				}
				existing[sub.Name] = v
				// The value created for the filter must be one it selects, as addForFilter requires.
				if created && !path.Filter.matches(attr, existing) {
					return badRequest(scimTypeNoTarget, "no value of %q matches the filter", attr.Name)
				}
				continue
			}

			// The value replaces or merges into one selected value, so it is a single complex value.
			v, err := canonicalSingle(attr, value)
			if err != nil {
				return err
			}
			next := v.(map[string]any)
			for _, immutable := range attr.SubAttributes {
				if immutable.Mutability != mutabilityImmutable {
					continue
				}
				old, had := existing[immutable.Name]
				updated, has := next[immutable.Name]
				// A replacement drops what it omits, while a merge keeps it.
				if had && (has && !reflect.DeepEqual(old, updated) || !has && op == patchReplace) {
					return immutableError(attr, immutable)
				}
			}
			if op == patchReplace {
				values[i] = next
				continue
			}
			maps.Copy(existing, next)
		}
		keepOnePrimary(values, matched)
		setList(resource, attr, values)
		return nil
	}
	return nil
}

// keepOnePrimary makes the first changed value that is primary the only primary value, as RFC 7644 section 3.5.2
// requires of a PATCH that sets primary to true. addValues does the same for the values it adds.
func keepOnePrimary(values []any, changed []int) {
	primary := -1
	for _, i := range changed {
		if m, ok := values[i].(map[string]any); ok && m["primary"] == true {
			primary = i
			break
		}
	}
	if primary < 0 {
		return
	}
	for i, v := range values {
		if m, ok := v.(map[string]any); ok && i != primary {
			delete(m, "primary")
		}
	}
}

// immutableError reports a change to an immutable sub-attribute of an existing value, such as the value of a group
// member. Such values can only be added and removed.
func immutableError(attr, sub *attribute) *Error {
	return badRequest(scimTypeMutability, "attribute %q is immutable; add or remove the %s value instead", attr.Name+"."+sub.Name, attr.Name)
}

// addForFilter adds the value of an operation whose filter selects no value. The value takes the sub-attributes that
// an equality filter sets, unless it sets them itself, and must then match the filter, so that the filter selects it.
func addForFilter(resource map[string]any, attr *attribute, f filter, value any) error {
	v, err := canonicalSingle(attr, value)
	if err != nil {
		return err
	}
	created, ok := valueFromEqualityFilter(attr, f)
	if !ok {
		created = map[string]any{}
	}
	maps.Copy(created, v.(map[string]any))
	if !f.matches(attr, created) {
		return badRequest(scimTypeNoTarget, "no value of %q matches the filter, and the value does not match it either", attr.Name)
	}
	return applyToAttribute(resource, attr, patchAdd, created)
}

// valueFromEqualityFilter builds a new value from a filter of equality comparisons joined by "and".
func valueFromEqualityFilter(attr *attribute, f filter) (map[string]any, bool) {
	value := map[string]any{}
	var collect func(filter) bool
	collect = func(f filter) bool {
		switch f := f.(type) {
		case comparison:
			sub := attr.subAttribute(f.Path.Name)
			if f.Operator != "eq" || f.Path.Sub != "" || sub == nil {
				return false
			}
			value[sub.Name] = f.Value
			return true
		case logical:
			return f.Operator == "and" && collect(f.Left) && collect(f.Right)
		}
		return false
	}
	return value, collect(f)
}

// removeValues removes the listed values from a multi-valued attribute. Complex values are matched by their
// value sub-attribute, and by the other sub-attributes a listed value sets, as selects says.
func removeValues(resource map[string]any, attr *attribute, value any) error {
	remove, err := canonicalList(attr, value)
	if err != nil {
		return err
	}

	set := newValueSet(attr, remove)
	values := listValue(resource[attr.Name])
	kept := make([]any, 0, len(values))
	for _, v := range values {
		if !set.contains(v) {
			kept = append(kept, v)
		}
	}
	setList(resource, attr, kept)
	return nil
}

// addValues appends values that are not already present. A value that is present apart from its primary flag is not
// added again, so that repeating an add changes nothing. When an added value is primary, it becomes the only primary
// value.
func addValues(attr *attribute, existing, added []any) []any {
	set := newValueSet(attr, existing)
	for _, v := range added {
		primary := false
		if m, ok := v.(map[string]any); ok {
			primary = m["primary"] == true
		}

		i := -1
		if set.byValue != nil {
			if set.contains(v) {
				// Values identified by their value sub-attribute, such as group members, have no primary flag.
				continue
			}
		} else {
			i = slices.IndexFunc(existing, func(e any) bool { return sameValue(attr, e, v) })
		}

		if primary {
			for _, e := range existing {
				if em, ok := e.(map[string]any); ok {
					delete(em, "primary")
				}
			}
		}
		if i >= 0 {
			if primary {
				existing[i].(map[string]any)["primary"] = true
			}
			continue
		}
		existing = append(existing, v)
		set.add(v)
	}
	return existing
}

// sameValue reports whether two values of a multi-valued complex attribute are the same apart from their primary
// flags. Strings are compared case-insensitively unless they are case exact, and an empty string or a false flag is
// the same as none.
func sameValue(attr *attribute, a, b any) bool {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if !aok || !bok {
		return reflect.DeepEqual(a, b)
	}
	for _, sub := range attr.SubAttributes {
		if sub.Name != "primary" && !subValuesEqual(sub, am[sub.Name], bm[sub.Name]) {
			return false
		}
	}
	return true
}

// selects reports whether a value that a remove lists selects an existing value of a multi-valued complex attribute:
// it has the existing value's value, and every other sub-attribute it sets apart from primary is equal, as sameValue
// compares them. So a listed email address removes that address whatever its type.
func selects(attr *attribute, selector, value map[string]any) bool {
	if absent(selector["value"]) {
		return false
	}
	for _, sub := range attr.SubAttributes {
		if sub.Name != "primary" && !absent(selector[sub.Name]) && !subValuesEqual(sub, selector[sub.Name], value[sub.Name]) {
			return false
		}
	}
	return true
}

func subValuesEqual(sub *attribute, a, b any) bool {
	if absent(a) || absent(b) {
		return absent(a) && absent(b)
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok && !sub.CaseExact {
		return strings.EqualFold(as, bs)
	}
	return reflect.DeepEqual(a, b)
}

// absent reports whether a sub-attribute value is the same as none: nil, an empty string, or false.
func absent(v any) bool {
	return v == nil || v == "" || v == false
}

func newValueSet(attr *attribute, values []any) *valueSet {
	set := &valueSet{
		attr: attr,
	}
	if identifiedByValue(attr) {
		set.byValue = make(map[string]struct{}, len(values))
	}
	for _, v := range values {
		set.add(v)
	}
	return set
}

func (s *valueSet) add(v any) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	if s.byValue == nil {
		s.others = append(s.others, m)
		return
	}
	if key, ok := s.key(m); ok {
		s.byValue[key] = struct{}{}
	}
}

func (s *valueSet) contains(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if s.byValue == nil {
		return slices.ContainsFunc(s.others, func(other map[string]any) bool { return selects(s.attr, other, m) })
	}
	key, ok := s.key(m)
	if !ok {
		return false
	}
	_, found := s.byValue[key]
	return found
}

// key returns the value sub-attribute that a value identified by it matches on. Unless it is case exact, case is
// ignored, as a filter on it ignores case.
func (s *valueSet) key(m map[string]any) (string, bool) {
	value, ok := m["value"].(string)
	if !ok || value == "" {
		return "", false
	}
	if sub := s.attr.subAttribute("value"); sub == nil || !sub.CaseExact {
		value = strings.ToLower(value)
	}
	return value, true
}

// identifiedByValue reports whether the attribute's values are references identified by their value
// sub-attribute alone.
func identifiedByValue(attr *attribute) bool {
	return attr.subAttribute("$ref") != nil && attr.subAttribute("value") != nil
}

func setList(resource map[string]any, attr *attribute, values []any) {
	if values == nil {
		values = []any{}
	}
	resource[attr.Name] = values
}

func listValue(v any) []any {
	switch v := v.(type) {
	case []any:
		return v
	case []map[string]any:
		out := make([]any, 0, len(v))
		for _, m := range v {
			out = append(out, m)
		}
		return out
	}
	return nil
}

func containsFold(values []any, s string) bool {
	for _, v := range values {
		if str, ok := v.(string); ok && strings.EqualFold(str, s) {
			return true
		}
	}
	return false
}
