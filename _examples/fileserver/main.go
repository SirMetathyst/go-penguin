//
// FileServer
// ===========
// This example demonstrates how to serve static files from your filesystem.
//
//
// Boot the server:
// ----------------
// $ go run main._go
//
// Client requests:
// ----------------
// $ curl http://localhost:3333/files/
// <pre>
// <a href="notes.txt">notes.txt</a>
// </pre>
//
// $ curl http://localhost:3333/files/notes.txt
// Notessszzz
//
package main

import (
	"github.com/SirMetathyst/go-penguin/middleware"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirMetathyst/go-penguin"
)

func main() {
	r := penguin.New()
	r.Use(middleware.Logger)

	// Index handler
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	})

	// Create a route along /files that will serve contents from
	// the ./data/ folder.
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "data"))
	FileServer(r, "/files", filesDir)

	http.ListenAndServe(":3333", r)
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r penguin.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := penguin.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
