package scim

import (
	"net/url"
	"strings"
)

// projection selects the attributes a response returns, from the attributes and excludedAttributes query
// parameters. They apply to every response that returns a resource, as RFC 7644 section 3.9 says, including those of
// creates and updates. The id and schemas attributes are always returned.
type projection struct {
	// selected is set when the attributes parameter selects what is returned, even when it names no attribute the
	// schema defines. Then only the attributes it names are returned.
	selected   bool
	attributes []attrPath
	excluded   []attrPath
}

func parseProjection(schema *resourceSchema, q url.Values) (*projection, error) {
	attributes, err := parseAttributeList(schema, q.Get("attributes"))
	if err != nil {
		return nil, err
	}
	excluded, err := parseAttributeList(schema, q.Get("excludedAttributes"))
	if err != nil {
		return nil, err
	}
	return &projection{
		selected:   strings.TrimSpace(q.Get("attributes")) != "",
		attributes: attributes,
		excluded:   excluded,
	}, nil
}

// parseAttributeList parses a comma-separated list of attribute paths. Attributes that the schema does not
// define are ignored, as they would be absent from the response anyway.
func parseAttributeList(schema *resourceSchema, s string) ([]attrPath, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	var paths []attrPath
	for part := range strings.SplitSeq(s, ",") {
		path, ok := parseAttrPath(strings.TrimSpace(part))
		if !ok {
			return nil, badRequest(scimTypeInvalidValue, "invalid attribute %q", part)
		}
		attr := schema.attribute(path.Name)
		if attr == nil || !path.inSchema(schema) {
			continue
		}
		path.Name = attr.Name
		if path.Sub != "" {
			sub := attr.subAttribute(path.Sub)
			if sub == nil {
				continue
			}
			path.Sub = sub.Name
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// includes reports whether the response returns any part of the top-level attribute.
func (p *projection) includes(name string) bool {
	if p.selected {
		for _, a := range p.attributes {
			if strings.EqualFold(a.Name, name) {
				return true
			}
		}
		return false
	}
	for _, e := range p.excluded {
		if strings.EqualFold(e.Name, name) && e.Sub == "" {
			return false
		}
	}
	return true
}

func (p *projection) apply(resource map[string]any) map[string]any {
	if p.selected {
		out := map[string]any{
			"id":      resource["id"],
			"schemas": resource["schemas"],
		}
		for _, a := range p.attributes {
			v, ok := resource[a.Name]
			if !ok {
				continue
			}
			if a.Sub == "" {
				out[a.Name] = v
				continue
			}
			out[a.Name] = mergeSubAttribute(out[a.Name], v, a.Sub)
		}
		return out
	}

	for _, e := range p.excluded {
		if e.Name == "id" || e.Name == "schemas" {
			continue
		}
		if e.Sub == "" {
			delete(resource, e.Name)
			continue
		}
		switch v := resource[e.Name].(type) {
		case map[string]any:
			delete(v, e.Sub)
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					delete(m, e.Sub)
				}
			}
		}
	}
	return resource
}

// mergeSubAttribute adds the sub-attribute of value, a complex or multi-valued complex attribute, to projected.
func mergeSubAttribute(projected, value any, sub string) any {
	switch v := value.(type) {
	case map[string]any:
		out, _ := projected.(map[string]any)
		if out == nil {
			out = map[string]any{}
		}
		if s, ok := v[sub]; ok {
			out[sub] = s
		}
		return out
	case []any:
		out, _ := projected.([]any)
		if out == nil {
			out = make([]any, len(v))
			for i := range out {
				out[i] = map[string]any{}
			}
		}
		for i, item := range v {
			if m, ok := item.(map[string]any); ok {
				if s, ok := m[sub]; ok {
					out[i].(map[string]any)[sub] = s
				}
			}
		}
		return out
	}
	return projected
}
