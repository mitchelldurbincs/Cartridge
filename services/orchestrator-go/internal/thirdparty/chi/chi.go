package chi

import (
	"context"
	"net/http"
	"strings"
)

type contextKey struct{}

// Router is the interface exposed by chi for registering routes.
type Router interface {
	Method(method, pattern string, handler http.HandlerFunc)
	Get(pattern string, handler http.HandlerFunc)
	Post(pattern string, handler http.HandlerFunc)
	Route(pattern string, fn func(r Router))
}

type Mux struct {
	routes []route
}

type route struct {
	method   string
	segments []segment
	handler  http.Handler
}

type segment struct {
	literal string
	param   string
}

func NewRouter() *Mux {
	return &Mux{}
}

func (m *Mux) Method(method, pattern string, handler http.HandlerFunc) {
	m.routes = append(m.routes, route{
		method:   strings.ToUpper(method),
		segments: parsePattern(pattern),
		handler:  handler,
	})
}

func (m *Mux) Get(pattern string, handler http.HandlerFunc) {
	m.Method(http.MethodGet, pattern, handler)
}
func (m *Mux) Post(pattern string, handler http.HandlerFunc) {
	m.Method(http.MethodPost, pattern, handler)
}

func (m *Mux) Route(pattern string, fn func(r Router)) {
	base := strings.TrimSuffix(pattern, "/")
	sub := &subRouter{mux: m, base: base}
	fn(sub)
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	for _, rt := range m.routes {
		if rt.method != "*" && rt.method != r.Method {
			continue
		}
		params, ok := match(rt.segments, path)
		if !ok {
			continue
		}
		ctx := context.WithValue(r.Context(), contextKey{}, params)
		rt.handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}
	http.NotFound(w, r)
}

type subRouter struct {
	mux  *Mux
	base string
}

func (sr *subRouter) Method(method, pattern string, handler http.HandlerFunc) {
	full := join(sr.base, pattern)
	sr.mux.Method(method, full, handler)
}

func (sr *subRouter) Get(pattern string, handler http.HandlerFunc) {
	sr.Method(http.MethodGet, pattern, handler)
}
func (sr *subRouter) Post(pattern string, handler http.HandlerFunc) {
	sr.Method(http.MethodPost, pattern, handler)
}

func (sr *subRouter) Route(pattern string, fn func(r Router)) {
	full := join(sr.base, pattern)
	sub := &subRouter{mux: sr.mux, base: full}
	fn(sub)
}

func join(base, pattern string) string {
	if pattern == "" || pattern == "/" {
		return base
	}
	if strings.HasSuffix(base, "/") {
		base = strings.TrimSuffix(base, "/")
	}
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	return base + pattern
}

func parsePattern(pattern string) []segment {
	if pattern == "" {
		return nil
	}
	pattern = strings.Trim(pattern, "/")
	if pattern == "" {
		return []segment{}
	}
	parts := strings.Split(pattern, "/")
	segments := make([]segment, 0, len(parts))
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			segments = append(segments, segment{param: part[1 : len(part)-1]})
			continue
		}
		segments = append(segments, segment{literal: part})
	}
	return segments
}

func match(segments []segment, path string) (map[string]string, bool) {
	path = strings.Trim(path, "/")
	var parts []string
	if path != "" {
		parts = strings.Split(path, "/")
	}
	if len(parts) != len(segments) {
		return nil, false
	}
	params := make(map[string]string)
	for i, seg := range segments {
		part := parts[i]
		if seg.param != "" {
			params[seg.param] = part
			continue
		}
		if seg.literal != part {
			return nil, false
		}
	}
	return params, true
}

// URLParam retrieves a path parameter set by the router.
func URLParam(r *http.Request, key string) string {
	params, _ := r.Context().Value(contextKey{}).(map[string]string)
	return params[key]
}
