package authz

import "net/http"

type pathMatcher struct {
	m *http.ServeMux
}

func newPathMatcher(paths ...string) *pathMatcher {
	m := http.NewServeMux()
	for _, path := range paths {
		m.Handle(path, (*fake)(nil))
	}
	return &pathMatcher{m: m}
}

type GetVar func(string) string

func (p *pathMatcher) Match(req *http.Request) (GetVar, bool) {
	_, pattern := p.m.Handler(req)
	if pattern == "" {
		return nil, false
	}
	return req.PathValue, true
}
