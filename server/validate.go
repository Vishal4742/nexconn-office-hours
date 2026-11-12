package main

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// The input rules of the API live in this file (NFR-SEC-3), so no handler invents its own.

// owner/repo must match ^[A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100}$.
var (
	ownerRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)
	repoRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	// Slugs are ours (lower case, digits, hyphens). P3 owns the final rule when it builds them from titles.
	slugRe = regexp.MustCompile(`^[a-z0-9-]{1,80}$`)
)

func validOwnerRepo(owner, repo string) bool {
	// "." and ".." pass the pattern, but GitHub has no such repos and as path parts they mean
	// "here" and "up".
	return ownerRe.MatchString(owner) && repoRe.MatchString(repo) && repo != "." && repo != ".."
}

// cleanLine makes one line of what someone typed: every run of spaces, tabs and line breaks
// becomes one space, and there is none at either end. For titles.
func cleanLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// cleanText trims what someone typed and keeps its line breaks, written the Unix way. For
// details and answers.
func cleanText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
}

// lengthBetween counts characters, not bytes: the limits in the SRS are what a person sees.
func lengthBetween(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return n >= min && n <= max
}

// clip cuts text we did not write (a repo's description) to at most limit characters, never in
// the middle of one.
func clip(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}
