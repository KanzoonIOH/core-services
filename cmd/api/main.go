package main

import (
	"log"
	"net/http"

	"aiac-service/internal/handler"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/health", handler.Health)

	log.Println("Starting server on :6767")
	if err := http.ListenAndServe(":6767", r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
