package main

import (
	"log"
	"net/http"
	"os"

	"aiac-service/internal/app"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		log.Fatal("SERVER_PORT is required")
	}

	conn, err := app.DB()
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	r := app.AppRouter(conn)

	log.Printf("Starting server on :%v", serverPort)
	if err := http.ListenAndServe(":"+serverPort, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
