package main

import (
	"github.com/SirMetathyst/go-penguin"
	"net/http"
)

// HomeController serves the home page
type HomeController struct{}

func (s *HomeController) Router(r penguin.Router) {
	r.Get("/", s.Index)
}

func (s *HomeController) Index(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Index"))
}

// LoginController handles login functionality
type LoginController struct{}

func (s *LoginController) Router(r penguin.Router) {
	r.Get("/", s.Index)
}

func (s *LoginController) Index(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("login"))
}

func main() {
	r := penguin.New()

	r.Controller("/", &HomeController{})
	r.Controller("/login", &LoginController{})

	http.ListenAndServe(":3333", r)
}
