// The numbers of FR-MET-1. This file is a contract (notes/phase-2-plan.md); server/shapes.go has
// the same shape.
import { ApiError, api } from "./api";
import { roomPath } from "./projects";
import type { RepoName } from "./projects";

export type Stats = {
  readonly sessions: number;
  readonly handsRaised: number;
  readonly handsAnswered: number;
  readonly faqEntries: number;
  /** People who joined a room. */
  readonly participants: number;
};

const FIELDS = [
  "sessions",
  "handsRaised",
  "handsAnswered",
  "faqEntries",
  "participants",
] as const;

/** One project's numbers, or those of every project together when repo is null. Public. */
export async function loadStats(repo: RepoName | null): Promise<Stats> {
  const data = await api.get<unknown>(
    repo ? `/api${roomPath(repo)}/stats` : "/api/stats",
  );
  const stats = data as Partial<Stats> | null;
  const isStats =
    typeof stats === "object" &&
    stats !== null &&
    FIELDS.every((field) => typeof stats[field] === "number");
  if (!isStats) {
    throw new ApiError(
      200,
      "The server sent something unexpected. Reload the page and try again.",
    );
  }
  return stats as Stats;
}
