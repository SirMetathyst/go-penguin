package penguin

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
)

type M map[string]any

type S []any

type ExecuteTemplate interface {
	ExecuteTemplate(w io.Writer, name string, data any) error
}

// HTML writes a string to the response, setting the Content-Type as text/template.
func HTML(w http.ResponseWriter, r *http.Request, status int, name string, v any) {
	if renderer := HTMLEngineFromCtx(r.Context()); renderer != nil {
		var buf bytes.Buffer
		if err := renderer.ExecuteTemplate(&buf, name, v); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/template; charset=utf-8")
		w.WriteHeader(status)
		_, _ = buf.WriteTo(w)
		return
	}
	panic("penguin: template renderer not assigned")
}

// XML marshals 'v' to JSON, setting the Content-Type as application/xml. It
// will automatically prepend a generic XML header (see encoding/xml.Header) if
// one is not found in the first 100 bytes of 'v'.
func XML(w http.ResponseWriter, r *http.Request, status int, v any) {
	b, err := xml.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)

	// Try to find <?xml header in first 100 bytes (just in case there're some XML comments).
	findHeaderUntil := len(b)
	if findHeaderUntil > 100 {
		findHeaderUntil = 100
	}
	if !bytes.Contains(b[:findHeaderUntil], []byte("<?xml")) {
		// No header found. Print it out first.
		_, _ = w.Write([]byte(xml.Header))
	}

	_, _ = w.Write(b)
}

// NoContent returns a HTTP 204 "No Content" response.
func NoContent(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// Text writes a string to the response, setting the Content-Type as
// text/plain.
func Text(w http.ResponseWriter, r *http.Request, status int, v string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(v))
}

// JSON marshals 'v' to JSON, automatically escaping HTML and setting the
// Content-Type as application/json.
func JSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// PureJSON marshals 'v' to JSON, setting the
// Content-Type as application/json and without escaping HTML
func PureJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// Data writes raw bytes to the response, setting the Content-Type as
// application/octet-stream.
func Data(w http.ResponseWriter, r *http.Request, status int, v []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(status)
	_, _ = w.Write(v)
}
