package main

import (
	"github.com/SirMetathyst/go-penguin"
	"github.com/SirMetathyst/go-penguin/middleware"
	"net/http"
)

func init() {
	penguin.RegisterMethod("LINK")
	penguin.RegisterMethod("UNLINK")
	penguin.RegisterMethod("WOOHOO")
}

func main() {
	r := penguin.New()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})
	r.MethodFunc("LINK", "/link", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom link method"))
	})
	r.MethodFunc("WOOHOO", "/woo", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom woohoo method"))
	})
	r.HandleFunc("/everything", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("capturing all standard http methods, as well as LINK, UNLINK and WOOHOO"))
	})
	http.ListenAndServe(":3333", r)
}
