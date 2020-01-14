package main

import (
	"io"
	"net/http"
)

func main() {
	http.ListenAndServe(":12345", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, vs := range r.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}

		defer r.Body.Close()

		_, _ = io.Copy(w, r.Body)
	}))
}
