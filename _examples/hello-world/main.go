package main

import (
	"github.com/SirMetathyst/go-penguin"
	"github.com/SirMetathyst/go-penguin/middleware"
	"net/http"
)

func main() {
	r := penguin.New()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})

	http.ListenAndServe(":3333", r)
}
