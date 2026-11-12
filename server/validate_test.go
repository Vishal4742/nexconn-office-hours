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

func TestCleanLineCleanTextAndLengthBetween(t *testing.T) {
	if got := cleanLine("  How do I\tretry \r\n only on 5xx?  "); got != "How do I retry only on 5xx?" {
		t.Errorf("cleanLine = %q", got)
	}
	if got := cleanText("  first\r\nsecond\rthird\n\n  "); got != "first\nsecond\nthird" {
		t.Errorf("cleanText = %q", got)
	}
	for _, tt := range []struct {
		s        string
		min, max int
		want     bool
	}{
		{"", 0, 5, true},
		{"", 1, 5, false},
		{"abcde", 5, 5, true},
		{"abcdef", 1, 5, false},
		{"naïve", 5, 5, true}, // characters, not bytes
		{"🙂🙂🙂", 3, 3, true},   // an emoji is one character here
		{strings.Repeat("語", 120), 10, 120, true},
		{strings.Repeat("語", 121), 10, 120, false},
	} {
		if got := lengthBetween(tt.s, tt.min, tt.max); got != tt.want {
			t.Errorf("NFR-SEC-3: lengthBetween(%q, %d, %d) = %v, want %v", tt.s, tt.min, tt.max, got, tt.want)
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
