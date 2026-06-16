package main

import (
	"log"

	"memolang/internal/app"
	"memolang/internal/db"
)

func main() {
	database, err := db.Open("memolang.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	r := app.NewRouter(database)

	log.Println("MemoLang server starting on :8080")
	log.Fatal(r.Run(":8080"))
}

