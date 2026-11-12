package pages

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// What the templates do with text someone typed. It is always text: the templates escape it, and
// only the http and https addresses inside it become links (NFR-SEC-6).

// part is a piece of typed text: plain text, or an address the page may link to.
type part struct {
	Text string
	Href string // set when Text is an http or https address
}

// addressRe finds what may be a web address: http:// or https:// at the start of a word, up to a
// space, a quote or an angle bracket.
var addressRe = regexp.MustCompile(`(?i)\bhttps?://[^\s\p{Z}<>"]+`)

// linked cuts typed text into pieces, so a template shows all of it as text and still links its
// web addresses. Nothing is lost or added: the pieces read as the text did.
func linked(text string) []part {
	var parts []part
	at := 0
	for _, m := range addressRe.FindAllStringIndex(text, -1) {
		addr := trimAddress(text[m[0]:m[1]])
		if !webAddress(addr) {
			continue
		}
		if m[0] > at {
			parts = append(parts, part{Text: text[at:m[0]]})
		}
		parts = append(parts, part{Text: addr, Href: addr})
		at = m[0] + len(addr)
	}
	if at < len(text) {
		parts = append(parts, part{Text: text[at:]})
	}
	return parts
}

// trimAddress takes off what ends a sentence rather than an address: a full stop, a question
// mark and the like, and a closing bracket that opened before the address.
func trimAddress(addr string) string {
	for {
		trimmed := strings.TrimRight(addr, ".,:;!?'*")
		for _, pair := range []string{"()", "[]"} {
			if strings.HasSuffix(trimmed, pair[1:]) && strings.Count(trimmed, pair[:1]) < strings.Count(trimmed, pair[1:]) {
				trimmed = trimmed[:len(trimmed)-1]
			}
		}
		if trimmed == addr {
			return addr
		}
		addr = trimmed
	}
}

// webAddress reports whether s is a whole http or https address with a host: the only kind of
// address from outside that a page links to.
func webAddress(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

// ownPath reports whether s is a path on our own origin. A browser reads "//host" and "/\host"
// as another site.
func ownPath(s string) bool {
	return strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") && !strings.HasPrefix(s, `/\`)
}

// start is the beginning of a text on one line, at most limit characters: every run of spaces
// and line breaks is one space, and a text that had to be cut ends in "…", after a whole word
// when there is one to cut after.
func start(text string, limit int) string {
	line := []rune(strings.Join(strings.Fields(text), " "))
	if len(line) <= limit {
		return string(line)
	}
	cut := string(line[:limit-1])
	if i := strings.LastIndexByte(cut, ' '); i > 0 && line[limit-1] != ' ' {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ") + "…"
}

// count says how many there are: "1 answer", "2 answers".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
