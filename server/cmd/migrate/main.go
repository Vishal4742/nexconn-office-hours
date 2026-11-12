// Command migrate applies the SQL files in server/migrations that the database hasn't seen yet.
//
//	set -a; . ./.env; set +a; go -C server run ./cmd/migrate
//
// It uses MIGRATE_DATABASE_URL when that is set (production, by hand, before merging the code that
// needs the new schema) and DATABASE_URL otherwise. It prints file names, never the URL.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"officehours/migrations"
	"officehours/store"
)

func main() {
	url := os.Getenv("MIGRATE_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		log.Fatal("migrate: set MIGRATE_DATABASE_URL or DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := store.Open(ctx, url)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	defer pool.Close()

	applied, err := store.Migrate(ctx, pool, migrations.FS)
	for _, name := range applied {
		fmt.Println("applied", name)
	}
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if len(applied) == 0 {
		fmt.Println("nothing to apply: the database is up to date")
	}
}
