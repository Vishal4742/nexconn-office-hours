import { afterEach, describe, expect, it, vi } from "vitest";

import { rememberMembership, wasMember } from "./memberHint";

const ROOM = { owner: "Vishal4742", repo: "Agentid-Kya-Solana" } as const;

function memoryStorage(): Storage {
  const items = new Map<string, string>();
  return {
    get length() {
      return items.size;
    },
    clear: () => items.clear(),
    getItem: (key) => items.get(key) ?? null,
    key: (index) => [...items.keys()][index] ?? null,
    removeItem: (key) => {
      items.delete(key);
    },
    setItem: (key, value) => {
      items.set(key, String(value));
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("the member hint", () => {
  it("is off for a room this browser was never a member of", () => {
    vi.stubGlobal("localStorage", memoryStorage());

    expect(wasMember(ROOM)).toBe(false);
  });

  it("remembers a member's room, whatever the case of its name", () => {
    vi.stubGlobal("localStorage", memoryStorage());

    rememberMembership(ROOM, true);

    expect(wasMember(ROOM)).toBe(true);
    expect(wasMember({ owner: "vishal4742", repo: "agentid-kya-solana" })).toBe(
      true,
    );
    expect(wasMember({ owner: "Vishal4742", repo: "whiteflow" })).toBe(false);
  });

  it("forgets the room once the visitor is no longer a member", () => {
    vi.stubGlobal("localStorage", memoryStorage());
    rememberMembership(ROOM, true);

    rememberMembership(ROOM, false);

    expect(wasMember(ROOM)).toBe(false);
  });

  it("is off, and never throws, when storage is blocked", () => {
    const blocked = (): never => {
      throw new DOMException("The operation is insecure.", "SecurityError");
    };
    vi.stubGlobal("localStorage", {
      getItem: blocked,
      setItem: blocked,
      removeItem: blocked,
    });

    expect(() => rememberMembership(ROOM, true)).not.toThrow();
    expect(() => rememberMembership(ROOM, false)).not.toThrow();
    expect(wasMember(ROOM)).toBe(false);
  });

  it("is off when there is no storage at all", () => {
    vi.stubGlobal("localStorage", undefined);

    expect(() => rememberMembership(ROOM, true)).not.toThrow();
    expect(wasMember(ROOM)).toBe(false);
  });
});
