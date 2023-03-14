package main

import (
	"fmt"
	"github.com/garethjevans/backport/pkg/webhook"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
)

const defaultPort = "3000"

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	controller := webhook.Controller{}

	r.Get("", controller.DefaultHandler)
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

func init() {
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	// LOG_LEVEL not set, let's default to debug
	if !ok {
		lvl = "debug"
	}
	// parse string, this is built-in feature of logrus
	ll, err := logrus.ParseLevel(lvl)
	if err != nil {
		ll = logrus.DebugLevel
	}
	// set global log level
	logrus.SetLevel(ll)
}
