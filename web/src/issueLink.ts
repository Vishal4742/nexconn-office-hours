import type { RepoName } from "./projects";

// The longest body an issue link carries, counted as it is written in the address: GitHub turns
// away an address that is too long, and a letter outside ASCII takes up to 12 characters there.
const MAX_BODY = 4000;

/** The body, cut a tenth at a time until it fits, with "…" where it was cut. */
function fitted(body: string): string {
  let letters = [...body];
  let text = body;
  while (letters.length > 0 && encodeURIComponent(text).length > MAX_BODY) {
    letters = letters.slice(0, Math.floor(letters.length * 0.9));
    text = `${letters.join("")}…`;
  }
  return letters.length > 0 ? text : "";
}

/**
 * FR-FAQ-5: the address of GitHub's "new issue" page for a repo, with the title and the body
 * filled in. The person still has to press GitHub's own button: the app writes nothing to GitHub.
 */
export function issueUrl(repo: RepoName, title: string, body: string): string {
  const query = new URLSearchParams({ title, body: fitted(body) });
  return `https://github.com/${encodeURIComponent(repo.owner)}/${encodeURIComponent(repo.repo)}/issues/new?${query.toString()}`;
}
