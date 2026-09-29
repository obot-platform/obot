package scim

import (
	"maps"
	"reflect"
	"slices"
	"strings"
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
}

// patchPath is a PATCH target: an attribute, an optional value filter, and an optional sub-attribute.
type patchPath struct {
	Attr   attrPath
	Filter filter
	// Sub is the sub-attribute after a value filter, as in emails[type eq "work"].value. Without a filter, the
	// sub-attribute is Attr.Sub.
	Sub string
}

// valueSet holds the values of a multi-valued attribute for membership tests. Values identified by a value
// sub-attribute, such as group members, match on it alone, in constant time. Other values, which are few, match
// when they are equal.
type valueSet struct {
	byValue map[any]struct{}
	others  []any
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
			Op:    op,
			Path:  strings.TrimSpace(path),
			Value: lookupKey(m, "value"),
		})
	}
	return ops, nil
}

// applyPatch applies operations, in order, to resource, which holds canonical attribute names. The caller
// validates the result and commits it once, so a failed operation leaves nothing applied.
func applyPatch(schema *resourceSchema, resource map[string]any, ops []patchOperation) error {
	for _, op := range ops {
		if err := applyOperation(schema, resource, op); err != nil {
			return err
		}
	}
	return nil
}

func applyOperation(schema *resourceSchema, resource map[string]any, op patchOperation) error {
	if op.Path == "" {
		if op.Op == patchRemove {
			return badRequest(scimTypeNoTarget, "remove requires a path")
		}

		values, ok := op.Value.(map[string]any)
		if !ok {
			return badRequest(scimTypeInvalidValue, "an %s without a path requires an object value", op.Op)
		}

		// Each member of the value is applied as though it were the path. Clients echo read-only attributes
		// such as id, and send attributes this server does not support, so both are ignored.
		for key, value := range values {
			if strings.EqualFold(key, schema.ID) {
				if nested, ok := value.(map[string]any); ok {
					if err := applyOperation(schema, resource, patchOperation{
						Op:    op.Op,
						Value: nested,
					}); err != nil {
						return err
					}
				}
				continue
			}

			path, err := parsePatchPath(key)
			if err != nil || !path.Attr.inSchema(schema) {
				continue
			}
			attr := schema.attribute(path.Attr.Name)
			if attr == nil || attr.Mutability == mutabilityReadOnly {
				continue
			}
			if err := applyToPath(resource, attr, path, op.Op, value); err != nil {
				return err
			}
		}
		return nil
	}

	path, err := parsePatchPath(op.Path)
	if err != nil {
		return err
	}
	if !path.Attr.inSchema(schema) {
		return badRequest(scimTypeInvalidPath, "attribute path %q is not in the %s schema", op.Path, schema.Name)
	}
	attr := schema.attribute(path.Attr.Name)
	if attr == nil {
		return badRequest(scimTypeInvalidPath, "unknown attribute %q", path.Attr.Name)
	}
	if attr.Mutability == mutabilityReadOnly {
		return badRequest(scimTypeMutability, "attribute %q is read-only", attr.Name)
	}
	if op.Op != patchRemove && op.Value == nil {
		return badRequest(scimTypeInvalidValue, "%s requires a value", op.Op)
	}
	return applyToPath(resource, attr, path, op.Op, op.Value)
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
		return patchPath{}, badRequest(scimTypeInvalidPath, "invalid value filter in path %q: %s", s, err.(*Error).Detail)
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

func applyToPath(resource map[string]any, attr *attribute, path patchPath, op string, value any) error {
	switch {
	case path.Filter != nil:
		if !attr.MultiValued || attr.Type != typeComplex {
			return badRequest(scimTypeInvalidPath, "a value filter applies only to a multi-valued complex attribute, not %q", attr.Name)
		}
		return applyFiltered(resource, attr, path, op, value)
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

func applyFiltered(resource map[string]any, attr *attribute, path patchPath, op string, value any) error {
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
		if len(matched) == 0 {
			if sub == nil {
				if op == patchAdd {
					return applyToAttribute(resource, attr, patchAdd, value)
				}
				return badRequest(scimTypeNoTarget, "no value of %q matches the filter", attr.Name)
			}
			// A sub-attribute of a value selected by equality creates the value, as clients do for
			// emails[type eq "work"].value.
			created, ok := valueFromEqualityFilter(attr, path.Filter)
			if !ok {
				return badRequest(scimTypeNoTarget, "no value of %q matches the filter", attr.Name)
			}
			values = append(values, created)
			matched = []int{len(values) - 1}
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
		setList(resource, attr, values)
		return nil
	}
	return nil
}

// immutableError reports a change to an immutable sub-attribute of an existing value, such as the value of a group
// member. Such values can only be added and removed.
func immutableError(attr, sub *attribute) *Error {
	return badRequest(scimTypeMutability, "attribute %q is immutable; add or remove the %s value instead", attr.Name+"."+sub.Name, attr.Name)
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
// value sub-attribute.
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
			i = slices.IndexFunc(existing, func(e any) bool { return equalIgnoringPrimary(e, v) })
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

// equalIgnoringPrimary reports whether two values of a multi-valued attribute are equal apart from their primary
// flags.
func equalIgnoringPrimary(a, b any) bool {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if !aok || !bok {
		return reflect.DeepEqual(a, b)
	}
	am, bm = maps.Clone(am), maps.Clone(bm)
	delete(am, "primary")
	delete(bm, "primary")
	return reflect.DeepEqual(am, bm)
}

func newValueSet(attr *attribute, values []any) *valueSet {
	set := &valueSet{}
	if identifiedByValue(attr) {
		set.byValue = make(map[any]struct{}, len(values))
	}
	for _, v := range values {
		set.add(v)
	}
	return set
}

func (s *valueSet) add(v any) {
	if s.byValue == nil {
		s.others = append(s.others, v)
		return
	}
	if m, ok := v.(map[string]any); ok && m["value"] != nil {
		s.byValue[m["value"]] = struct{}{}
	}
}

func (s *valueSet) contains(v any) bool {
	if s.byValue == nil {
		for _, other := range s.others {
			if reflect.DeepEqual(other, v) {
				return true
			}
		}
		return false
	}
	m, ok := v.(map[string]any)
	if !ok || m["value"] == nil {
		return false
	}
	_, found := s.byValue[m["value"]]
	return found
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
