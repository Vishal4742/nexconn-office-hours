import { describe, expect, it } from "vitest";

import { maintainerKey } from "./keys";

const press = (key: string, more: Partial<KeyboardEventInit> = {}) => ({
  key,
  altKey: false,
  ctrlKey: false,
  metaKey: false,
  repeat: false,
  ...more,
});
const nothing: readonly EventTarget[] = [];

describe("maintainerKey", () => {
  it("N, D, S and A, in either case", () => {
    expect(maintainerKey(press("n"), nothing)).toBe("next");
    expect(maintainerKey(press("D"), nothing)).toBe("done");
    expect(maintainerKey(press("s"), nothing)).toBe("skip");
    expect(maintainerKey(press("a"), nothing)).toBe("away");
    expect(maintainerKey(press("x"), nothing)).toBeNull();
  });

  it("never with a modifier, nor while a key is held down", () => {
    for (const more of [
      { ctrlKey: true },
      { metaKey: true },
      { altKey: true },
      { repeat: true },
    ]) {
      expect(maintainerKey(press("n", more), nothing)).toBeNull();
    }
  });

  it("never while typing or while a sheet is open", () => {
    const tagged = (tagName: string, extra: object = {}) =>
      ({
        tagName,
        isContentEditable: false,
        ...extra,
      }) as unknown as EventTarget;
    for (const target of [
      tagged("INPUT"),
      tagged("TEXTAREA"),
      tagged("SELECT"),
      tagged("DIALOG"),
      tagged("DIV", { isContentEditable: true }),
    ]) {
      expect(maintainerKey(press("n"), [target])).toBeNull();
    }
    expect(maintainerKey(press("n"), [tagged("BUTTON")])).toBe("next");
  });

  it("never while a sheet is open, even when focus has left it for the page", () => {
    // A disabled "Publish" button, or a click on a sheet's text, leaves focus on <body>.
    const body = { tagName: "BODY" } as unknown as EventTarget;
    expect(maintainerKey(press("s"), [body], true)).toBeNull();
    expect(maintainerKey(press("s"), [body], false)).toBe("skip");
  });
});
