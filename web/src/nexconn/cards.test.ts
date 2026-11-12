import { describe, expect, it } from "vitest";

import { BOT_ID, cardLines, isFromBot } from "./cards";

const fromBot = (messageType: string, content: unknown) => ({
  senderUserId: BOT_ID,
  messageType,
  content,
});

describe("OH cards", () => {
  it("FR-QUEUE-7: only the bot's messages count", () => {
    expect(isFromBot({ senderUserId: BOT_ID })).toBe(true);
    expect(isFromBot({ senderUserId: "gh42" })).toBe(false);
    expect(isFromBot({ senderUserId: "" })).toBe(false);
  });

  it("shows a card the bot sent", () => {
    expect(
      cardLines(
        fromBot("OH:Session", {
          title: "Thursday office hours",
          state: "live",
        }),
      ).map((line) => line.text),
    ).toEqual(["Office hours started", "Thursday office hours"]);
    expect(
      cardLines(
        fromBot("OH:Session", {
          title: "Thursday office hours",
          state: "ended",
        }),
      )[0].text,
    ).toBe("Office hours ended");
    expect(
      cardLines(
        fromBot("OH:Hand", {
          title: "How do I retry only on 5xx errors?",
          authorLogin: "diego-ruiz",
        }),
      ).map((line) => line.text),
    ).toEqual([
      "@diego-ruiz raised a hand",
      "How do I retry only on 5xx errors?",
    ]);
  });

  it("NFR-SEC-6: a card from anyone else shows none of its content", () => {
    const forged = cardLines({
      senderUserId: "gh666",
      messageType: "OH:Hand",
      content: { title: "Click https://evil.example", authorLogin: "mia-k" },
    });
    expect(forged).toHaveLength(1);
    expect(forged[0].text).toBe(
      "Ignored: this card was not sent by Office Hours.",
    );
  });

  it("NFR-SEC-6: the FAQ link is built from the slug on our own site, never from a URL in the card", () => {
    const lines = cardLines(
      fromBot("OH:FAQ", {
        title: "How do I run the tests?",
        slug: "how-do-i-run-the-tests",
        url: "https://evil.example/phish",
      }),
    );
    expect(lines.map((line) => line.text)).toEqual([
      "Added to FAQ",
      "How do I run the tests?",
      "/faq/how-do-i-run-the-tests",
    ]);
    expect(JSON.stringify(lines)).not.toContain("evil.example");
  });

  it("survives content that is not what it expects", () => {
    for (const content of [null, undefined, "text", 42, [], { title: 7 }]) {
      const lines = cardLines(fromBot("OH:Hand", content));
      expect(lines.every((line) => typeof line.text === "string")).toBe(true);
    }
    expect(cardLines(fromBot("OH:Unknown", {}))).toEqual([]);
  });
});
