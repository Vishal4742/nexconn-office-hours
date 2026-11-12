package main

import (
	"strings"
	"testing"
)

// slugFits fails the test unless slug can be part of a FAQ entry's address: what slugRe accepts,
// at most 72 characters (the store may add "-2", "-3" and must stay within 80), and hyphens only
// between words.
func slugFits(t *testing.T, title, slug string) {
	t.Helper()
	if !slugRe.MatchString(slug) || len(slug) > 72 || strings.Trim(slug, "-") != slug || strings.Contains(slug, "--") {
		t.Errorf("slugOf(%q) = %q: want lower case letters, digits and single hyphens between them, 1 to 72 characters", title, slug)
	}
}

func TestSlugOf(t *testing.T) {
	for _, c := range []struct{ name, title, want string }{
		{"a plain title", "How do I retry only on 5xx?", "how-do-i-retry-only-on-5xx"},
		{"capitals and digits", "HTTP/2 and QUIC in v3.1", "http-2-and-quic-in-v3-1"},
		{"a run of punctuation is one hyphen", "Why does `cache.Get()` -- fail?! (v2)", "why-does-cache-get-fail-v2"},
		{"spaces at both ends", "   Where are the docs?   ", "where-are-the-docs"},
		{"hyphens and underscores at both ends", "--_ retry_policy _--", "retry-policy"},
		{"tabs and line breaks", "first\tsecond\r\nthird", "first-second-third"},
		{"accents: a letter outside a to z is a gap", "Café crème, déjà vu", "caf-cr-me-d-j-vu"},
		{"CJK between Latin words", "キャッシュ cache の設定 config", "cache-config"},
		{"CJK only", "キャッシュの設定方法を教えてください", "question"},
		{"Cyrillic only", "Как настроить повторы?", "question"},
		{"emoji", "🔥 Hot reload 🚀 broken?", "hot-reload-broken"},
		{"only symbols", "?!… — ¿¡ ***", "question"},
		{"only spaces", "   ", "question"},
		{"nothing", "", "question"},
		{"markup", `<script>alert("x")</script>`, "script-alert-x-script"},
		{"a path that climbs", "../../etc/passwd", "etc-passwd"},
		{"bytes that are no text", "retry\xff\xfepolicy", "retry-policy"},
		{"72 characters stay as they are", strings.Repeat("a", 72), strings.Repeat("a", 72)},
		{"one word of 200 characters is cut at 72", strings.Repeat("a", 200), strings.Repeat("a", 72)},
		{"200 characters of words end on a whole word", strings.Repeat("retry ", 34), strings.Repeat("retry-", 11) + "retry"},
		{"a cut inside a word drops that word", "How do I configure the retry policy so that it only applies to idempotent requests?", "how-do-i-configure-the-retry-policy-so-that-it-only-applies-to"},
		{"a cut at the end of a word keeps it", strings.Repeat("abcdefgh ", 7) + "abcdefghi and more", strings.Repeat("abcdefgh-", 7) + "abcdefghi"},
		{"a word too long for what is left is dropped", "go " + strings.Repeat("x", 100), "go"},
	} {
		t.Run("FR-FAQ-1: "+c.name, func(t *testing.T) {
			got := slugOf(c.title)
			if got != c.want {
				t.Errorf("slugOf(%q) = %q, want %q", c.title, got, c.want)
			}
			slugFits(t, c.title, got)
		})
	}
}

// FuzzSlugOf: whatever a title holds, its slug fits a FAQ entry's address (FR-FAQ-1, FR-FAQ-2).
// `go test` runs the seeds; `go -C server test -run '^$' -fuzz FuzzSlugOf -fuzztime 30s .` looks
// for a title that breaks it.
func FuzzSlugOf(f *testing.F) {
	for _, seed := range []string{"How do I retry only on 5xx?", "", "-", "--a--", "キャッシュ", "İ K ß", "a\x00b\xffc", strings.Repeat("ab ", 99), strings.Repeat("-a", 99)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, title string) {
		slugFits(t, title, slugOf(title))
	})
}
