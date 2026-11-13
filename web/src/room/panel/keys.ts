// The maintainer's keys (docs/UI.md S3): N takes the next hand, D opens Done, S skips and A marks
// the author away. Plain logic, so it can be tested; useMaintainerKey listens.
import { useEffect, useRef } from "react";

export type MaintainerKey = "next" | "done" | "skip" | "away";

const KEYS: Readonly<Record<string, MaintainerKey>> = {
  n: "next",
  d: "done",
  s: "skip",
  a: "away",
};

type Press = {
  readonly key: string;
  readonly altKey: boolean;
  readonly ctrlKey: boolean;
  readonly metaKey: boolean;
  readonly repeat: boolean;
};

/** Where a press can't be meant for the queue: someone is typing, or a sheet is open. */
function isBusy(target: EventTarget): boolean {
  const element = target as Partial<HTMLElement>;
  const tag = element.tagName ?? "";
  return (
    element.isContentEditable === true ||
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    tag === "SELECT" ||
    tag === "DIALOG"
  );
}

/**
 * Which of the maintainer's keys a press is, or null. The path is the event's composed path, so a
 * press inside Chat UI's own input (in a shadow root) counts as typing too.
 */
export function maintainerKey(
  press: Press,
  path: readonly EventTarget[],
): MaintainerKey | null {
  if (press.altKey || press.ctrlKey || press.metaKey || press.repeat) {
    return null;
  }
  const key = KEYS[press.key.toLowerCase()];
  if (!key || path.some(isBusy)) return null;
  return key;
}

/** Calls back when the maintainer presses `key` and `isOn` is true. */
export function useMaintainerKey(
  key: MaintainerKey,
  isOn: boolean,
  onPress: () => void,
): void {
  const latest = useRef(onPress);
  useEffect(() => {
    latest.current = onPress;
  });
  useEffect(() => {
    if (!isOn) return;
    const listen = (event: KeyboardEvent) => {
      if (maintainerKey(event, event.composedPath()) !== key) return;
      event.preventDefault();
      latest.current();
    };
    document.addEventListener("keydown", listen);
    return () => document.removeEventListener("keydown", listen);
  }, [key, isOn]);
}
