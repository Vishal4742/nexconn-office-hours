import type { RepoName } from "./projects";

// GitHub cuts off an address that is too long; a body of this size always fits.
const MAX_BODY = 4000;

/**
 * FR-FAQ-5: the address of GitHub's "new issue" page for a repo, with the title and the body
 * filled in. The person still has to press GitHub's own button: the app writes nothing to GitHub.
 */
export function issueUrl(repo: RepoName, title: string, body: string): string {
  const cut = body.length > MAX_BODY ? `${body.slice(0, MAX_BODY - 1)}…` : body;
  const query = new URLSearchParams({ title, body: cut });
  return `https://github.com/${encodeURIComponent(repo.owner)}/${encodeURIComponent(repo.repo)}/issues/new?${query.toString()}`;
}
