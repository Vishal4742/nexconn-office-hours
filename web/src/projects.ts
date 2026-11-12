import { ApiError, api } from "./api";
import { isRepoName } from "./router";

export type RepoName = { readonly owner: string; readonly repo: string };

/** A registered repository, as the server describes it to the signed-in person. */
export type Project = RepoName & {
  readonly id: string;
  readonly description: string;
  readonly isMember: boolean;
  readonly isMaintainer: boolean;
};

const GITHUB_ADDRESS =
  /^(?:(?:https?:\/\/)?(?:www\.)?github\.com\/|git@github\.com:)/i;

/** Reads "owner/repo", or a GitHub address someone pasted. Null when it is neither. */
export function parseRepo(input: string): RepoName | null {
  const text = input.trim();
  const isAddress = GITHUB_ADDRESS.test(text);
  const parts = text
    .replace(GITHUB_ADDRESS, "")
    .replace(/[?#].*$/, "")
    .replace(/\/+$/, "")
    .split("/");
  // An address may point inside the repo (…/issues/12). Typed by hand it is owner/repo, no more.
  if (parts.length < 2 || (!isAddress && parts.length > 2)) return null;
  const owner = parts[0];
  const repo = parts[1].replace(/\.git$/, "");
  return isRepoName(owner, repo) ? { owner, repo } : null;
}

/** Where a project's room lives in the app (FR-PROJ-3). */
export function roomPath({ owner, repo }: RepoName): string {
  return `/p/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`;
}

function isProject(value: unknown): value is Project {
  if (typeof value !== "object" || value === null) return false;
  const project = value as Record<string, unknown>;
  return (
    typeof project.id === "string" &&
    typeof project.owner === "string" &&
    typeof project.repo === "string" &&
    typeof project.description === "string" &&
    typeof project.isMember === "boolean" &&
    typeof project.isMaintainer === "boolean"
  );
}

function unexpected(): ApiError {
  return new ApiError(
    200,
    "The server sent something unexpected. Reload the page and try again.",
  );
}

/** FR-PROJ-1, FR-PROJ-2: registers a repo the signed-in person maintains, or returns its project. */
export async function registerProject(name: RepoName): Promise<Project> {
  const data = await api.post<unknown>("/api/projects", name);
  if (!isProject(data)) throw unexpected();
  return data;
}

/** The rooms the signed-in person is in, in the server's order (by name). */
export async function listMyProjects(): Promise<readonly Project[]> {
  const data = await api.get<unknown>("/api/me/projects");
  if (!Array.isArray(data) || !data.every(isProject)) throw unexpected();
  return data;
}
