package mux

import (
	"net/http"
	"net/url"
	"strings"
)

type (
	MiddlewareFunc func(HandlerFunc) HandlerFunc
	HandlerFunc    func(w http.ResponseWriter, r *http.Request) error
)

// Mux is a simple wrapper for http.ServeMux. The wrapper is created because
// we don't want to add more dependency as we only need to serve a simple JSON APIs.
type Mux struct {
	// Pattern will be appended to every http method if the pattern is not empty.
	pattern     string
	muxer       *http.ServeMux
	middlewares []MiddlewareFunc // Stack of middlewares.
}

// New returns a new mux object.
func New() *Mux {
	return &Mux{
		muxer:       http.NewServeMux(),
		middlewares: make([]MiddlewareFunc, 0),
	}
}

// Use wraps all handler to and enforce to use the registered middleware.
func (m *Mux) Use(middlewares ...MiddlewareFunc) {
	m.middlewares = append(m.middlewares, middlewares...)
}

func (m *Mux) HandlerFunc(method, pattern string, handler HandlerFunc) {
	m.handlerFunc(strings.ToUpper(method), pattern, handler)
}

func (m *Mux) Get(pattern string, handler HandlerFunc) {
	m.handlerFunc(http.MethodGet, pattern, handler)
}

func (m *Mux) Post(pattern string, handler HandlerFunc) {
	m.handlerFunc(http.MethodPost, pattern, handler)
}

func (m *Mux) Put(pattern string, handler HandlerFunc) {
	m.handlerFunc(http.MethodPut, pattern, handler)
}

func (m *Mux) Patch(pattern string, handler HandlerFunc) {
	m.handlerFunc(http.MethodPatch, pattern, handler)
}

func (m *Mux) Delete(pattern string, handler HandlerFunc) {
	m.handlerFunc(http.MethodDelete, pattern, handler)
}

func (m *Mux) Options(pattern string, handler HandlerFunc) {
	m.handlerFunc(http.MethodOptions, pattern, handler)
}

func (m *Mux) Head(pattern string, handler HandlerFunc) {
	m.handlerFunc(http.MethodHead, pattern, handler)
}

func (m *Mux) Group(fn func(m *Mux)) {
	clone := &Mux{
		muxer:       m.muxer,
		middlewares: m.middlewares,
	}
	fn(clone)
}

// Route handle pattern and assume of it's child route use the same pattern that defined when a route is created.
func (m *Mux) Route(pattern string, fn func(m *Mux)) {
	// Check whether the mux already have a pattern, if this is a route inside a route then we should always
	// append the previous  pattern.
	if m.pattern != "" {
		var err error
		pattern, err = url.JoinPath(m.pattern, pattern)
		if err != nil {
			panic(err)
		}
	}
	clone := &Mux{
		pattern:     pattern,
		muxer:       m.muxer,
		middlewares: m.middlewares,
	}
	fn(clone)
}

func (m *Mux) handlerFunc(method, pattern string, handler HandlerFunc) {
	// If the pattern in mux is not empty, then it should be a pattern inside a Route. Then we should append
	// the route pattern to the pattern that we want to register.
	if m.pattern != "" {
		var err error
		pattern, err = url.JoinPath(m.pattern, pattern)
		if err != nil {
			panic(err)
		}
	}

	// Stack the middleware from the last one to the frist one. But because we are stacking/wrapping them backwards,
	// the first middleware will be the first one to be executed as the (n) middleware will be wrapped with (n-1).
	for i := range m.middlewares {
		handler = m.middlewares[len(m.middlewares)-1-i](handler)
	}

	// Since go v1.22.0 it is now possible to route the handler using "{METHOD} + {pattern}". For example, "GET /v1/some/endpoint".
	// And it also handles the wildcard within pattern like "GET /v1/some/endpoint/{id}".
	// For more information you can look at the documentation: https://pkg.go.dev/net/http#ServeMux.
	pattern = method + " " + pattern
	m.muxer.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		rwDelegator := newResponseWriterDelegator(pattern, w)
		handler(rwDelegator, r)
	})
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.muxer.ServeHTTP(w, r)
}
