// "Already answered?" in the raise-hand sheet (FR-HAND-3, FR-FAQ-4): FAQ entries that look like
// the question being typed. Kept apart from React so its timing can be tested on its own.
import type { FaqEntry } from "../../faq";
import { HAND_TITLE, charCount, cleanLine, cleanText } from "./limits";

/** How long typing must pause before the server is asked. */
export const SIMILAR_WAIT_MS = 300;

/** The text to search for: the title and the details, cleaned; "" while the title is too short. */
export function similarText(title: string, details: string): string {
  const line = cleanLine(title);
  if (charCount(line) < HAND_TITLE.min) return "";
  const more = cleanText(details);
  return more === "" ? line : `${line}\n${more}`;
}

export type SimilarSearch = {
  /** The text in the form changed. "" clears the list and asks nothing. */
  readonly ask: (text: string) => void;
  /** The sheet closed: nothing more is asked, and nothing that arrives is shown. */
  readonly stop: () => void;
};

/**
 * Asks `find` once typing has paused, and shows only the answer for the text that is in the form
 * now: an answer that arrives for older text is dropped, whatever order the answers come in.
 */
export function similarSearch(
  find: (text: string) => Promise<readonly FaqEntry[]>,
  show: (entries: readonly FaqEntry[]) => void,
  waitMs = SIMILAR_WAIT_MS,
): SimilarSearch {
  let latest: string | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function stop(): void {
    latest = null;
    clearTimeout(timer);
  }

  function ask(text: string): void {
    stop();
    latest = text;
    if (text === "") {
      show([]);
      return;
    }
    timer = setTimeout(() => {
      find(text).then(
        (entries) => {
          if (latest === text) show(entries);
        },
        () => {
          // The suggestions are a help, not a step: when the search fails, the list stays as it
          // was and the hand can still be raised.
        },
      );
    }, waitMs);
  }

  return { ask, stop };
}
