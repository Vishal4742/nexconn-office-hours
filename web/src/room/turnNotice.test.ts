import { afterEach, describe, expect, it, vi } from "vitest";

import { ana, hand, mia } from "../queue/fixtures";
import { notifyState, showTurnNotice } from "./turnNotice";

type Shown = { title: string; options: NotificationOptions };

/** A Notification constructor that records what it was asked to show. */
function fakeNotifications(permission: NotificationPermission) {
  const shown: Shown[] = [];
  class FakeNotification {
    static permission = permission;
    onclick: (() => void) | null = null;
    constructor(title: string, options: NotificationOptions) {
      shown.push({ title, options });
    }
    close() {}
  }
  vi.stubGlobal("Notification", FakeNotification);
  return shown;
}

const question = hand("200", ana, { title: "Can I pin a version?" });

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("showTurnNotice", () => {
  it("FR-NOTIF-2: tells a page in the background that it's my turn, and who answers", () => {
    const shown = fakeNotifications("granted");
    vi.stubGlobal("document", { visibilityState: "hidden" });

    showTurnNotice(
      { kind: "turn", hand: { ...question, takenBy: mia } },
      "acme/widgets",
    );

    expect(shown).toEqual([
      {
        title: "It's your turn in acme/widgets",
        options: {
          body: "@mia-k is answering: Can I pin a version?",
          tag: "office-hours-turn",
        },
      },
    ]);
  });

  it("says I'm next under the same tag, so the turn notice replaces it", () => {
    const shown = fakeNotifications("granted");
    vi.stubGlobal("document", { visibilityState: "hidden" });

    showTurnNotice({ kind: "next", hand: question }, "acme/widgets");

    expect(shown[0].title).toBe("You're next in acme/widgets");
    expect(shown[0].options.tag).toBe("office-hours-turn");
  });

  it("shows nothing while the page is in front: the banner says it there", () => {
    const shown = fakeNotifications("granted");
    vi.stubGlobal("document", { visibilityState: "visible" });

    showTurnNotice({ kind: "turn", hand: question }, "acme/widgets");

    expect(shown).toEqual([]);
  });

  it("shows nothing without permission", () => {
    const shown = fakeNotifications("default");
    vi.stubGlobal("document", { visibilityState: "hidden" });

    showTurnNotice({ kind: "turn", hand: question }, "acme/widgets");

    expect(shown).toEqual([]);
  });

  it("never throws where pages may not show notifications (Chrome on Android)", () => {
    class Refusing {
      static permission = "granted";
      constructor() {
        throw new TypeError("Illegal constructor");
      }
    }
    vi.stubGlobal("Notification", Refusing);
    vi.stubGlobal("document", { visibilityState: "hidden" });

    expect(() =>
      showTurnNotice({ kind: "turn", hand: question }, "acme/widgets"),
    ).not.toThrow();
  });
});

describe("notifyState", () => {
  it("is unsupported where the browser has no notifications", () => {
    vi.stubGlobal("Notification", undefined);
    expect(notifyState()).toBe("unsupported");
  });

  it("is the browser's permission otherwise", () => {
    fakeNotifications("denied");
    expect(notifyState()).toBe("denied");
  });
});
