// A hand's details are plain text (FR-HAND-1). An address in them becomes a link only when it is
// http or https (NFR-SEC-6); everything else, javascript: included, stays text.

export type Part =
  | { readonly kind: "text"; readonly text: string }
  | { readonly kind: "link"; readonly text: string; readonly href: string };

// From a word boundary, so an address glued to a word stays text.
const ADDRESS = /\bhttps?:\/\/[^\s<>]+/gi;
const TRAILING = ".,;:!?'\"]}";

const count = (text: string, char: string) => text.split(char).length - 1;

/**
 * The address without the punctuation that follows it in a sentence. A closing bracket stays
 * when the address opened one ("…/wiki/Cache_(computing)").
 */
function trimEnd(address: string): string {
  let end = address.length;
  for (;;) {
    const kept = address.slice(0, end);
    const last = kept.at(-1) ?? "";
    const isLoose =
      TRAILING.includes(last) ||
      (last === ")" && count(kept, ")") > count(kept, "("));
    if (!last || !isLoose) return kept;
    end -= 1;
  }
}

/** Where an address really goes, or null when it is not an http or https address. */
function hrefOf(address: string): string | null {
  try {
    const url = new URL(address);
    const isWeb = url.protocol === "http:" || url.protocol === "https:";
    return isWeb && url.hostname ? url.href : null;
  } catch {
    return null;
  }
}

/** The text in parts, each address a link. Joined, the parts' texts are the text given. */
export function linkParts(text: string): readonly Part[] {
  const parts: Part[] = [];
  let from = 0;
  for (const match of text.matchAll(ADDRESS)) {
    const address = trimEnd(match[0]);
    const href = hrefOf(address);
    if (!href) continue;
    if (match.index > from) {
      parts.push({ kind: "text", text: text.slice(from, match.index) });
    }
    parts.push({ kind: "link", text: address, href });
    from = match.index + address.length;
  }
  if (from < text.length) parts.push({ kind: "text", text: text.slice(from) });
  return parts;
}
