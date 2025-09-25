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
	if p == nil {
		return nil, false
	}

	r := req.Clone(req.Context())
	_, pattern := p.m.Handler(r)
	if pattern == "" {
		return nil, false
	}

	// Note: This will reset the path values for this request, otherwise they won't match what we have set in the authorize router.
	p.m.ServeHTTP(nil, r)
	return r.PathValue, true
}
