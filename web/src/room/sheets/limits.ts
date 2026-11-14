// What each field of the sheets accepts, counted the way the server counts (server/validate.go),
// so a sheet can say what is wrong before anything is sent. The server still checks everything.
import { fromLocalInput } from "../../queue/times";

/** A field's length in characters, after cleaning. */
export type Limit = { readonly min: number; readonly max: number };

export const SESSION_TITLE: Limit = { min: 5, max: 80 }; // FR-SESS-1
export const HAND_TITLE: Limit = { min: 10, max: 120 }; // FR-HAND-1
export const HAND_DETAILS: Limit = { min: 0, max: 2000 }; // FR-HAND-1
export const ANSWER: Limit = { min: 20, max: 2000 }; // FR-FAQ-1
export const MAX_LINKS = 5; // FR-FAQ-1
const MAX_LINK_CHARS = 500;

// Exactly what Go's unicode.IsSpace calls space (JavaScript's \s differs in U+0085 and U+FEFF).
const SPACE =
  "[\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000]";
const SPACES = new RegExp(`${SPACE}+`, "g");
const ENDS = new RegExp(`^${SPACE}+|${SPACE}+$`, "g");

/** One line, like the server's cleanLine: every run of white space is one space, none at the ends. */
export function cleanLine(text: string): string {
  return text.replace(SPACES, " ").replace(ENDS, "");
}

/** Like the server's cleanText: line breaks kept and written "\n", white space at the ends gone. */
export function cleanText(text: string): string {
  return text.replace(/\r\n?/g, "\n").replace(ENDS, "");
}

/** Characters as a person sees them, as Go counts runes: an emoji is one, not two. */
export function charCount(text: string): number {
  return [...text].length;
}

/** A count as the sheets write it: 2,000. */
export const amount = (count: number) => count.toLocaleString("en-US");

/** Why cleaned text does not fit its limit, in a sentence for next to the field; null when it fits. */
export function lengthProblem(
  text: string,
  { min, max }: Limit,
): string | null {
  const count = charCount(text);
  if (count < min) {
    return `Write at least ${amount(min)} characters; this has ${amount(count)}.`;
  }
  if (count > max) {
    return `Keep it to ${amount(max)} characters; this has ${amount(count)}.`;
  }
  return null;
}

/**
 * FR-SESS-5: why a planned time from an <input type="datetime-local"> can't be used, by the
 * server's rules (sessionNextTime in server/api_sessions.go); null when it can. `now` is the
 * page's clock in milliseconds.
 */
export function scheduleProblem(value: string, now: number): string | null {
  const at = fromLocalInput(value);
  if (!at) return "Pick a day and a time.";
  if (at.getTime() <= now) return "Pick a time that hasn't passed yet.";
  const yearAway = new Date(now);
  yearAway.setFullYear(yearAway.getFullYear() + 1);
  if (at.getTime() >= yearAway.getTime()) {
    return "Pick a time less than a year away.";
  }
  return null;
}

export type Links = {
  /** Every link once, in the order written. */
  readonly links: readonly string[];
  /** What is wrong with them, said next to the field, or null. */
  readonly problem: string | null;
};

function isWebAddress(line: string): boolean {
  if (!/^https?:\/\/\S+$/i.test(line)) return false;
  try {
    return new URL(line).hostname !== "";
  } catch {
    return false;
  }
}

const quoted = (line: string) =>
  `"${line.length > 40 ? `${line.slice(0, 40)}…` : line}"`;

function linkProblem(line: string): string | null {
  if (!isWebAddress(line)) {
    return `${quoted(line)} is not a link. Put one address per line, starting with http:// or https://.`;
  }
  if (charCount(line) > MAX_LINK_CHARS) {
    return `${quoted(line)} is longer than ${MAX_LINK_CHARS} characters.`;
  }
  return null;
}

/** FR-FAQ-1: the Done sheet's links, one per line. Blank lines are left out and a repeat counts once. */
export function readLinks(text: string): Links {
  const lines = text
    .split(/\r\n|\r|\n/)
    .map(cleanLine)
    .filter(Boolean);
  const links = [...new Set(lines)];
  const wrong = links.map(linkProblem).find((problem) => problem !== null);
  if (wrong) return { links, problem: wrong };
  if (links.length > MAX_LINKS) {
    return {
      links,
      problem: `Add at most ${MAX_LINKS} links; there are ${links.length}.`,
    };
  }
  return { links, problem: null };
}
