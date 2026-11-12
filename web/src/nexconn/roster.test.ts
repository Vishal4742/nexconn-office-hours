import { beforeEach, describe, expect, it } from "vitest";

import { BOT_ID } from "./cards";
import {
  forgetEverything,
  groupProfiles,
  isKnownMember,
  membersOf,
  rememberRoom,
  strangers,
  userProfiles,
} from "./roster";

const mia = {
  userId: "gh42",
  name: "Mia K",
  avatarUrl: "https://avatars.example/u/42",
};
const quiet = { userId: "gh7", name: "quiet-dev", avatarUrl: "" };

beforeEach(() => forgetEverything());

describe("what Chat UI's hooks answer from, without the network", () => {
  it("FR-PROJ-4: knows the people of a room it was told about, and the bot", () => {
    rememberRoom("pabc", "paperplane/stash", [mia, quiet]);
    expect(userProfiles(["gh42", "gh7", BOT_ID])).toEqual([
      { userId: "gh42", name: "Mia K", avatarUrl: mia.avatarUrl },
      { userId: "gh7", name: "quiet-dev" }, // no avatar: the key is left out, Chat UI uses its own
      { userId: BOT_ID, name: "Office Hours" },
    ]);
    expect(membersOf("pabc").map((m) => m.userId)).toEqual([
      BOT_ID,
      "gh42",
      "gh7",
    ]);
    expect(groupProfiles(["pabc"])).toEqual([
      { groupId: "pabc", name: "paperplane/stash", memberCount: 3 },
    ]);
  });

  it("always has a non-empty name, because Chat UI refuses a profile without one", () => {
    for (const profile of userProfiles(["gh999", ""])) {
      expect(profile.name.length).toBeGreaterThan(0);
    }
    expect(groupProfiles(["pnope"])[0].name.length).toBeGreaterThan(0);
    expect(membersOf("pnope")).toEqual([{ userId: BOT_ID }]);
  });

  it("can tell a sender it has never heard of, so the room can refresh its members", () => {
    rememberRoom("pabc", "paperplane/stash", [mia]);
    expect(isKnownMember("pabc", "gh42")).toBe(true);
    expect(isKnownMember("pabc", BOT_ID)).toBe(true);
    expect(isKnownMember("pabc", "gh7")).toBe(false);
    expect(isKnownMember("pother", "gh42")).toBe(false);
  });

  it("replaces a room's people when told again, and keeps other rooms apart", () => {
    rememberRoom("pabc", "paperplane/stash", [mia]);
    rememberRoom("pxyz", "paperplane/other", [quiet]);
    rememberRoom("pabc", "paperplane/stash", [mia, quiet]);
    expect(membersOf("pabc")).toHaveLength(3);
    expect(membersOf("pxyz").map((m) => m.userId)).toEqual([BOT_ID, "gh7"]);
  });

  it("FR-PROJ-4: lists the senders a room has not heard of, each once", () => {
    rememberRoom("pabc", "paperplane/stash", [mia]);
    const from = (groupId: string, senderUserId: string) => ({
      groupId,
      senderUserId,
    });
    // Someone who joined after the room was opened writes twice; so does a second newcomer.
    expect(
      strangers("pabc", [
        from("pabc", "gh7"),
        from("pabc", "gh42"),
        from("pabc", BOT_ID),
        from("pabc", "gh7"),
        from("pabc", "gh9"),
      ]),
    ).toEqual(["gh7", "gh9"]);
    // Known people and the bot are no reason to ask the server again.
    expect(
      strangers("pabc", [from("pabc", "gh42"), from("pabc", BOT_ID)]),
    ).toEqual([]);
    // A message in another room says nothing about this one.
    expect(strangers("pabc", [from("pxyz", "gh7")])).toEqual([]);
    expect(strangers("pabc", [])).toEqual([]);
  });
});
