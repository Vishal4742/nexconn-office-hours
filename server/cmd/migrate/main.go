// Command migrate applies the SQL files in server/migrations that the database hasn't seen yet.
//
//	set -a; . ./.env; set +a; go -C server run ./cmd/migrate
//
// It uses MIGRATE_DATABASE_URL when that is set (production, by hand, before merging the code that
// needs the new schema) and DATABASE_URL otherwise. It says which of the two it used and the
// database's host, so a fallback to the local database can't pass for a production run, and it
// prints file names. Never the address itself: it holds the password.
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"officehours/migrations"
	"officehours/store"
)

// target picks the address to migrate, says which variable it came from, and names its host.
func target(migrateURL, databaseURL string) (address, from, host string) {
	switch {
	case migrateURL != "":
		address, from = migrateURL, "MIGRATE_DATABASE_URL"
	case databaseURL != "":
		address, from = databaseURL, "DATABASE_URL"
	default:
		return "", "", ""
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" {
		return address, from, "an address that is not a URL"
	}
	return address, from, u.Hostname()
}

func main() {
	address, from, host := target(os.Getenv("MIGRATE_DATABASE_URL"), os.Getenv("DATABASE_URL"))
	if address == "" {
		log.Fatal("migrate: set MIGRATE_DATABASE_URL or DATABASE_URL")
	}
	fmt.Printf("migrating the database at %s (from %s)\n", host, from)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := store.Open(ctx, address)
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
