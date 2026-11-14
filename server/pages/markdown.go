package pages

import (
	"strconv"
	"strings"
)

// What agents read (FR-FAQ-7): an answer as Markdown, and a project's answers as its llms.txt
// (llmstxt.org). Typed text is text here too (NFR-SEC-6): it can't make a link, an image, code or
// markup, and a question or an answer is quoted line by line, so nothing in it can start a section
// of its own. Every address is ours, built from the project's registered name and a slug.

// llmsExcerptMax is how much of an answer the llms.txt shows after its title, in characters: as
// much as the page's description.
const llmsExcerptMax = 200

// mdText escapes what would make a link, an image, code or HTML in Markdown. A typed backslash is
// escaped too, so it can't undo an escape.
var mdText = strings.NewReplacer(`\`, `\\`, "`", "\\`", "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`).Replace

// mdLineEnds writes every line ending the Unix way: Markdown ends a line at a lone "\r" too.
var mdLineEnds = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace

// mdLine is typed text on one line: every run of spaces and line breaks is one space.
func mdLine(s string) string { return mdText(strings.Join(strings.Fields(s), " ")) }

// mdQuote is typed text quoted line by line. Every line carries the quote's mark, so a heading or
// a list in the text stays inside the quote.
func mdQuote(s string) string {
	lines := strings.Split(mdText(mdLineEnds(s)), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight("> "+line, " ")
	}
	return strings.Join(lines, "\n")
}

// Markdown is the answer as a Markdown document: what its page says, and where to go next.
func (e Entry) Markdown() string {
	var b strings.Builder
	b.WriteString("# " + mdLine(e.Title) + "\n\n")
	b.WriteString("Answered live in the office hours of " + e.Owner + "/" + e.Repo + " on " + e.CreatedAt.UTC().Format(dayLayout))
	if e.Author.Login != "" {
		b.WriteString(" by @" + mdLine(e.Author.Login))
	}
	b.WriteString(".")
	if e.AskedBy > 1 {
		b.WriteString(" Asked by " + strconv.Itoa(e.AskedBy) + " people.")
	}
	b.WriteString("\n\nSource: " + e.URL() + "\n\n")
	if e.Question != "" {
		b.WriteString("## Question\n\n" + mdQuote(e.Question) + "\n\n")
	}
	b.WriteString("## Answer\n\n" + mdQuote(e.Answer) + "\n\n")
	if len(e.Links) > 0 {
		b.WriteString("## Links\n\n")
		for _, link := range e.Links {
			b.WriteString("- " + mdLine(link) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Still stuck?\n\n")
	b.WriteString("- Ask it live in the room: " + e.Origin + e.RoomPath() + "\n")
	b.WriteString("- The room's sessions, as a calendar: " + e.Origin + e.CalendarPath() + "\n")
	b.WriteString("- All the answers of " + e.Owner + "/" + e.Repo + ": " + e.Origin + e.LLMsPath() + "\n")
	return b.String()
}

// LLMsTxt is the project's answers as its llms.txt: what they are, where to ask live, and each
// answer linked to its Markdown, with how it begins.
func (l List) LLMsTxt() string {
	name := l.Owner + "/" + l.Repo
	var b strings.Builder
	b.WriteString("# " + name + " office hours\n\n")
	b.WriteString("> Questions about " + name + " that its maintainers answered live, the newest first. Each link is the answer as Markdown.\n\n")
	b.WriteString("- Ask live in the room: " + l.Origin + l.RoomPath() + "\n")
	b.WriteString("- The room's sessions, as a calendar: " + l.Origin + l.CalendarPath() + "\n\n")
	b.WriteString("## Answers\n\n")
	if len(l.Entries) == 0 {
		b.WriteString("No answers yet.\n")
		return b.String()
	}
	for _, e := range l.Entries {
		b.WriteString("- [" + mdLine(e.Title) + "](" + l.MarkdownURL(e.Slug) + "): " + mdLine(start(e.Answer, llmsExcerptMax)) + "\n")
	}
	if l.Cut {
		b.WriteString("\nThese are the newest " + strconv.Itoa(len(l.Entries)) + " answers. To search all of them, open " + l.ListURL() + "?q= followed by your words.\n")
	}
	return b.String()
}
