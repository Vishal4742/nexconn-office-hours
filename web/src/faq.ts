// The FAQ as the room's pages use it. The public FAQ pages themselves are rendered by the Go
// server. This file is a contract (notes/phase-2-plan.md); server/shapes.go has the same shape.
import { ApiError, api } from "./api";
import { roomPath } from "./projects";
import type { RepoName } from "./projects";

export type FaqEntry = {
  readonly id: string;
  readonly slug: string;
  readonly title: string;
  readonly question: string;
  readonly answer: string;
  readonly links: readonly string[];
  /** How many people asked it. Who they were is not public (FR-HAND-8). */
  readonly askedBy: number;
  readonly createdAt: string;
  /** The login of the maintainer who answered. */
  readonly author: string;
};

/**
 * The address of a project's FAQ list, or of one entry. Always on our own origin and built from
 * the slug: an address that arrived in a message is never used (NFR-SEC-6). The Go server renders
 * these pages, so link to them with a plain <a>, not with the app's Link.
 */
export function faqPath(repo: RepoName, slug?: string): string {
  const list = `${roomPath(repo)}/faq`;
  return slug ? `${list}/${encodeURIComponent(slug)}` : list;
}

function asEntries(data: unknown): readonly FaqEntry[] {
  const isList =
    Array.isArray(data) &&
    data.every(
      (entry: unknown) =>
        typeof entry === "object" &&
        entry !== null &&
        typeof (entry as FaqEntry).slug === "string" &&
        typeof (entry as FaqEntry).title === "string",
    );
  if (!isList) {
    throw new ApiError(
      200,
      "The server sent something unexpected. Reload the page and try again.",
    );
  }
  return data as readonly FaqEntry[];
}

/**
 * FR-HAND-3, FR-FAQ-4: up to 3 entries that look like the question being typed, the best first.
 * The text travels in the request's body, never in an address.
 */
export async function similarFaq(
  repo: RepoName,
  text: string,
): Promise<readonly FaqEntry[]> {
  return asEntries(
    await api.post<unknown>(`/api${roomPath(repo)}/faq/similar`, { text }),
  );
}

/** FR-PROJ-3: the project's newest entries, for the join page. Public. */
export async function latestFaq(repo: RepoName): Promise<readonly FaqEntry[]> {
  return asEntries(await api.get<unknown>(`/api${roomPath(repo)}/faq/latest`));
}

/** FR-FAQ-6 (maintainer): the entry's page answers "removed" from then on. */
export async function deleteFaq(id: string): Promise<void> {
  await api.post<unknown>(`/api/faq/${encodeURIComponent(id)}/delete`);
}
