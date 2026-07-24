package app

import (
	"aic3-service/internal/lib"
	"log"
	"os"
	"strconv"
)

func ClickHouse() *lib.ClickHouseClient {
	host := os.Getenv("CLICKHOUSE_HOST")
	// fmt.Printf("%v", host)
	if host == "" {
		log.Fatal("CLICKHOUSE_HOST is required")
	}

	portStr := os.Getenv("CLICKHOUSE_PORT")
	if portStr == "" {
		log.Fatal("CLICKHOUSE_PORT is required")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("invalid CLICKHOUSE_PORT: %v", err)
	}

	database := os.Getenv("CLICKHOUSE_DATABASE")
	if database == "" {
		log.Fatal("CLICKHOUSE_DATABASE is required")
	}

	user := os.Getenv("CLICKHOUSE_USER")
	if user == "" {
		log.Fatal("CLICKHOUSE_USER is required")
	}

	password := os.Getenv("CLICKHOUSE_PASSWORD")
	if password == "" {
		log.Fatal("CLICKHOUSE_PASSWORD is required")
	}

	ch, err := lib.NewClickHouseClient(host, port, database, user, password)
	if err != nil {
		log.Fatalf("clickhouse: %v", err)
	}

	return ch
}
