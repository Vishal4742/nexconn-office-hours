import { describe, expect, it } from "vitest";

import {
  ANSWER,
  HAND_DETAILS,
  HAND_TITLE,
  SESSION_TITLE,
  charCount,
  cleanLine,
  cleanText,
  lengthProblem,
  readLinks,
  scheduleProblem,
} from "./limits";

const chars = (count: number, char = "a") => char.repeat(count);

describe("cleanLine and cleanText", () => {
  it("make a title one line the way the server does: every run of white space is one space", () => {
    expect(cleanLine("  How do\tI \n retry?  ")).toBe("How do I retry?");
    expect(cleanLine(" no-break　spaces\u0085too ")).toBe(
      "no-break spaces too",
    );
  });

  it("keep the line breaks of details and answers, written the Unix way, and trim the ends", () => {
    expect(cleanText("\r\n  first\r\nsecond\rthird \n\n")).toBe(
      "first\nsecond\nthird",
    );
  });
});

describe("charCount", () => {
  it("counts characters as a person sees them: an emoji is one, as on the server", () => {
    expect(charCount("😀ab")).toBe(3);
    expect(charCount("")).toBe(0);
  });
});

describe("lengthProblem", () => {
  const fields = [
    ["FR-SESS-1: a session's title", SESSION_TITLE, 5, 80],
    ["FR-HAND-1: a hand's title", HAND_TITLE, 10, 120],
    ["FR-FAQ-1: an answer", ANSWER, 20, 2000],
  ] as const;

  for (const [name, limit, min, max] of fields) {
    it(`${name} fits from ${min} to ${max} characters and not one outside`, () => {
      expect(lengthProblem(chars(min - 1), limit)).not.toBeNull();
      expect(lengthProblem(chars(min), limit)).toBeNull();
      expect(lengthProblem(chars(max), limit)).toBeNull();
      expect(lengthProblem(chars(max + 1), limit)).not.toBeNull();
    });
  }

  it("FR-HAND-1: details may be empty and hold at most 2,000 characters", () => {
    expect(lengthProblem("", HAND_DETAILS)).toBeNull();
    expect(lengthProblem(chars(2000), HAND_DETAILS)).toBeNull();
    expect(lengthProblem(chars(2001), HAND_DETAILS)).not.toBeNull();
  });

  it("counts an emoji as one character at the edges", () => {
    expect(lengthProblem(chars(5, "😀"), SESSION_TITLE)).toBeNull();
    expect(lengthProblem(chars(80, "😀"), SESSION_TITLE)).toBeNull();
    expect(lengthProblem(chars(81, "😀"), SESSION_TITLE)).not.toBeNull();
  });

  it("counts what the server keeps: spaces at the ends and runs of spaces do not count", () => {
    expect(lengthProblem(cleanLine("  abcd  "), SESSION_TITLE)).not.toBeNull();
    expect(lengthProblem(cleanLine("ab    cd e"), SESSION_TITLE)).toBeNull();
  });

  it("says how many characters it takes and how many there are", () => {
    expect(lengthProblem("short", HAND_TITLE)).toBe(
      "Write at least 10 characters; this has 5.",
    );
    expect(lengthProblem(chars(2001), ANSWER)).toBe(
      "Keep it to 2,000 characters; this has 2,001.",
    );
  });
});

describe("readLinks", () => {
  it("FR-FAQ-1: reads one link per line and leaves out blank lines", () => {
    expect(
      readLinks("\n https://stash.dev/docs \n\n\nhttp://example.com/a?b=1\n  "),
    ).toEqual({
      links: ["https://stash.dev/docs", "http://example.com/a?b=1"],
      problem: null,
    });
    expect(readLinks("")).toEqual({ links: [], problem: null });
  });

  it("keeps a link written twice once, in its first place", () => {
    expect(
      readLinks("https://a.dev\nhttps://b.dev\r\nhttps://a.dev").links,
    ).toEqual(["https://a.dev", "https://b.dev"]);
  });

  it("refuses a line that is not an http or https address, and names it", () => {
    for (const line of [
      "ftp://files.dev/x",
      "javascript:alert(1)",
      "example.com",
      "https://",
      "https://a.dev https://b.dev",
    ]) {
      const { problem } = readLinks(`https://ok.dev\n${line}`);
      expect(problem, line).toContain(`"${line}"`);
      expect(problem, line).toContain("http://");
    }
  });

  it("refuses more than 5 links, and counts a repeat once", () => {
    const six = [1, 2, 3, 4, 5, 6].map((n) => `https://a.dev/${n}`);
    expect(readLinks(six.join("\n")).problem).toBe(
      "Add at most 5 links; there are 6.",
    );
    const fiveAndRepeat = [...six.slice(0, 5), six[0]].join("\n");
    expect(readLinks(fiveAndRepeat).problem).toBeNull();
  });

  it("refuses a link longer than 500 characters", () => {
    const long = `https://a.dev/${chars(486)}`;
    expect(long).toHaveLength(500);
    expect(readLinks(long).problem).toBeNull();
    expect(readLinks(`${long}b`).problem).toContain("500");
  });
});

describe("scheduleProblem", () => {
  // The viewer's wall clock, as <input type="datetime-local"> holds it: 13 Nov 2026, 12:00.
  const now = new Date(2026, 10, 13, 12, 0).getTime();

  it("FR-SESS-5: accepts a time still to come and less than a year away", () => {
    expect(scheduleProblem("2026-11-13T12:01", now)).toBeNull();
    expect(scheduleProblem("2027-11-13T11:59", now)).toBeNull();
  });

  it("asks for a day and a time when the field is empty or unreadable", () => {
    expect(scheduleProblem("", now)).toBe("Pick a day and a time.");
    expect(scheduleProblem("13/11/2026 18:30", now)).toBe(
      "Pick a day and a time.",
    );
  });

  it("refuses a time that has passed, now included, as the server does", () => {
    expect(scheduleProblem("2026-11-13T12:00", now)).toBe(
      "Pick a time that hasn't passed yet.",
    );
    expect(scheduleProblem("2026-11-12T18:30", now)).toBe(
      "Pick a time that hasn't passed yet.",
    );
  });

  it("refuses a time a year away or more, as the server does", () => {
    expect(scheduleProblem("2027-11-13T12:00", now)).toBe(
      "Pick a time less than a year away.",
    );
  });
});
