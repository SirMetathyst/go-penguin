package main

import (
	"fmt"
	"github.com/SirMetathyst/go-penguin"
	"github.com/SirMetathyst/go-penguin/_examples/template/embedded"
	"log"
	"net/http"
)

func main() {

	r := penguin.New()

	handle("embed", Embed, r)
	handle("glob", HTMLGlob, r)
	handle("template", HTML, r)

	if err := http.ListenAndServe(":3333", r); err != nil {
		log.Fatalln(err)
	}
}

type HTMLFunc int

const (
	HTMLGlob HTMLFunc = iota
	HTML
	Embed
)

func handle(name string, ft HTMLFunc, r penguin.Router) {

	r.Route(fmt.Sprintf("/%s", name), func(r penguin.Router) {

		switch ft {
		case HTMLGlob:
			r.HTMLGlob("./template/*.tmpl")
			break
		case HTML:
			r.HTML("./template/cat.tmpl", "./template/dog.tmpl", "./template/index.tmpl")
			break
		case Embed:
			r.HTMLFs(embedded.FS, "*.tmpl")
			break
		}

		r.Get("/cat", func(w http.ResponseWriter, r *http.Request) {
			penguin.HTML(w, r, http.StatusOK, "cat.tmpl", penguin.M{"name": "Frederick", "title": name})
		})
		r.Get("/dog", func(w http.ResponseWriter, r *http.Request) {
			penguin.HTML(w, r, http.StatusOK, "cat.tmpl", penguin.M{"name": "Ludwig", "title": name})
		})

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			penguin.HTML(w, r, http.StatusOK, "index.tmpl", penguin.M{
				"title": name,
				"elements": penguin.S{
					penguin.M{"url": fmt.Sprintf("/%s/cat", name), "name": "cat"},
					penguin.M{"url": fmt.Sprintf("/%s/cat", name), "name": "dog"},
				},
			})
		})
	})
}
