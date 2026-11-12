import { describe, expect, it } from "vitest";

import type { Stats } from "../stats";
import { hasNothingYet, statTiles } from "./numbers";

const stats: Stats = {
  sessions: 12,
  handsRaised: 48,
  handsAnswered: 41,
  faqEntries: 37,
  participants: 63,
};
const all = (count: number): Stats => ({
  sessions: count,
  handsRaised: count,
  handsAnswered: count,
  faqEntries: count,
  participants: count,
});

describe("the five numbers", () => {
  it("FR-MET-1: each number with its plain name, in the order of the requirement", () => {
    expect(statTiles(stats, "en")).toEqual([
      { key: "sessions", value: "12", name: "sessions held" },
      { key: "handsRaised", value: "48", name: "hands raised" },
      { key: "handsAnswered", value: "41", name: "hands answered" },
      { key: "faqEntries", value: "37", name: "FAQ entries" },
      { key: "participants", value: "63", name: "participants" },
    ]);
  });

  it("FR-MET-1: one of something is named as one", () => {
    expect(statTiles(all(1), "en").map((tile) => tile.name)).toEqual([
      "session held",
      "hand raised",
      "hand answered",
      "FAQ entry",
      "participant",
    ]);
  });

  it("FR-MET-1: none of something is still a number", () => {
    const tiles = statTiles(all(0), "en");
    expect(tiles.map((tile) => tile.value)).toEqual(["0", "0", "0", "0", "0"]);
    expect(tiles[0].name).toBe("sessions held");
    expect(tiles[3].name).toBe("FAQ entries");
  });

  it("FR-MET-1: a big number is grouped the way the viewer writes numbers", () => {
    const big = { ...stats, handsRaised: 1234567 };
    expect(statTiles(big, "en")[1].value).toBe("1,234,567");
    expect(statTiles(big, "de")[1].value).toBe("1.234.567");
  });
});

describe("nothing to count yet", () => {
  it("FR-MET-1: before the first session there is nothing, whoever has joined", () => {
    expect(hasNothingYet(all(0))).toBe(true);
    // The person who registered a repo is in its room from the start.
    expect(hasNothingYet({ ...all(0), participants: 3 })).toBe(true);
  });

  it("FR-MET-1: one session is something", () => {
    expect(hasNothingYet({ ...all(0), sessions: 1 })).toBe(false);
    expect(hasNothingYet(stats)).toBe(false);
  });
});
