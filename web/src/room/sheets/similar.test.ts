import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { FaqEntry } from "../../faq";
import { SIMILAR_WAIT_MS, similarSearch, similarText } from "./similar";

function entry(slug: string): FaqEntry {
  return {
    id: slug,
    slug,
    title: slug,
    question: slug,
    answer: "",
    links: [],
    askedBy: 1,
    createdAt: "2026-11-14T11:38:00.000Z",
    author: "mia-k",
  };
}

type Asked = {
  readonly text: string;
  readonly answer: (entries: readonly FaqEntry[]) => void;
  readonly fail: () => void;
};

/** A search whose answers the test sends when it wants, in any order. */
function fakeSearch() {
  const asked: Asked[] = [];
  const shown: string[][] = [];
  const search = similarSearch(
    (text) =>
      new Promise((resolve, reject) => {
        asked.push({
          text,
          answer: resolve,
          fail: () => reject(new Error("offline")),
        });
      }),
    (entries) => shown.push(entries.map((found) => found.slug)),
  );
  return { asked, shown, search };
}

/** Lets the answers that were sent reach the search. */
const settle = () => vi.advanceTimersByTimeAsync(0);

describe("similarText", () => {
  it("FR-HAND-3: asks nothing while the title is shorter than a title may be", () => {
    expect(similarText("  How   do I?  ", "The details are long enough")).toBe(
      "",
    );
    expect(similarText("How do I x", "")).toBe("How do I x");
  });

  it("FR-FAQ-4: searches the title and the details together, cleaned", () => {
    expect(similarText("  How do I retry? ", " \n ")).toBe("How do I retry?");
    expect(similarText("How do I retry?", "  Only on 5xx.\r\nNot 404. ")).toBe(
      "How do I retry?\nOnly on 5xx.\nNot 404.",
    );
  });
});

describe("similarSearch", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("FR-HAND-3: asks once, 300 ms after the last change", () => {
    const { asked, search } = fakeSearch();
    search.ask("How do I re");
    vi.advanceTimersByTime(200);
    search.ask("How do I retry?");
    vi.advanceTimersByTime(SIMILAR_WAIT_MS - 1);
    expect(asked).toHaveLength(0);
    vi.advanceTimersByTime(1);
    expect(asked.map((one) => one.text)).toEqual(["How do I retry?"]);
  });

  it("shows the answer for the text in the form and drops one that arrives for older text", async () => {
    const { asked, shown, search } = fakeSearch();
    search.ask("How do I retry?");
    vi.advanceTimersByTime(SIMILAR_WAIT_MS);
    search.ask("How do I retry on 5xx?");
    vi.advanceTimersByTime(SIMILAR_WAIT_MS);
    asked[1].answer([entry("retry-on-5xx")]);
    asked[0].answer([entry("retry")]);
    await settle();
    expect(shown).toEqual([["retry-on-5xx"]]);
  });

  it("clears the list at once when the text is too short, and drops the answer still on its way", async () => {
    const { asked, shown, search } = fakeSearch();
    search.ask("How do I retry?");
    vi.advanceTimersByTime(SIMILAR_WAIT_MS);
    search.ask("");
    expect(shown).toEqual([[]]);
    asked[0].answer([entry("retry")]);
    await settle();
    vi.advanceTimersByTime(SIMILAR_WAIT_MS);
    expect(shown).toEqual([[]]);
    expect(asked).toHaveLength(1);
  });

  it("asks and shows nothing more once the sheet has closed", async () => {
    const { asked, shown, search } = fakeSearch();
    search.ask("How do I retry?");
    vi.advanceTimersByTime(SIMILAR_WAIT_MS);
    search.ask("How do I retry on 5xx?");
    search.stop();
    vi.advanceTimersByTime(SIMILAR_WAIT_MS);
    asked[0].answer([entry("retry")]);
    await settle();
    expect(asked).toHaveLength(1);
    expect(shown).toEqual([]);
  });

  it("keeps what it shows when a search fails: the suggestions are a help, not a step", async () => {
    const { asked, shown, search } = fakeSearch();
    search.ask("How do I retry?");
    vi.advanceTimersByTime(SIMILAR_WAIT_MS);
    asked[0].fail();
    await settle();
    expect(shown).toEqual([]);
  });
});
