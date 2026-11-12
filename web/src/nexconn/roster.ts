// Who is in which room, kept in memory. Chat UI's hooks answer from here and never from the
// network: it clears its member list and awaits the members hook every time a room is selected,
// and it copies names and avatars at that moment. So a room loads its people from our server
// first, tells this module, and only then opens the channel (docs/ARCHITECTURE.md §4).

import { BOT_ID } from "./cards";

export type Person = {
  readonly userId: string;
  readonly name: string;
  readonly avatarUrl: string;
};

type UserProfile = {
  readonly userId: string;
  readonly name: string;
  readonly avatarUrl?: string;
};

type Room = { readonly name: string; readonly people: readonly Person[] };

const BOT: Person = { userId: BOT_ID, name: "Office Hours", avatarUrl: "" };

const rooms = new Map<string, Room>();
const people = new Map<string, Person>([[BOT_ID, BOT]]);

/** Records a room's name and members (the bot is always one of them). Replaces what was there. */
export function rememberRoom(
  groupId: string,
  name: string,
  members: readonly Person[],
): void {
  rooms.set(groupId, { name, people: members });
  members.forEach((person) => people.set(person.userId, person));
}

/** For tests: start empty. */
export function forgetEverything(): void {
  rooms.clear();
  people.clear();
  people.set(BOT_ID, BOT);
}

function toProfile(person: Person): UserProfile {
  // Chat UI keeps its own default picture when no avatar is given; an empty string would break it.
  return person.avatarUrl
    ? {
        userId: person.userId,
        name: person.name,
        avatarUrl: person.avatarUrl,
      }
    : { userId: person.userId, name: person.name };
}

/** reqUserProfiles: every profile needs a non-empty name, known person or not. */
export function userProfiles(userIds: readonly string[]): UserProfile[] {
  return userIds.map((userId) => {
    const person = people.get(userId);
    return person ? toProfile(person) : { userId, name: "New member" };
  });
}

/** reqGroupMembers: user ids only. Chat UI's nickname field pairs names wrongly, so it is not used. */
export function membersOf(groupId: string): { readonly userId: string }[] {
  const members = rooms.get(groupId)?.people ?? [];
  return [BOT, ...members.filter((person) => person.userId !== BOT_ID)].map(
    (person) => ({ userId: person.userId }),
  );
}

/** reqGroupProfiles */
export function groupProfiles(groupIds: readonly string[]) {
  return groupIds.map((groupId) => ({
    groupId,
    name: rooms.get(groupId)?.name ?? "Office Hours room",
    memberCount: membersOf(groupId).length,
  }));
}

/** A message from someone this returns false for means the room should reload its members. */
export function isKnownMember(groupId: string, userId: string): boolean {
  return membersOf(groupId).some((member) => member.userId === userId);
}

/** Everyone the page has been told about, for filling Chat UI's profile cache. */
export function knownPeople(): readonly Person[] {
  return [...people.values()];
}
