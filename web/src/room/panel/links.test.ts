import { describe, expect, it } from "vitest";

import { linkParts } from "./links";

const text = (value: string) => ({ kind: "text", text: value });
const link = (value: string, href = value) => ({
  kind: "link",
  text: value,
  href,
});

describe("linkParts: FR-HAND-1, details are plain text with clickable links", () => {
  it("leaves text without an address as it is, line breaks included", () => {
    expect(linkParts("It retries a 404.\nWhy?")).toEqual([
      text("It retries a 404.\nWhy?"),
    ]);
    expect(linkParts("")).toEqual([]);
  });

  it("makes an http or https address a link", () => {
    expect(linkParts("See https://example.com/a?b=1 for more")).toEqual([
      text("See "),
      link("https://example.com/a?b=1"),
      text(" for more"),
    ]);
    expect(linkParts("a\nhttp://x.dev/y\nb")).toEqual([
      text("a\n"),
      link("http://x.dev/y"),
      text("\nb"),
    ]);
  });

  it("finds every address", () => {
    expect(linkParts("https://a.dev and https://b.dev")).toEqual([
      link("https://a.dev", "https://a.dev/"),
      text(" and "),
      link("https://b.dev", "https://b.dev/"),
    ]);
  });

  it("NFR-SEC-6: every other kind of address stays text", () => {
    for (const value of [
      "javascript:alert(1)",
      "JaVaScRiPt:alert(1)",
      "data:text/html,<b>x</b>",
      "ftp://files.example.com/a",
      "mailto:someone@example.com",
      "file:///etc/passwd",
      "www.example.com",
      "foohttps://evil.example", // glued to a word
      "https://", // nothing after the scheme
    ]) {
      expect(linkParts(value)).toEqual([text(value)]);
    }
  });

  it("leaves the punctuation after an address out of it", () => {
    expect(linkParts("Look at https://example.com/docs.")).toEqual([
      text("Look at "),
      link("https://example.com/docs"),
      text("."),
    ]);
    expect(linkParts("(see https://example.com/x)")).toEqual([
      text("(see "),
      link("https://example.com/x"),
      text(")"),
    ]);
    expect(linkParts('"https://example.com/q", she said')).toEqual([
      text('"'),
      link("https://example.com/q"),
      text('", she said'),
    ]);
  });

  it("keeps a closing bracket that belongs to the address", () => {
    const address = "https://en.wikipedia.org/wiki/Cache_(computing)";
    expect(linkParts(address)).toEqual([link(address)]);
  });

  it("shows the address as it was written and links where it really goes", () => {
    const [part] = linkParts("HTTPS://Example.COM/Path");
    expect(part).toEqual(
      link("HTTPS://Example.COM/Path", "https://example.com/Path"),
    );
  });

  it("gives back every character it was given, in order", () => {
    for (const value of [
      "plain",
      "x https://a.dev/(b) y. https://c.dev, z",
      "https://a.dev\n\nhttps://b.dev",
      "javascript:alert(1) then https://ok.dev",
    ]) {
      expect(
        linkParts(value)
          .map((part) => part.text)
          .join(""),
      ).toBe(value);
    }
  });
});
