package main

import "strings"

// The address of a FAQ entry ends in a slug made from its question's title (FR-FAQ-1, FR-FAQ-2).
// Owned by the maintainer-actions part of the parallel build (notes/phase-2-plan.md, worker 4).

const (
	// slugMax leaves room for the "-2", "-3" the store adds to make a slug unique in its project:
	// the whole stays within the 80 characters of slugRe.
	slugMax = 72
	// slugEmpty is the slug of a title with no letter from a to z and no digit in it.
	slugEmpty = "question"
)

// slugOf makes a slug from a title: lower case, the letters a to z and the digits kept, every run
// of anything else one hyphen, no hyphen at either end. A slug longer than slugMax is cut, at the
// end of a word where it has one. The result always matches slugRe.
//
// ponytail: a letter outside a to z is a gap, not its nearest Latin letter: "café" is "caf", and
// a title in a script without Latin letters is "question" (the store numbers them). Fold accents
// with golang.org/x/text/unicode/norm, which the module graph already holds, once a project asks
// its questions in another language.
func slugOf(title string) string {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	slug := strings.Join(words, "-")
	if len(slug) > slugMax {
		end := slugMax
		// A hyphen at slugMax means a word ends right there. An earlier one is where the last
		// whole word ends: the word after it would be cut in two.
		if i := strings.LastIndexByte(slug[:slugMax+1], '-'); i > 0 {
			end = i
		}
		slug = slug[:end]
	}
	if slug == "" {
		return slugEmpty
	}
	return slug
}
