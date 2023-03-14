package main

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5/middleware"
)

const defaultPort = 3000

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})
	http.ListenAndServe(fmt.Sprintf(":%d", port()), r)
}

func port() int {
	s := os.Getenv("PORT")
	if s == "" {
		return defaultPort
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
