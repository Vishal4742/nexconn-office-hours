import type { RepoName } from "../projects";

// Whether this browser was in a room as a member the last time it opened it. A hint for the first
// render only: it picks what shows while the project loads (the room's skeleton, or the join
// card's photo, NFR-PERF-2), never what anyone may do. Storage can be missing, blocked or full;
// then the hint is simply off.

function keyOf({ owner, repo }: RepoName): string {
  return `oh:member:${owner.toLowerCase()}/${repo.toLowerCase()}`;
}

export function wasMember(name: RepoName): boolean {
  try {
    return globalThis.localStorage?.getItem(keyOf(name)) === "1";
  } catch {
    return false;
  }
}

export function rememberMembership(name: RepoName, isMember: boolean): void {
  try {
    if (isMember) globalThis.localStorage?.setItem(keyOf(name), "1");
    else globalThis.localStorage?.removeItem(keyOf(name));
  } catch {
    // Blocked or full storage: the next visit shows the other loading view, nothing more.
  }
}
