package main

import (
	"database/sql"
	"github.com/joho/godotenv"
	"log"
	"os"

	"beta-payment-api-client/internal/migration"
	_ "github.com/lib/pq"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatal("Missing required environment variables")
	}

	if len(os.Args) < 2 {
		log.Fatal("Usage: go run cmd/migrate.go [up|down]")
	}
	command := os.Args[1]

	// Read config from env
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbSSLMode := os.Getenv("DB_SSLMODE")

	if dbHost == "" || dbUser == "" || dbPassword == "" || dbName == "" {
		log.Fatal("Missing required environment variables")
	}

	dsn := "host=" + dbHost +
		" port=" + dbPort +
		" user=" + dbUser +
		" password=" + dbPassword +
		" dbname=" + dbName +
		" sslmode=" + dbSSLMode

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Migrate
	dir := "migration"
	switch command {
	case "up":
		migration.MigrateUp(db, dir)
	case "down":
		migration.MigrateDown(db, dir)
	default:
		log.Fatalf("Unknown command: %s", command)
	}
}
