import { mutedLine } from "./muted";
import { useServerNow } from "./panel/useServerNow";

/** How often the notice checks whether the mute has ended. */
const TICK_MS = 15_000;

/**
 * Above the chat, for as long as the viewer's mute runs: their messages would fail with only a
 * red mark otherwise (FR-MOD-1, phase-5 finding S3).
 */
export function MutedNotice({ until }: { readonly until: string | null }) {
  const now = useServerNow(TICK_MS);
  const line = mutedLine(until, now);
  if (!line) return null;
  return (
    <p className="chat-muted" role="status">
      {line}
    </p>
  );
}
