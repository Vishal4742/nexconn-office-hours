package main

import "testing"

// A fake login, joined into the address at run time like the store's tests do, so CI's secret scan
// can stay strict about real addresses in the code.
const fakeLogin = "u:not-a-password"

func TestTarget(t *testing.T) {
	for _, tt := range []struct {
		migrate, database, wantFrom, wantHost string
	}{
		{"postgresql://" + fakeLogin + "@ep-x.ap-southeast-1.aws.neon.tech/db?sslmode=require&channel_binding=require", "postgres://localhost:5432/dev", "MIGRATE_DATABASE_URL", "ep-x.ap-southeast-1.aws.neon.tech"},
		{"", "postgres://localhost:5432/officehours_dev?sslmode=disable", "DATABASE_URL", "localhost"},
		{"", "", "", ""},
		{"", "host=localhost dbname=dev", "DATABASE_URL", "an address that is not a URL"},
	} {
		url, from, host := target(tt.migrate, tt.database)
		if from != tt.wantFrom || host != tt.wantHost {
			t.Errorf("target(%q, %q) = from %q host %q, want %q %q", tt.migrate, tt.database, from, host, tt.wantFrom, tt.wantHost)
		}
		if want := map[string]string{"MIGRATE_DATABASE_URL": tt.migrate, "DATABASE_URL": tt.database, "": ""}[from]; url != want {
			t.Errorf("target(%q, %q) chose the wrong address", tt.migrate, tt.database)
		}
	}
}
