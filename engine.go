package penguin

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"sync"
)

var _ Router = &Engine{}

// Engine is a simple HTTP route multiplexer that parses a request path,
// records any URL params, and executes an end handler. It implements
// the http.Handler interface and is friendly with the standard library.
//
// Engine is designed to be fast, minimal and offer a powerful API for building
// modular and composable HTTP services with a large set of handlers. It's
// particularly useful for writing large REST API services that break a handler
// into many smaller parts composed of middlewares and end handlers.
type Engine struct {
	// The computed mux handler made of the chained middleware stack and
	// the tree router
	handler http.Handler

	// The radix trie router
	tree *node

	// Custom method not allowed handler
	methodNotAllowedHandler http.HandlerFunc

	// A reference to the parent mux used by subrouters when mounting
	// to a parent mux
	parent *Engine

	// Routing context pool
	pool *sync.Pool

	// Custom route not found handler
	notFoundHandler http.HandlerFunc

	// The middleware stack
	middlewares []func(http.Handler) http.Handler

	// Controls the behaviour of middleware chain generation when a mux
	// is registered as an inline group inside another mux.
	inline bool
}

// New returns a newly initialized Engine object that implements the Router
// interface.
func New() *Engine {
	mux := &Engine{tree: &node{}, pool: &sync.Pool{}}
	mux.pool.New = func() interface{} {
		return NewRouteContext()
	}
	return mux
}

// ServeHTTP is the single method of the http.Handler interface that makes
// Engine interoperable with the standard library. It uses a sync.Pool to get and
// reuse routing contexts for each request.
func (mx *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Ensure the mux has some routes defined on the mux
	if mx.handler == nil {
		mx.NotFoundHandler().ServeHTTP(w, r)
		return
	}

	// Check if a routing context already exists from a parent router.
	rctx, _ := r.Context().Value(RouteCtxKey).(*Context)
	if rctx != nil {
		mx.handler.ServeHTTP(w, r)
		return
	}

	// Fetch a RouteContext object from the sync pool, and call the computed
	// mx.handler that is comprised of mx.middlewares + mx.routeHTTP.
	// Once the request is finished, reset the routing context and put it back
	// into the pool for reuse from another request.
	rctx = mx.pool.Get().(*Context)
	rctx.Reset()
	rctx.Routes = mx
	rctx.parentCtx = r.Context()

	// NOTE: r.WithContext() causes 2 allocations and context.WithValue() causes 1 allocation
	r = r.WithContext(context.WithValue(r.Context(), RouteCtxKey, rctx))

	// Serve the request and once its done, put the request context back in the sync pool
	mx.handler.ServeHTTP(w, r)
	mx.pool.Put(rctx)
}

// Use appends a middleware handler to the Engine middleware stack.
//
// The middleware stack for any Engine will execute before searching for a matching
// route to a specific handler, which provides opportunity to respond early,
// change the course of the request execution, or set request-scoped values for
// the next http.Handler.
func (mx *Engine) Use(middlewares ...func(http.Handler) http.Handler) {
	if mx.handler != nil {
		panic("chi: all middlewares must be defined before routes on a mux")
	}
	mx.middlewares = append(mx.middlewares, middlewares...)
}

// HTML takes an ExecuteTemplate interface to handle execution of templates.
func (mx *Engine) HTML(handler ExecuteTemplate) {
	mx.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rctx := RouteContext(r.Context())
			rctx.HTMLEngine = handler
			next.ServeHTTP(w, r)
		})
	})
}

// HTMLGlob parses the template definitions in the files identified by the patterns and calls Engine.Use
// with middleware that injects the templates for use by HTML. If the templates fail to parse the method will panic.
func (mx *Engine) HTMLGlob(patterns ...string) {
	var tmpl = template.New("")
	for _, pattern := range patterns {
		tmpl = template.Must(tmpl.ParseGlob(pattern))
	}
	mx.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rctx := RouteContext(r.Context())
			rctx.HTMLEngine = tmpl
			next.ServeHTTP(w, r)
		})
	})
}

// HTMLGlobReloadable parses the template definitions in the files identified by the patterns and calls Engine.Use
// with middleware that injects the templates for use by HTML but will reload and parse the templates with each
// request if reload is set to true. If the templates fail to parse the method will panic.
func (mx *Engine) HTMLGlobReloadable(reload bool, patterns ...string) {
	var tmpl *template.Template
	loadFn := func() {
		tmpl = template.New("")
		for _, pattern := range patterns {
			tmpl = template.Must(tmpl.ParseGlob(pattern))
		}
	}
	loadFn()
	mx.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reload {
				loadFn()
			}
			rctx := RouteContext(r.Context())
			rctx.HTMLEngine = tmpl
			next.ServeHTTP(w, r)
		})
	})
}

// HTMLFs is like Engine.HTMLGlob but reads from the file system fs instead of the host operating system's file system.
// It accepts a list of glob patterns (Note that most file names serve as glob patterns matching only themselves.) and
// will be injected into each request for use by HTML. If the templates fail to parse the method will panic.
func (mx *Engine) HTMLFs(fs fs.FS, patterns ...string) {
	tmpl := template.Must(template.ParseFS(fs, patterns...))
	mx.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rctx := RouteContext(r.Context())
			rctx.HTMLEngine = tmpl
			next.ServeHTTP(w, r)
		})
	})
}

// Static adds a handler using http.FileSystem that serves HTTP requests with the contents of the file system rooted at rootPath.
func (mx *Engine) Static(rootPath string) {
	mx.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(rootPath))))
}

// StaticFS adds a handler using http.FileSystem that serves HTTP requests with the contents of the file system rooted at rootPath.
// fs is converted to a FileSystem implementation, for use with the FileServer.
func (mx *Engine) StaticFS(fs fs.FS) {
	mx.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(fs))))
}

// HTMLFsReloadable is like Engine.HTMLGlob but reads from the file system fs instead of the host operating system's file system.
// It accepts a list of glob patterns (Note that most file names serve as glob patterns matching only themselves.) and
// will be injected into each request for use by HTML. The templates will be reloaded and parsed on each
// request when reload is set to true. If the templates fail to parse the method will panic.
func (mx *Engine) HTMLFsReloadable(reload bool, fs fs.FS, patterns ...string) {
	var tmpl *template.Template
	loadFn := func() {
		tmpl = template.Must(template.ParseFS(fs, patterns...))
	}
	loadFn()
	mx.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reload {
				loadFn()
			}
			rctx := RouteContext(r.Context())
			rctx.HTMLEngine = tmpl
			next.ServeHTTP(w, r)
		})
	})
}

// Handle adds the route `pattern` that matches any http method to
// execute the `handler` http.Handler.
func (mx *Engine) Handle(pattern string, handler http.Handler) {
	mx.handle(mALL, pattern, handler)
}

// HandleFunc adds the route `pattern` that matches any http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) HandleFunc(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mALL, pattern, handlerFn)
}

// Method adds the route `pattern` that matches `method` http method to
// execute the `handler` http.Handler.
func (mx *Engine) Method(method, pattern string, handler http.Handler) {
	m, ok := methodMap[strings.ToUpper(method)]
	if !ok {
		panic(fmt.Sprintf("chi: '%s' http method is not supported.", method))
	}
	mx.handle(m, pattern, handler)
}

// MethodFunc adds the route `pattern` that matches `method` http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) MethodFunc(method, pattern string, handlerFn http.HandlerFunc) {
	mx.Method(method, pattern, handlerFn)
}

// Connect adds the route `pattern` that matches a CONNECT http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Connect(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mCONNECT, pattern, handlerFn)
}

// Delete adds the route `pattern` that matches a DELETE http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Delete(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mDELETE, pattern, handlerFn)
}

// Get adds the route `pattern` that matches a GET http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Get(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mGET, pattern, handlerFn)
}

// Head adds the route `pattern` that matches a HEAD http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Head(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mHEAD, pattern, handlerFn)
}

// Options adds the route `pattern` that matches a OPTIONS http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Options(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mOPTIONS, pattern, handlerFn)
}

// Patch adds the route `pattern` that matches a PATCH http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Patch(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mPATCH, pattern, handlerFn)
}

// Post adds the route `pattern` that matches a POST http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Post(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mPOST, pattern, handlerFn)
}

// Put adds the route `pattern` that matches a PUT http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Put(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mPUT, pattern, handlerFn)
}

// Trace adds the route `pattern` that matches a TRACE http method to
// execute the `handlerFn` http.HandlerFunc.
func (mx *Engine) Trace(pattern string, handlerFn http.HandlerFunc) {
	mx.handle(mTRACE, pattern, handlerFn)
}

// NotFound sets a custom http.HandlerFunc for routing paths that could
// not be found. The default 404 handler is `http.NotFound`.
func (mx *Engine) NotFound(handlerFn http.HandlerFunc) {
	// Build NotFound handler chain
	m := mx
	hFn := handlerFn
	if mx.inline && mx.parent != nil {
		m = mx.parent
		hFn = Chain(mx.middlewares...).HandlerFunc(hFn).ServeHTTP
	}

	// Update the notFoundHandler from this point forward
	m.notFoundHandler = hFn
	m.updateSubRoutes(func(subMux *Engine) {
		if subMux.notFoundHandler == nil {
			subMux.NotFound(hFn)
		}
	})
}

// MethodNotAllowed sets a custom http.HandlerFunc for routing paths where the
// method is unresolved. The default handler returns a 405 with an empty body.
func (mx *Engine) MethodNotAllowed(handlerFn http.HandlerFunc) {
	// Build MethodNotAllowed handler chain
	m := mx
	hFn := handlerFn
	if mx.inline && mx.parent != nil {
		m = mx.parent
		hFn = Chain(mx.middlewares...).HandlerFunc(hFn).ServeHTTP
	}

	// Update the methodNotAllowedHandler from this point forward
	m.methodNotAllowedHandler = hFn
	m.updateSubRoutes(func(subMux *Engine) {
		if subMux.methodNotAllowedHandler == nil {
			subMux.MethodNotAllowed(hFn)
		}
	})
}

// With adds inline middlewares for an endpoint handler.
func (mx *Engine) With(middlewares ...func(http.Handler) http.Handler) Router {
	// Similarly as in handle(), we must build the mux handler once additional
	// middleware registration isn't allowed for this stack, like now.
	if !mx.inline && mx.handler == nil {
		mx.updateRouteHandler()
	}

	// Copy middlewares from parent inline muxs
	var mws Middlewares
	if mx.inline {
		mws = make(Middlewares, len(mx.middlewares))
		copy(mws, mx.middlewares)
	}
	mws = append(mws, middlewares...)

	im := &Engine{
		pool: mx.pool, inline: true, parent: mx, tree: mx.tree, middlewares: mws,
		notFoundHandler: mx.notFoundHandler, methodNotAllowedHandler: mx.methodNotAllowedHandler,
	}

	return im
}

// Group creates a new inline-Engine with a fresh middleware stack. It's useful
// for a group of handlers along the same routing path that use an additional
// set of middlewares. See _examples/.
func (mx *Engine) Group(fn func(r Router)) Router {
	im := mx.With().(*Engine)
	if fn != nil {
		fn(im)
	}
	return im
}

// Route creates a new Engine with a fresh middleware stack and mounts it
// along the `pattern` as a subrouter. Effectively, this is a short-hand
// call to Mount. See _examples/.
func (mx *Engine) Route(pattern string, fn func(r Router)) Router {
	if fn == nil {
		panic(fmt.Sprintf("chi: attempting to Route() a nil subrouter on '%s'", pattern))
	}
	subRouter := New()
	fn(subRouter)
	mx.Mount(pattern, subRouter)
	return subRouter
}

// Mount attaches another http.Handler or chi Router as a subrouter along a routing
// path. It's very useful to split up a large API as many independent routers and
// compose them as a single service using Mount. See _examples/.
//
// Note that Mount() simply sets a wildcard along the `pattern` that will continue
// routing at the `handler`, which in most cases is another chi.Router. As a result,
// if you define two Mount() routes on the exact same pattern the mount will panic.
func (mx *Engine) Mount(pattern string, handler http.Handler) {
	if handler == nil {
		panic(fmt.Sprintf("chi: attempting to Mount() a nil handler on '%s'", pattern))
	}

	// Provide runtime safety for ensuring a pattern isn't mounted on an existing
	// routing pattern.
	if mx.tree.findPattern(pattern+"*") || mx.tree.findPattern(pattern+"/*") {
		panic(fmt.Sprintf("chi: attempting to Mount() a handler on an existing path, '%s'", pattern))
	}

	// Assign sub-Router's with the parent not found & method not allowed handler if not specified.
	subr, ok := handler.(*Engine)
	if ok && subr.notFoundHandler == nil && mx.notFoundHandler != nil {
		subr.NotFound(mx.notFoundHandler)
	}
	if ok && subr.methodNotAllowedHandler == nil && mx.methodNotAllowedHandler != nil {
		subr.MethodNotAllowed(mx.methodNotAllowedHandler)
	}

	mountHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rctx := RouteContext(r.Context())

		// shift the url path past the previous subrouter
		rctx.RoutePath = mx.nextRoutePath(rctx)

		// reset the wildcard URLParam which connects the subrouter
		n := len(rctx.URLParams.Keys) - 1
		if n >= 0 && rctx.URLParams.Keys[n] == "*" && len(rctx.URLParams.Values) > n {
			rctx.URLParams.Values[n] = ""
		}

		handler.ServeHTTP(w, r)
	})

	if pattern == "" || pattern[len(pattern)-1] != '/' {
		mx.handle(mALL|mSTUB, pattern, mountHandler)
		mx.handle(mALL|mSTUB, pattern+"/", mountHandler)
		pattern += "/"
	}

	method := mALL
	subroutes, _ := handler.(Routes)
	if subroutes != nil {
		method |= mSTUB
	}
	n := mx.handle(method, pattern+"*", mountHandler)

	if subroutes != nil {
		n.subroutes = subroutes
	}
}

// Controller is a shorthand for Router.Route("/pattern", (&HomeController{}).Router)
// and makes it clearer that a controller is being used at a glance
func (mx *Engine) Controller(pattern string, c Controller) {
	mx.Route(pattern, func(r Router) {
		c.Router(r)
	})
}

// Routes returns a slice of routing information from the tree,
// useful for traversing available routes of a router.
func (mx *Engine) Routes() []Route {
	return mx.tree.routes()
}

// Middlewares returns a slice of middleware handler functions.
func (mx *Engine) Middlewares() Middlewares {
	return mx.middlewares
}

// Match searches the routing tree for a handler that matches the method/path.
// It's similar to routing a http request, but without executing the handler
// thereafter.
//
// Note: the *Context state is updated during execution, so manage
// the state carefully or make a NewRouteContext().
func (mx *Engine) Match(rctx *Context, method, path string) bool {
	m, ok := methodMap[method]
	if !ok {
		return false
	}

	node, _, h := mx.tree.FindRoute(rctx, m, path)

	if node != nil && node.subroutes != nil {
		rctx.RoutePath = mx.nextRoutePath(rctx)
		return node.subroutes.Match(rctx, method, rctx.RoutePath)
	}

	return h != nil
}

// NotFoundHandler returns the default Engine 404 responder whenever a route
// cannot be found.
func (mx *Engine) NotFoundHandler() http.HandlerFunc {
	if mx.notFoundHandler != nil {
		return mx.notFoundHandler
	}
	return http.NotFound
}

// MethodNotAllowedHandler returns the default Engine 405 responder whenever
// a method cannot be resolved for a route.
func (mx *Engine) MethodNotAllowedHandler() http.HandlerFunc {
	if mx.methodNotAllowedHandler != nil {
		return mx.methodNotAllowedHandler
	}
	return methodNotAllowedHandler
}

// handle registers a http.Handler in the routing tree for a particular http method
// and routing pattern.
func (mx *Engine) handle(method methodTyp, pattern string, handler http.Handler) *node {
	if len(pattern) == 0 || pattern[0] != '/' {
		panic(fmt.Sprintf("chi: routing pattern must begin with '/' in '%s'", pattern))
	}

	// Build the computed routing handler for this routing pattern.
	if !mx.inline && mx.handler == nil {
		mx.updateRouteHandler()
	}

	// Build endpoint handler with inline middlewares for the route
	var h http.Handler
	if mx.inline {
		mx.handler = http.HandlerFunc(mx.routeHTTP)
		h = Chain(mx.middlewares...).Handler(handler)
	} else {
		h = handler
	}

	// Add the endpoint to the tree and return the node
	return mx.tree.InsertRoute(method, pattern, h)
}

// routeHTTP routes a http.Request through the Engine routing tree to serve
// the matching handler for a particular http method.
func (mx *Engine) routeHTTP(w http.ResponseWriter, r *http.Request) {
	// Grab the route context object
	rctx := r.Context().Value(RouteCtxKey).(*Context)

	// The request routing path
	routePath := rctx.RoutePath
	if routePath == "" {
		if r.URL.RawPath != "" {
			routePath = r.URL.RawPath
		} else {
			routePath = r.URL.Path
		}
		if routePath == "" {
			routePath = "/"
		}
	}

	// Check if method is supported by chi
	if rctx.RouteMethod == "" {
		rctx.RouteMethod = r.Method
	}
	method, ok := methodMap[rctx.RouteMethod]
	if !ok {
		mx.MethodNotAllowedHandler().ServeHTTP(w, r)
		return
	}

	// Find the route
	if _, _, h := mx.tree.FindRoute(rctx, method, routePath); h != nil {
		h.ServeHTTP(w, r)
		return
	}
	if rctx.methodNotAllowed {
		mx.MethodNotAllowedHandler().ServeHTTP(w, r)
	} else {
		mx.NotFoundHandler().ServeHTTP(w, r)
	}
}

func (mx *Engine) nextRoutePath(rctx *Context) string {
	routePath := "/"
	nx := len(rctx.routeParams.Keys) - 1 // index of last param in list
	if nx >= 0 && rctx.routeParams.Keys[nx] == "*" && len(rctx.routeParams.Values) > nx {
		routePath = "/" + rctx.routeParams.Values[nx]
	}
	return routePath
}

// Recursively update data on child routers.
func (mx *Engine) updateSubRoutes(fn func(subMux *Engine)) {
	for _, r := range mx.tree.routes() {
		subMux, ok := r.SubRoutes.(*Engine)
		if !ok {
			continue
		}
		fn(subMux)
	}
}

// updateRouteHandler builds the single mux handler that is a chain of the middleware
// stack, as defined by calls to Use(), and the tree router (Engine) itself. After this
// point, no other middlewares can be registered on this Engine's stack. But you can still
// compose additional middlewares via Group()'s or using a chained middleware handler.
func (mx *Engine) updateRouteHandler() {
	mx.handler = chain(mx.middlewares, http.HandlerFunc(mx.routeHTTP))
}

// methodNotAllowedHandler is a helper function to respond with a 405,
// method not allowed.
func methodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(405)
	w.Write(nil)
}
