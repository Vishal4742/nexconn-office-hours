// The light of the room's photo follows the session's clock (docs/UI.md S3): the same desk from
// afternoon to night. Plain logic, so it can be tested; the panel draws it.
import { toMs } from "./clock";
import type { QueueSession } from "./types";

export type Light = "day" | "golden" | "dusk" | "night";

/** The length a session without a plan is drawn for, in minutes. */
const DEFAULT_MINUTES = 45;

/**
 * The light for a session at a moment of the server's clock. Afternoon for the first 35 % of the
 * planned length, golden hour to 70 %, dusk to 95 %, then night. Last call and an ended session
 * are night; no session, a clock before the start or a time that can't be read are afternoon.
 */
export function sessionLight(
  session: QueueSession | null,
  serverNow: number,
): Light {
  if (!session) return "day";
  if (!session.live || session.handsClosed) return "night";
  const minutes = session.plannedMinutes || DEFAULT_MINUTES;
  const share = (serverNow - toMs(session.startedAt)) / (minutes * 60_000);
  if (!(share >= 0.35)) return "day"; // NaN too
  if (share < 0.7) return "golden";
  if (share < 0.95) return "dusk";
  return "night";
}
