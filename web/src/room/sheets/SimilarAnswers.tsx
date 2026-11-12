import { useEffect, useId, useState } from "react";

import { faqPath, similarFaq } from "../../faq";
import type { FaqEntry } from "../../faq";
import type { RepoName } from "../../projects";
import { similarSearch, similarText } from "./similar";

type Props = {
  readonly project: RepoName;
  /** What is typed in the raise-hand sheet, as it is. */
  readonly title: string;
  readonly details: string;
  /** "That solved it": the person leaves without raising a hand. */
  readonly onSolved: () => void;
};

/** The server sends at most 3 (FR-FAQ-4); the sheet never shows more. */
const SHOWN = 3;

function lookAlike(count: number): string {
  if (count === 0) return "";
  return count === 1
    ? "1 answered question looks like yours."
    : `${count} answered questions look like yours.`;
}

/**
 * "Already answered?" (FR-HAND-3): up to 3 FAQ entries like the question being typed, each opening
 * in place to show its answer. Nothing shows while there is nothing to show.
 */
export function SimilarAnswers({ project, title, details, onSolved }: Props) {
  const headId = useId();
  const [entries, setEntries] = useState<readonly FaqEntry[]>([]);
  const [openId, setOpenId] = useState<string | null>(null);
  const [search] = useState(() =>
    similarSearch((text) => similarFaq(project, text), setEntries),
  );
  const text = similarText(title, details);
  const shown = entries.slice(0, SHOWN);

  useEffect(() => search.ask(text), [search, text]);
  useEffect(() => () => search.stop(), [search]);

  return (
    <>
      <p className="sr-only" role="status">
        {lookAlike(shown.length)}
      </p>
      {shown.length > 0 && (
        <section className="sheet-similar" aria-labelledby={headId}>
          <h3 id={headId}>Already answered?</h3>
          <ul>
            {shown.map((entry) => (
              <Entry
                key={entry.id}
                project={project}
                entry={entry}
                isOpen={entry.id === openId}
                onToggle={() =>
                  setOpenId(entry.id === openId ? null : entry.id)
                }
                onSolved={onSolved}
              />
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

type EntryProps = {
  readonly project: RepoName;
  readonly entry: FaqEntry;
  readonly isOpen: boolean;
  readonly onToggle: () => void;
  readonly onSolved: () => void;
};

function Entry({ project, entry, isOpen, onToggle, onSolved }: EntryProps) {
  const bodyId = useId();
  return (
    <li>
      <button
        type="button"
        className="sheet-entry"
        aria-expanded={isOpen}
        aria-controls={bodyId}
        onClick={onToggle}
      >
        <span>{entry.title}</span>
        <svg
          viewBox="0 0 16 16"
          aria-hidden="true"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="m4 6 4 4 4-4" />
        </svg>
      </button>
      <div className="sheet-entry-body" id={bodyId} hidden={!isOpen}>
        <p className="sheet-entry-answer">{entry.answer}</p>
        <div className="sheet-entry-actions">
          <button type="button" className="btn primary sm" onClick={onSolved}>
            That solved it
          </button>
          {/* The Go server renders the FAQ pages: a plain link, in a new tab, so the sheet stays. */}
          <a href={faqPath(project, entry.slug)} target="_blank" rel="noopener">
            Open its page
            <span className="sr-only"> (opens in a new tab)</span>
          </a>
        </div>
      </div>
    </li>
  );
}
