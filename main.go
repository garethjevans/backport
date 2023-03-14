package main

import (
	"fmt"
	"github.com/garethjevans/backport/pkg/webhook"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"net/http"
	"os"
)

const defaultPort = "3000"

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	controller := webhook.Controller{}

	r.Get("/", controller.DefaultHandler)
	r.Get("/health", controller.Health)
	r.Get("/ready", controller.Ready)

	http.ListenAndServe(fmt.Sprintf(":%s", port()), r)
}

func port() string {
	s := os.Getenv("PORT")
	if s == "" {
		return defaultPort
	}
	return s
}
