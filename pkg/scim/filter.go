package scim

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	// maxFilterLength bounds the filters this server parses.
	maxFilterLength = 4096
)

// filter is a parsed SCIM filter expression, as RFC 7644 section 3.4.2.2 defines it. Attribute names and
// operators are case-insensitive. Filters are never translated into SQL: list filters are matched against a
// fixed set of supported shapes, and PATCH value filters are evaluated in memory.
type filter interface {
	matches(schema *attribute, value map[string]any) bool
}

type attrPath struct {
	// URN is the schema URN prefix, if the path had one.
	URN  string
	Name string
	Sub  string
}

type comparison struct {
	Path     attrPath
	Operator string
	Value    any
}

type logical struct {
	Operator    string
	Left, Right filter
}

type not struct {
	Inner filter
}

type valuePath struct {
	Path   attrPath
	Filter filter
}

type filterParser struct {
	tokens []string
	pos    int
}

// listFilter is a filter that a list request supports: equality on one attribute.
type listFilter struct {
	Attribute string
	Value     string
}

// parseFilter parses a SCIM filter expression.
func parseFilter(s string) (filter, error) {
	if len(s) > maxFilterLength {
		return nil, badRequest(scimTypeInvalidFilter, "filter is longer than %d characters", maxFilterLength)
	}

	tokens, err := tokenizeFilter(s)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, badRequest(scimTypeInvalidFilter, "filter is empty")
	}

	p := &filterParser{
		tokens: tokens,
	}
	f, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.tokens) {
		return nil, badRequest(scimTypeInvalidFilter, "unexpected %q in filter", p.tokens[p.pos])
	}
	return f, nil
}

// tokenizeFilter splits a filter into parentheses, brackets, quoted strings, and words.
func tokenizeFilter(s string) ([]string, error) {
	var tokens []string
	for i := 0; i < len(s); {
		switch c := s[i]; c {
		case ' ', '\t', '\n', '\r':
			i++
		case '(', ')', '[', ']':
			tokens = append(tokens, string(c))
			i++
		case '"':
			end := i + 1
			for ; end < len(s); end++ {
				if s[end] == '\\' {
					end++
					continue
				}
				if s[end] == '"' {
					break
				}
			}
			if end >= len(s) {
				return nil, badRequest(scimTypeInvalidFilter, "unterminated string in filter")
			}
			tokens = append(tokens, s[i:end+1])
			i = end + 1
		default:
			end := i
			for end < len(s) && !strings.ContainsRune(" \t\n\r()[]\"", rune(s[end])) {
				end++
			}
			tokens = append(tokens, s[i:end])
			i = end
		}
	}
	return tokens, nil
}

func (p *filterParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *filterParser) next() string {
	t := p.peek()
	if t != "" {
		p.pos++
	}
	return t
}

func (p *filterParser) expect(token string) error {
	if t := p.next(); t != token {
		if t == "" {
			return badRequest(scimTypeInvalidFilter, "expected %q at end of filter", token)
		}
		return badRequest(scimTypeInvalidFilter, "expected %q in filter, found %q", token, t)
	}
	return nil
}

func (p *filterParser) parseOr() (filter, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for strings.EqualFold(p.peek(), "or") {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = logical{
			Operator: "or",
			Left:     left,
			Right:    right,
		}
	}
	return left, nil
}

func (p *filterParser) parseAnd() (filter, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for strings.EqualFold(p.peek(), "and") {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = logical{
			Operator: "and",
			Left:     left,
			Right:    right,
		}
	}
	return left, nil
}

func (p *filterParser) parseUnary() (filter, error) {
	switch t := p.peek(); {
	case strings.EqualFold(t, "not"):
		p.next()
		if err := p.expect("("); err != nil {
			return nil, err
		}
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		return not{
			Inner: inner,
		}, nil
	case t == "(":
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		return inner, nil
	case t == "":
		return nil, badRequest(scimTypeInvalidFilter, "filter ends unexpectedly")
	default:
		return p.parseAttributeExpression()
	}
}

func (p *filterParser) parseAttributeExpression() (filter, error) {
	token := p.next()
	path, ok := parseAttrPath(token)
	if !ok {
		return nil, badRequest(scimTypeInvalidFilter, "invalid attribute path %q in filter", token)
	}

	if p.peek() == "[" {
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if err := p.expect("]"); err != nil {
			return nil, err
		}
		if path.Sub != "" {
			return nil, badRequest(scimTypeInvalidFilter, "a value filter must follow a complex attribute")
		}
		return valuePath{
			Path:   path,
			Filter: inner,
		}, nil
	}

	op := strings.ToLower(p.next())
	switch op {
	case "pr":
		return comparison{
			Path:     path,
			Operator: op,
		}, nil
	case "eq", "ne", "co", "sw", "ew", "gt", "ge", "lt", "le":
		value, err := parseFilterValue(p.next())
		if err != nil {
			return nil, err
		}
		return comparison{
			Path:     path,
			Operator: op,
			Value:    value,
		}, nil
	case "":
		return nil, badRequest(scimTypeInvalidFilter, "filter ends after %q", path.String())
	default:
		return nil, badRequest(scimTypeInvalidFilter, "unsupported filter operator %q", op)
	}
}

// parseFilterValue parses a comparison value: a quoted string, a number, true, false, or null.
func parseFilterValue(token string) (any, error) {
	switch {
	case token == "":
		return nil, badRequest(scimTypeInvalidFilter, "filter ends before a comparison value")
	case strings.HasPrefix(token, `"`):
		var s string
		if err := json.Unmarshal([]byte(token), &s); err != nil {
			return nil, badRequest(scimTypeInvalidFilter, "invalid string %s in filter", token)
		}
		return s, nil
	case strings.EqualFold(token, "true"):
		return true, nil
	case strings.EqualFold(token, "false"):
		return false, nil
	case strings.EqualFold(token, "null"):
		return nil, nil
	}

	f, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return nil, badRequest(scimTypeInvalidFilter, "invalid comparison value %q in filter", token)
	}
	return f, nil
}

// parseAttrPath parses an attribute path: an optional schema URN, an attribute name, and an optional
// sub-attribute name.
func parseAttrPath(s string) (attrPath, bool) {
	var path attrPath
	if strings.HasPrefix(strings.ToLower(s), "urn:") {
		i := strings.LastIndex(s, ":")
		path.URN, s = s[:i], s[i+1:]
	}

	name, sub, hasSub := strings.Cut(s, ".")
	if !validAttributeName(name) || (hasSub && !validAttributeName(sub)) {
		return attrPath{}, false
	}

	path.Name = name
	path.Sub = sub
	return path, true
}

// validAttributeName reports whether s is an ATTRNAME: a letter or "$" followed by letters, digits, "-", or "_".
func validAttributeName(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c == '$' && i == 0:
		case i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '_'):
		default:
			return false
		}
	}
	return true
}

func (p attrPath) String() string {
	s := p.Name
	if p.Sub != "" {
		s += "." + p.Sub
	}
	if p.URN != "" {
		s = p.URN + ":" + s
	}
	return s
}

// inSchema reports whether the path's URN, if any, names the resource's own schema. Extension schemas are not
// supported.
func (p attrPath) inSchema(schema *resourceSchema) bool {
	return p.URN == "" || strings.EqualFold(p.URN, schema.ID)
}

func (c comparison) matches(parent *attribute, value map[string]any) bool {
	actual, attr := lookupSubValue(parent, value, c.Path)
	caseExact := attr != nil && attr.CaseExact

	if c.Operator == "pr" {
		return present(actual)
	}

	if list, ok := actual.([]any); ok {
		for _, item := range list {
			if compare(c.Operator, item, c.Value, caseExact) {
				return true
			}
		}
		return false
	}
	return compare(c.Operator, actual, c.Value, caseExact)
}

func (l logical) matches(parent *attribute, value map[string]any) bool {
	if l.Operator == "and" {
		return l.Left.matches(parent, value) && l.Right.matches(parent, value)
	}
	return l.Left.matches(parent, value) || l.Right.matches(parent, value)
}

func (n not) matches(parent *attribute, value map[string]any) bool {
	return !n.Inner.matches(parent, value)
}

func (v valuePath) matches(parent *attribute, value map[string]any) bool {
	actual, attr := lookupSubValue(parent, value, v.Path)
	items, ok := actual.([]any)
	if !ok {
		if m, ok := actual.(map[string]any); ok {
			items = []any{m}
		}
	}
	for _, item := range items {
		if m, ok := item.(map[string]any); ok && v.Filter.matches(attr, m) {
			return true
		}
	}
	return false
}

// lookupSubValue returns the value a path names within value, which is an element of a complex attribute, and the
// attribute that describes it. Names are compared case-insensitively.
func lookupSubValue(parent *attribute, value map[string]any, path attrPath) (any, *attribute) {
	var attr *attribute
	if parent != nil {
		attr = parent.subAttribute(path.Name)
	}

	v := lookupKey(value, path.Name)
	if path.Sub == "" {
		return v, attr
	}

	m, ok := v.(map[string]any)
	if !ok {
		return nil, nil
	}
	var sub *attribute
	if attr != nil {
		sub = attr.subAttribute(path.Sub)
	}
	return lookupKey(m, path.Sub), sub
}

func lookupKey(m map[string]any, key string) any {
	if v, ok := m[key]; ok {
		return v
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return nil
}

// hasKey reports whether m has key, matched as lookupKey matches it.
func hasKey(m map[string]any, key string) bool {
	if _, ok := m[key]; ok {
		return true
	}
	for k := range m {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}

func present(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

func compare(op string, actual, expected any, caseExact bool) bool {
	switch expected := expected.(type) {
	case nil:
		switch op {
		case "eq":
			return !present(actual)
		case "ne":
			return present(actual)
		}
		return false
	case bool:
		a, ok := actual.(bool)
		switch op {
		case "eq":
			return ok && a == expected
		case "ne":
			return !ok || a != expected
		}
		return false
	case float64:
		a, ok := toFloat(actual)
		if !ok {
			return op == "ne"
		}
		switch op {
		case "eq":
			return a == expected
		case "ne":
			return a != expected
		case "gt":
			return a > expected
		case "ge":
			return a >= expected
		case "lt":
			return a < expected
		case "le":
			return a <= expected
		}
		return false
	case string:
		a, ok := actual.(string)
		if !ok {
			return op == "ne"
		}
		if !caseExact {
			a, expected = strings.ToLower(a), strings.ToLower(expected)
		}
		switch op {
		case "eq":
			return a == expected
		case "ne":
			return a != expected
		case "co":
			return strings.Contains(a, expected)
		case "sw":
			return strings.HasPrefix(a, expected)
		case "ew":
			return strings.HasSuffix(a, expected)
		case "gt":
			return a > expected
		case "ge":
			return a >= expected
		case "lt":
			return a < expected
		case "le":
			return a <= expected
		}
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch v := v.(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case int:
		return float64(v), true
	}
	return 0, false
}

// parseListFilter parses a list request's filter and checks that it is equality on one of the supported
// attributes, compared case-insensitively. Anything else is rejected rather than answered partially.
func parseListFilter(schema *resourceSchema, s string, supported ...string) (*listFilter, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	f, err := parseFilter(s)
	if err != nil {
		return nil, err
	}

	unsupported := badRequest(scimTypeInvalidFilter, "only %s filters are supported", supportedFilters(supported))
	c, ok := f.(comparison)
	if !ok || c.Operator != "eq" || c.Path.Sub != "" || !c.Path.inSchema(schema) {
		return nil, unsupported
	}
	value, ok := c.Value.(string)
	if !ok {
		return nil, unsupported
	}

	for _, name := range supported {
		if strings.EqualFold(c.Path.Name, name) {
			return &listFilter{
				Attribute: name,
				Value:     value,
			}, nil
		}
	}
	return nil, unsupported
}

func supportedFilters(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, `"`+name+` eq"`)
	}
	return strings.Join(quoted, " and ")
}
