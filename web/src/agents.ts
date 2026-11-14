// FR-AGENT-3: the AI apps a person let act for them (through the sign-in at /oauth/authorize),
// and Disconnect, which refuses the app from its next request on.
import { ApiError, api } from "./api";

export type Agent = {
  readonly id: string;
  /** What the app called itself when it registered: text, not proof of who made it. */
  readonly app: string;
  /** Where the consent page sent the person back: a host, or an app's own scheme. */
  readonly returnsTo: string;
  readonly connectedAt: string;
  /** null until the app's first request. */
  readonly lastUsedAt: string | null;
};

function isAgent(value: unknown): value is Agent {
  if (typeof value !== "object" || value === null) return false;
  const agent = value as Record<string, unknown>;
  return (
    typeof agent.id === "string" &&
    typeof agent.app === "string" &&
    typeof agent.returnsTo === "string" &&
    typeof agent.connectedAt === "string" &&
    (agent.lastUsedAt === null || typeof agent.lastUsedAt === "string")
  );
}

/** The signed-in person's connected apps, the newest first. */
export async function listAgents(): Promise<readonly Agent[]> {
  const data = await api.get<unknown>("/api/me/agents");
  if (!Array.isArray(data) || !data.every(isAgent)) {
    throw new ApiError(
      200,
      "The server sent something unexpected. Reload the page and try again.",
    );
  }
  return data;
}

/** FR-AGENT-1: the address an AI app adds as an MCP server: this site's own /mcp. */
export function mcpAddress(origin: string): string {
  return `${origin}/mcp`;
}

/** The one line that adds Office Hours to Claude Code. */
export function claudeCodeCommand(address: string): string {
  return `claude mcp add --transport http office-hours ${address}`;
}

/** Ends one app's access. */
export function disconnectAgent(id: string): Promise<void> {
  return api.post(`/api/me/agents/${encodeURIComponent(id)}/disconnect`);
}

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** "Used 10 min ago", from the last request the app made. */
export function lastUsedLabel(lastUsedAt: string | null, now: number): string {
  if (lastUsedAt === null) return "Not used yet";
  const ago = now - Date.parse(lastUsedAt);
  if (ago < MINUTE) return "Used just now";
  if (ago < HOUR) return `Used ${Math.floor(ago / MINUTE)} min ago`;
  if (ago < DAY) return `Used ${Math.floor(ago / HOUR)} h ago`;
  const days = Math.floor(ago / DAY);
  return `Used ${days} ${days === 1 ? "day" : "days"} ago`;
}
