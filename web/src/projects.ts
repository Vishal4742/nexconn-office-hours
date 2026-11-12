import { ApiError, api } from "./api";
import type { Person } from "./nexconn/roster";
import { isRepoName } from "./router";

export type RepoName = { readonly owner: string; readonly repo: string };

/** A registered repository, as the server describes it to whoever asks. */
export type Project = RepoName & {
  readonly description: string;
  readonly isMember: boolean;
  readonly isMaintainer: boolean;
  /** The Nexconn group of its room. The server tells it to members only. */
  readonly groupId?: string;
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

/** Whether two names are the same repository. GitHub ignores letter case. */
export function isSameRepo(a: RepoName, b: RepoName): boolean {
  return (
    a.owner.toLowerCase() === b.owner.toLowerCase() &&
    a.repo.toLowerCase() === b.repo.toLowerCase()
  );
}

function isProject(value: unknown): value is Project {
  if (typeof value !== "object" || value === null) return false;
  const project = value as Record<string, unknown>;
  return (
    (project.groupId === undefined || typeof project.groupId === "string") &&
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

/** FR-PROJ-3: what a project's page shows first. Public; the answer depends on who asks. */
export async function loadProject(name: RepoName): Promise<Project> {
  const data = await api.get<unknown>(`/api${roomPath(name)}`);
  if (!isProject(data)) throw unexpected();
  return data;
}

/** FR-PROJ-4: joins the room. The answer is the project as a member sees it. */
export async function joinProject(name: RepoName): Promise<Project> {
  const data = await api.post<unknown>(`/api${roomPath(name)}/join`);
  if (!isProject(data)) throw unexpected();
  return data;
}

function isPerson(value: unknown): value is Person {
  if (typeof value !== "object" || value === null) return false;
  const person = value as Record<string, unknown>;
  return (
    typeof person.userId === "string" &&
    typeof person.name === "string" &&
    person.name !== "" && // Chat UI refuses a profile without a name
    typeof person.avatarUrl === "string"
  );
}

/** FR-PROJ-4: who is in the room, for the chat's names and avatars. Members only. */
export async function listMembers(name: RepoName): Promise<readonly Person[]> {
  const data = await api.get<unknown>(`/api${roomPath(name)}/members`);
  if (!Array.isArray(data) || !data.every(isPerson)) throw unexpected();
  return data;
}
