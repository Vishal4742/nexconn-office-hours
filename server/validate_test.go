package main

import (
	"strings"
	"testing"
)

func TestValidOwnerRepo(t *testing.T) {
	tests := []struct {
		owner, repo string
		want        bool
	}{
		{"paperplane", "stash", true},
		{"a-b", "My.Repo_1-x", true},
		{strings.Repeat("a", 39), strings.Repeat("r", 100), true},
		{"", "stash", false},
		{"paperplane", "", false},
		{strings.Repeat("a", 40), "stash", false},
		{"paperplane", strings.Repeat("r", 101), false},
		{"under_score", "stash", false},
		{"paperplane", "sta sh", false},
		{"paperplane", "st/ash", false},
		{"paperplane", ".", false}, // GitHub has no such repos, and as path parts they mean "here" and "up"
		{"paperplane", "..", false},
		{"paperplane", ".github", true},
	}
	for _, tt := range tests {
		if got := validOwnerRepo(tt.owner, tt.repo); got != tt.want {
			t.Errorf("NFR-SEC-3: validOwnerRepo(%q, %q) = %v, want %v", tt.owner, tt.repo, got, tt.want)
		}
	}
}

func TestClip(t *testing.T) {
	for _, tt := range []struct {
		in    string
		limit int
		want  string
	}{
		{"", 5, ""},
		{"short", 5, "short"},
		{"a tiny HTTP cache", 6, "a tiny"},
		{"naïve café", 5, "naïve"}, // characters, not bytes: "ï" is two bytes
		{"日本語のリポジトリ", 3, "日本語"},
		{"anything", 0, ""},
	} {
		if got := clip(tt.in, tt.limit); got != tt.want {
			t.Errorf("clip(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
		}
	}
}
