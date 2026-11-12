import { describe, expect, it } from "vitest";

import { hand, liveSession, mia, NOW, queues } from "../queue/fixtures";
import type { WatchedRoom } from "../queue/store";
import type { Hand, HandStatus, Queue } from "../queue/types";
import { BOT_ID, describeCard, faqIdOf, isFromBot } from "./cards";
import type { Card, CardContext, CardMessage, Removal } from "./cards";

const ROOM: WatchedRoom = {
  owner: "paperplane",
  repo: "stash",
  groupId: "pabc",
  isMaintainer: false,
};
const AS_MAINTAINER: WatchedRoom = { ...ROOM, isMaintainer: true };
const ORIGIN = "https://officehours.example";
const IGNORED = "Ignored: this card was not sent by Office Hours.";

function fromBot(
  messageType: string,
  content: unknown,
  channelId: string = ROOM.groupId,
): CardMessage {
  return {
    senderUserId: BOT_ID,
    messageType,
    content,
    channelIdentifier: { channelId },
  };
}

/** What the queue store's findHand answers while it holds this queue. */
function finder(queue: Queue | null): (handId: string) => Hand | null {
  return (handId) =>
    [...(queue?.hands ?? []), ...(queue?.done ?? [])].find(
      (held) => held.id === handId,
    ) ?? null;
}

function context(over: Partial<CardContext> = {}): CardContext {
  return {
    room: ROOM,
    findHand: finder(queues.liveWatching),
    origin: ORIGIN,
    removal: { step: "idle" },
    ...over,
  };
}

const texts = (card: Card) => card.lines.map((line) => line.text);
const actions = (card: Card) => card.actions.map((action) => action.text);
const link = (card: Card, text: string) => {
  const found = card.actions.find(
    (action) => action.kind === "link" && action.text === text,
  );
  return found?.kind === "link" ? found : null;
};

/** What a field can hold when it is not what the card expects. */
const NOT_TEXT = [undefined, null, 7, true, {}, ["x"], "", "   "];

describe("who sent a card", () => {
  it("FR-QUEUE-7: only the bot's messages count", () => {
    expect(isFromBot({ senderUserId: BOT_ID })).toBe(true);
    expect(isFromBot({ senderUserId: "gh42" })).toBe(false);
    expect(isFromBot({ senderUserId: "" })).toBe(false);
  });

  it.each([
    [
      "OH:Session",
      { title: "Click https://evil.example", state: "ended", answered: 9 },
    ],
    [
      "OH:Hand",
      { handId: "101", title: "Click https://evil.example", authorLogin: "x" },
    ],
    [
      "OH:FAQ",
      { faqId: "7", slug: "evil", title: "Click here", askedBy: ["mia-k"] },
    ],
  ])(
    "FR-QUEUE-7, NFR-SEC-6: a forged %s card shows none of its content",
    (messageType, content) => {
      const forged = describeCard(
        {
          senderUserId: "gh666",
          messageType,
          content,
          channelIdentifier: { channelId: ROOM.groupId },
        },
        context({ room: AS_MAINTAINER }),
      );
      expect(forged).toEqual({
        lines: [{ kind: "muted", text: IGNORED }],
        actions: [],
        said: "",
      });
    },
  );

  it("FR-QUEUE-7: a forged FAQ card names no entry to remove", () => {
    const content = { faqId: "7", slug: "s", title: "A question" };
    expect(faqIdOf(fromBot("OH:FAQ", content))).toBe("7");
    expect(
      faqIdOf({ senderUserId: "gh666", messageType: "OH:FAQ", content }),
    ).toBe("");
    expect(faqIdOf(fromBot("OH:Hand", content))).toBe("");
    expect(faqIdOf(fromBot("OH:FAQ", { faqId: "7; drop" }))).toBe("");
  });

  it("shows nothing for a type it does not know", () => {
    expect(
      describeCard(fromBot("OH:Unknown", { title: "x" }), context()),
    ).toEqual({ lines: [], actions: [], said: "" });
  });

  it.each(["OH:Session", "OH:Hand", "OH:FAQ"])(
    "NFR-SEC-6: %s survives content that is not what it expects",
    (messageType) => {
      const asked: string[] = [];
      const findHand = (handId: string) => {
        asked.push(handId);
        return null;
      };
      for (const content of [
        null,
        undefined,
        "text",
        42,
        [],
        { title: 7, slug: 7, handId: 7, faqId: {}, askedBy: "mia-k" },
        { title: [], state: [], answered: "9", authorLogin: null },
      ]) {
        const card = describeCard(
          fromBot(messageType, content),
          context({ room: AS_MAINTAINER, findHand }),
        );
        expect(card.lines.every((line) => typeof line.text === "string")).toBe(
          true,
        );
        expect(card.lines.some((line) => line.kind === "title")).toBe(false);
        expect(card.actions).toEqual([]);
      }
      expect(asked).toEqual([]);
    },
  );
});

describe("the session card", () => {
  it("FR-SESS-3: says that a session started, with its title", () => {
    const card = describeCard(
      fromBot("OH:Session", {
        sessionId: "12",
        title: "Thursday office hours",
        state: "live",
        answered: 0,
        carried: 0,
        notReached: 0,
      }),
      context(),
    );
    expect(card).toEqual({
      lines: [
        { kind: "eyebrow", text: "Office hours started" },
        { kind: "title", text: "Thursday office hours" },
      ],
      actions: [],
      said: "",
    });
  });

  it("FR-SESS-4, FR-SESS-7: says how an ended session's hands were left", () => {
    const card = describeCard(
      fromBot("OH:Session", {
        title: "Thursday office hours",
        state: "ended",
        answered: 9,
        carried: 2,
        notReached: 1,
      }),
      context(),
    );
    expect(texts(card)).toEqual([
      "Office hours ended",
      "Thursday office hours",
      "9 answered · 2 carried over · 1 not reached",
    ]);
  });

  it.each([
    [{ answered: 9, carried: 0, notReached: 1 }, "9 answered · 1 not reached"],
    [{ answered: 0, carried: 3, notReached: 0 }, "3 carried over"],
    [{ answered: 1, carried: 0, notReached: 0 }, "1 answered"],
    [{ answered: 0, carried: 0, notReached: 4 }, "4 not reached"],
  ])("FR-SESS-4: leaves out a count of 0 (%o)", (counts, summary) => {
    const card = describeCard(
      fromBot("OH:Session", { title: "Thursday", state: "ended", ...counts }),
      context(),
    );
    expect(texts(card)).toEqual(["Office hours ended", "Thursday", summary]);
  });

  it("FR-SESS-4: an ended session nobody raised a hand in has no counts", () => {
    const card = describeCard(
      fromBot("OH:Session", {
        title: "Thursday",
        state: "ended",
        answered: 0,
        carried: 0,
        notReached: 0,
      }),
      context(),
    );
    expect(texts(card)).toEqual(["Office hours ended", "Thursday"]);
  });

  it("the counts belong to the end: a start shows none", () => {
    const card = describeCard(
      fromBot("OH:Session", { title: "Thursday", state: "live", answered: 9 }),
      context(),
    );
    expect(texts(card)).toEqual(["Office hours started", "Thursday"]);
  });

  it.each([-1, 1.5, "9", Number.NaN, Infinity, null, undefined, 2 ** 60, [3]])(
    "NFR-SEC-6: a count that is not a whole number from 0 up shows nothing (%o)",
    (answered) => {
      const card = describeCard(
        fromBot("OH:Session", {
          title: "Thursday",
          state: "ended",
          answered,
          carried: 2,
          notReached: answered,
        }),
        context(),
      );
      expect(texts(card)).toEqual([
        "Office hours ended",
        "Thursday",
        "2 carried over",
      ]);
    },
  );

  it.each([undefined, "paused", 1, null, "LIVE"])(
    "NFR-SEC-6: a state it does not know says neither started nor ended (%o)",
    (state) => {
      const card = describeCard(
        fromBot("OH:Session", { title: "Thursday", state, answered: 9 }),
        context(),
      );
      expect(texts(card)).toEqual(["Office hours", "Thursday"]);
    },
  );

  it.each([...NOT_TEXT, "x".repeat(301)])(
    "NFR-SEC-6: a title that is not a line of text shows nothing (%o)",
    (title) => {
      const card = describeCard(
        fromBot("OH:Session", { title, state: "live" }),
        context(),
      );
      expect(texts(card)).toEqual(["Office hours started"]);
    },
  );
});

describe("the hand card", () => {
  const raised = (over: Record<string, unknown> = {}) =>
    fromBot("OH:Hand", {
      handId: "101",
      sessionId: "12",
      title: "How do I retry only on 5xx?",
      authorLogin: "diego-ruiz",
      ...over,
    });

  const STATUSES: readonly (readonly [HandStatus, string])[] = [
    ["waiting", "In line"],
    ["answering", "Being answered now"],
    ["answered", "Answered"],
    ["skipped", "Skipped"],
    ["withdrawn", "Withdrawn"],
    ["removed", "Removed by a maintainer"],
    ["not_reached", "Not reached"],
    ["carried", "Carried to the next session"],
    ["merged", "Merged into another question"],
  ];

  it.each(STATUSES)(
    "FR-HAND-5: a hand that is %s says so from the queue",
    (status, text) => {
      const queue: Queue = {
        ...queues.liveEmpty,
        done: [hand("101", mia, { status, title: "", details: "" })],
      };
      const card = describeCard(raised(), context({ findHand: finder(queue) }));
      expect(card.lines[card.lines.length - 1]).toEqual({
        kind: "status",
        text,
        status,
      });
    },
  );

  it("FR-HAND-5: says who raised it, its title and where it stands now", () => {
    expect(describeCard(raised(), context())).toEqual({
      lines: [
        { kind: "muted", text: "@diego-ruiz raised a hand" },
        { kind: "title", text: "How do I retry only on 5xx?" },
        { kind: "status", text: "Being answered now", status: "answering" },
      ],
      actions: [],
      said: "",
    });
    const waiting = describeCard(
      raised({ handId: "102", authorLogin: "ana-ferreira" }),
      context(),
    );
    expect(texts(waiting)).toEqual([
      "@ana-ferreira raised a hand",
      "How do I retry only on 5xx?",
      "In line",
    ]);
  });

  it("FR-HAND-5, FR-FAQ-2: an answered hand links to its FAQ page on our own site", () => {
    const card = describeCard(
      raised({
        handId: "100",
        authorLogin: "liwen",
        url: "https://evil.example",
      }),
      context(),
    );
    expect(texts(card)[2]).toBe("Answered");
    expect(card.actions).toEqual([
      {
        kind: "link",
        text: "Read the answer",
        href: "/p/paperplane/stash/faq/where-are-the-docs-for-custom-cache-keys",
        rel: "noopener",
      },
    ]);
    expect(JSON.stringify(card)).not.toContain("evil.example");
  });

  it("FR-FAQ-6: an answered hand whose entry was removed has no link", () => {
    const queue: Queue = {
      ...queues.liveEmpty,
      done: [hand("100", mia, { status: "answered", faqId: "", faqSlug: "" })],
    };
    const card = describeCard(
      raised({ handId: "100" }),
      context({ findHand: finder(queue) }),
    );
    expect(texts(card)[2]).toBe("Answered");
    expect(card.actions).toEqual([]);
  });

  it.each([
    "../../admin",
    "Has-Upper",
    "a b",
    "a/b",
    "x".repeat(81),
    undefined,
  ])("NFR-SEC-6: a slug of the wrong shape makes no link (%o)", (faqSlug) => {
    const answered = {
      ...hand("100", mia, { status: "answered", faqId: "7" }),
      faqSlug,
    } as Hand;
    const queue: Queue = { ...queues.liveEmpty, done: [answered] };
    const card = describeCard(
      raised({ handId: "100" }),
      context({ findHand: finder(queue) }),
    );
    expect(card.actions).toEqual([]);
  });

  it("FR-MOD-1: a removed hand's card no longer shows its words", () => {
    const card = describeCard(
      raised({ handId: "105", title: "Words a maintainer took out" }),
      context({ findHand: finder(queues.ended) }),
    );
    expect(card).toEqual({
      lines: [
        { kind: "muted", text: "@diego-ruiz raised a hand" },
        {
          kind: "status",
          text: "Removed by a maintainer",
          status: "removed",
        },
      ],
      actions: [],
      said: "",
    });
    expect(JSON.stringify(card)).not.toContain("took out");
  });

  it("FR-HAND-5: a hand the queue does not hold shows no status", () => {
    const noStatus = [
      "@diego-ruiz raised a hand",
      "How do I retry only on 5xx?",
    ];
    // An older session's hand, a queue that has not loaded, and no room at all.
    expect(texts(describeCard(raised({ handId: "9" }), context()))).toEqual(
      noStatus,
    );
    expect(
      texts(describeCard(raised(), context({ findHand: finder(null) }))),
    ).toEqual(noStatus);
    expect(
      texts(
        describeCard(raised(), context({ room: null, findHand: finder(null) })),
      ),
    ).toEqual(noStatus);
  });

  it("FR-SESS-7: a carried hand is in line again once the next session holds it", () => {
    const next: Queue = {
      ...queues.liveEmpty,
      session: { ...liveSession, id: "13", number: 4 },
      hands: [hand("102", mia)],
      serverTime: NOW,
    };
    const before = describeCard(
      raised({ handId: "102" }),
      context({ findHand: finder(queues.ended) }),
    );
    const after = describeCard(
      raised({ handId: "102" }),
      context({ findHand: finder(next) }),
    );
    expect(texts(before)[2]).toBe("Carried to the next session");
    expect(texts(after)[2]).toBe("In line");
  });

  it.each([12, "12abc", "", "-1", "0", "1e3", " 12", "9".repeat(20), null])(
    "NFR-SEC-6: a hand id of the wrong shape is never looked up (%o)",
    (handId) => {
      const asked: string[] = [];
      const card = describeCard(
        raised({ handId }),
        context({
          findHand: (id) => {
            asked.push(id);
            return hand(id, mia, { status: "answering" });
          },
        }),
      );
      expect(asked).toEqual([]);
      expect(texts(card)).toEqual([
        "@diego-ruiz raised a hand",
        "How do I retry only on 5xx?",
      ]);
    },
  );

  it.each(["mia k", "a".repeat(40), "", 7, "<img>", "@mia", "mia_k", null])(
    "NFR-SEC-6: a login of the wrong shape is not shown (%o)",
    (authorLogin) => {
      const card = describeCard(raised({ authorLogin }), context());
      expect(texts(card)).toEqual([
        "A hand was raised",
        "How do I retry only on 5xx?",
        "Being answered now",
      ]);
    },
  );

  it.each([...NOT_TEXT, "x".repeat(301)])(
    "NFR-SEC-6: a title that is not a line of text shows nothing (%o)",
    (title) => {
      const card = describeCard(raised({ title }), context());
      expect(texts(card)).toEqual([
        "@diego-ruiz raised a hand",
        "Being answered now",
      ]);
    },
  );

  it("a status this page does not know yet shows no status", () => {
    const queue: Queue = {
      ...queues.liveEmpty,
      done: [{ ...hand("101", mia), status: "paused" } as unknown as Hand],
    };
    const card = describeCard(raised(), context({ findHand: finder(queue) }));
    expect(texts(card)).toEqual([
      "@diego-ruiz raised a hand",
      "How do I retry only on 5xx?",
    ]);
  });
});

describe("the FAQ card", () => {
  const added = (over: Record<string, unknown> = {}, channelId?: string) =>
    fromBot(
      "OH:FAQ",
      {
        faqId: "7",
        slug: "how-do-i-run-the-tests",
        title: "How do I run the tests?",
        askedBy: ["diego-ruiz"],
        ...over,
      },
      channelId,
    );
  const ANSWER = "/p/paperplane/stash/faq/how-do-i-run-the-tests";

  it("FR-FAQ-2: says what was added, who asked, and links to the answer on our own site", () => {
    const card = describeCard(added(), context());
    expect(card.lines).toEqual([
      { kind: "eyebrow", text: "Added to FAQ" },
      { kind: "title", text: "How do I run the tests?" },
      { kind: "muted", text: "Asked by @diego-ruiz" },
    ]);
    expect(actions(card)).toEqual(["Read the answer", "Open a GitHub issue"]);
    expect(link(card, "Read the answer")).toEqual({
      kind: "link",
      text: "Read the answer",
      href: ANSWER,
      rel: "noopener",
    });
    expect(card.said).toBe("");
  });

  it("NFR-SEC-6: an address inside the message is never used", () => {
    const card = describeCard(
      added({
        url: "https://evil.example/phish",
        href: "javascript:alert(1)",
        link: "//evil.example",
        issueUrl: "https://evil.example/issue",
      }),
      context({ room: AS_MAINTAINER }),
    );
    const said = JSON.stringify(card);
    expect(said).not.toContain("evil.example");
    expect(said).not.toContain("javascript:");
    expect(link(card, "Read the answer")?.href).toBe(ANSWER);
  });

  it("FR-FAQ-5: offers a GitHub issue of the room's repo, filled in, in a new tab that tells GitHub nothing", () => {
    const issue = link(describeCard(added(), context()), "Open a GitHub issue");
    expect(issue?.rel).toBe("noopener noreferrer nofollow");
    const url = new URL(issue?.href ?? "");
    expect(url.origin + url.pathname).toBe(
      "https://github.com/paperplane/stash/issues/new",
    );
    expect(url.searchParams.get("title")).toBe("How do I run the tests?");
    expect(url.searchParams.get("body")).toBe(
      `This question was answered in office hours:\n${ORIGIN}${ANSWER}`,
    );
  });

  it("FR-HAND-8: the issue does not name who asked", () => {
    const issue = link(
      describeCard(
        added({ askedBy: ["diego-ruiz", "ana-ferreira"] }),
        context(),
      ),
      "Open a GitHub issue",
    );
    expect(decodeURIComponent(issue?.href ?? "")).not.toContain("diego-ruiz");
  });

  it.each([
    [["mia-k"], "Asked by @mia-k"],
    [["mia-k", "liwen"], "Asked by @mia-k and @liwen"],
    [
      ["mia-k", "liwen", "ana-ferreira"],
      "Asked by @mia-k, @liwen and @ana-ferreira",
    ],
    [["a", "b", "c", "d"], "Asked by @a, @b and 2 more"],
    [["a", "b", "c", "d", "e", "f"], "Asked by @a, @b and 4 more"],
  ])("FR-HAND-8: names who asked (%o)", (askedBy, sentence) => {
    expect(texts(describeCard(added({ askedBy }), context()))[2]).toBe(
      sentence,
    );
  });

  it.each([
    [undefined, []],
    ["mia-k", []],
    [{ 0: "mia-k" }, []],
    [[], []],
    [[7, null, "mia k", "<b>", ""], []],
    [["mia-k", 7, "not a login", "liwen"], ["Asked by @mia-k and @liwen"]],
    [["mia-k", "mia-k", "liwen"], ["Asked by @mia-k and @liwen"]],
  ])(
    "NFR-SEC-6: only logins of the right shape are named (%o)",
    (askedBy, sentence) => {
      expect(
        texts(describeCard(added({ askedBy }), context())).slice(2),
      ).toEqual(sentence);
    },
  );

  it.each(["pother", "", "PABC", undefined])(
    "NFR-SEC-6: a card of another room gets no link built from this room (%o)",
    (channelId) => {
      const message: CardMessage = {
        ...added(),
        channelIdentifier: channelId === undefined ? undefined : { channelId },
      };
      const card = describeCard(message, context({ room: AS_MAINTAINER }));
      expect(texts(card)).toEqual([
        "Added to FAQ",
        "How do I run the tests?",
        "Asked by @diego-ruiz",
      ]);
      expect(card.actions).toEqual([]);
    },
  );

  it("NFR-SEC-6: no room is watched, so there is nothing to build a link from", () => {
    const card = describeCard(added(), context({ room: null }));
    expect(texts(card)[1]).toBe("How do I run the tests?");
    expect(card.actions).toEqual([]);
  });

  it("NFR-SEC-6: a watched room without a group matches no message", () => {
    const noGroup = { ...AS_MAINTAINER, groupId: "" };
    expect(
      describeCard(added({}, ""), context({ room: noGroup })).actions,
    ).toEqual([]);
  });

  it.each(["../../admin", "Has-Upper", "a b", "a/b", "x".repeat(81), "", 7])(
    "NFR-SEC-6: a slug of the wrong shape makes no link to an answer (%o)",
    (slug) => {
      const card = describeCard(added({ slug }), context());
      expect(actions(card)).toEqual(["Open a GitHub issue"]);
      const body = new URL(
        card.actions[0].kind === "link" ? card.actions[0].href : "",
      ).searchParams.get("body");
      expect(body).toBe("This question was answered in office hours.");
    },
  );

  it.each([...NOT_TEXT, "x".repeat(301)])(
    "NFR-SEC-6: without a title there is no issue to open (%o)",
    (title) => {
      const card = describeCard(added({ title }), context());
      expect(texts(card)).toEqual(["Added to FAQ", "Asked by @diego-ruiz"]);
      expect(actions(card)).toEqual(["Read the answer"]);
    },
  );

  describe("Remove from the FAQ (FR-FAQ-6)", () => {
    const maintainer = (removal: Removal = { step: "idle" }) =>
      context({ room: AS_MAINTAINER, removal });

    it("is offered to a maintainer, after the two links", () => {
      const card = describeCard(added(), maintainer());
      expect(card.actions[2]).toEqual({
        kind: "button",
        text: "Remove from the FAQ",
        does: "ask",
      });
      expect(card.actions).toHaveLength(3);
    });

    it("NFR-SEC-2: is not offered to a member, whatever the card's state says", () => {
      for (const step of ["idle", "asking", "sending", "removed"] as const) {
        const card = describeCard(added(), context({ removal: { step } }));
        expect(actions(card)).toEqual([
          "Read the answer",
          "Open a GitHub issue",
        ]);
        expect(card.said).toBe("");
      }
    });

    it.each([7, "", "7a", "-7", "0", undefined])(
      "NFR-SEC-6: is not offered without an entry id of the right shape (%o)",
      (faqId) => {
        const card = describeCard(added({ faqId }), maintainer());
        expect(actions(card)).toEqual([
          "Read the answer",
          "Open a GitHub issue",
        ]);
      },
    );

    it("asks once inside the card, with the safe answer first", () => {
      const card = describeCard(added(), maintainer({ step: "asking" }));
      expect(texts(card)).toEqual([
        "Added to FAQ",
        "How do I run the tests?",
        "Asked by @diego-ruiz",
        "Remove this answer from the FAQ? Its page will say that it was removed.",
      ]);
      expect(card.actions).toEqual([
        { kind: "button", text: "Keep it", does: "keep" },
        { kind: "button", text: "Remove it", does: "remove" },
      ]);
    });

    it("offers nothing while the server is asked", () => {
      const card = describeCard(added(), maintainer({ step: "sending" }));
      expect(card.actions).toEqual([]);
      expect(card.said).toBe("Removing…");
    });

    it("says that the entry was removed, and no longer links to it", () => {
      const card = describeCard(added(), maintainer({ step: "removed" }));
      expect(texts(card)).toEqual([
        "Added to FAQ",
        "How do I run the tests?",
        "Asked by @diego-ruiz",
      ]);
      expect(card.actions).toEqual([]);
      expect(card.said).toBe(
        "Removed from the FAQ. Its page now says that it was removed.",
      );
    });

    it("shows the server's refusal and lets the maintainer try again", () => {
      const card = describeCard(
        added(),
        maintainer({
          step: "refused",
          message: "Only a maintainer of this project can do that.",
        }),
      );
      expect(card.said).toBe("Only a maintainer of this project can do that.");
      expect(actions(card)).toEqual([
        "Read the answer",
        "Open a GitHub issue",
        "Remove from the FAQ",
      ]);
    });
  });
});
