package main

import (
	"log"
	"os"

	"memolang/internal/app"
	"memolang/internal/db"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "memolang.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	r := app.NewRouter(database)

	log.Println("MemoLang server starting on :8080")
	log.Fatal(r.Run(":8080"))
}

