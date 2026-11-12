package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Fake credentials are joined at run time, so CI's secret scan can stay strict about real URLs.
const fakeLogin = "user:s3cretpw"

func TestOpenRejectsABadURLWithoutLeakingIt(t *testing.T) {
	_, err := Open(context.Background(), "postgres://"+fakeLogin+"@host:notaport/db")
	if err == nil {
		t.Fatal("Open(bad url) = nil error, want one")
	}
	if strings.Contains(err.Error(), "s3cretpw") {
		t.Fatalf("NFR-SEC-1: the error text contains the password: %v", err)
	}
}

func TestOpenUsesASmallPoolAndDoesNotConnectYet(t *testing.T) {
	// Nothing listens on port 1: Open must still succeed, because it connects on first use.
	pool, err := Open(context.Background(), "postgres://"+fakeLogin+"@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()
	if got := pool.Config().MaxConns; got != 2 {
		t.Errorf("MaxConns = %d, want 2", got)
	}
	if got := pool.Config().MaxConnIdleTime; got != 10*time.Second {
		t.Errorf("MaxConnIdleTime = %v, want 10s", got)
	}
}

func TestOpenReachesARealDatabase(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run this against a real Postgres")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
