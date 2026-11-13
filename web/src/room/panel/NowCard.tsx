import { Avatar } from "../../components/Avatar";
import { Check, GitHubMark } from "../../components/Icons";
import { issueUrl } from "../../issueLink";
import type { Project } from "../../projects";
import { markAway, skipHand } from "../../queue/api";
import { questionTimer } from "../../queue/polish";
import { actOnQueue } from "../../queue/store";
import type { Hand } from "../../queue/types";
import { Details } from "./Details";
import { useMaintainerKey } from "./keys";
import { Said } from "./Said";
import { useAction } from "./useAction";
import { useServerNow } from "./useServerNow";

type Props = {
  readonly project: Project;
  /** The hand being answered. */
  readonly hand: Hand;
  readonly isMaintainer: boolean;
  readonly onDone: (hand: Hand) => void;
};

/** FR-FAQ-5: what the GitHub issue says, the person who asked named. */
function issueBody(hand: Hand): string {
  const credit = `Asked by @${hand.author.login} in office hours.`;
  return hand.details ? `${hand.details}\n\n${credit}` : credit;
}

/**
 * "Now answering": the one solid blue card (docs/UI.md S3). A maintainer also sees whether the
 * author said "I'm here" (FR-QUEUE-9) and finishes the hand from here: Done opens the Done sheet,
 * Skip and Away act at once (FR-QUEUE-3, FR-QUEUE-8).
 */
export function NowCard({ project, hand, isMaintainer, onDone }: Props) {
  const action = useAction();
  const skip = () => void action.run(() => actOnQueue(() => skipHand(hand.id)));
  const away = () => void action.run(() => actOnQueue(() => markAway(hand.id)));
  // The maintainer's keys (docs/UI.md S3). Away only while the author has not said "I'm here".
  const canAct = isMaintainer && !action.isBusy;
  useMaintainerKey("done", canAct, () => onDone(hand));
  useMaintainerKey("skip", canAct, skip);
  useMaintainerKey("away", canAct && !hand.ackedAt, away);

  return (
    <section className="q-now" aria-label="Now answering">
      <div className="q-now-top">
        <span className="q-eyebrow">Now answering</span>
        {isMaintainer && <Timer takenAt={hand.takenAt} />}
        {isMaintainer && (
          <span className="q-ack">
            {hand.ackedAt ? (
              <>
                <Check />
                here
              </>
            ) : (
              "waiting for a reply"
            )}
          </span>
        )}
      </div>
      <div className="q-now-who">
        <Avatar name={hand.author.name} url={hand.author.avatarUrl} />
        <span className="q-login">
          {hand.isMine ? "You" : `@${hand.author.login}`}
        </span>
      </div>
      <p className="q-now-title">{hand.title}</p>
      {hand.details && (
        <Details className="q-now-details" text={hand.details} />
      )}
      {hand.plusCount > 0 && (
        <p className="q-also">+{hand.plusCount} also asking</p>
      )}
      <div className="q-now-actions">
        {isMaintainer && (
          <>
            <button
              type="button"
              className="btn q-on-blue sm"
              disabled={action.isBusy}
              onClick={() => onDone(hand)}
              aria-keyshortcuts="D"
            >
              <Check />
              Done <kbd className="q-kbd">D</kbd>
            </button>
            <button
              type="button"
              className="btn q-on-blue-ghost sm"
              disabled={action.isBusy}
              onClick={skip}
              aria-keyshortcuts="S"
            >
              Skip <kbd className="q-kbd">S</kbd>
            </button>
            {!hand.ackedAt && (
              <button
                type="button"
                className="btn q-on-blue-ghost sm"
                title="No reply: they move back in line"
                disabled={action.isBusy}
                onClick={away}
                aria-keyshortcuts="A"
              >
                Away <kbd className="q-kbd">A</kbd>
              </button>
            )}
          </>
        )}
        <a
          className="btn q-on-blue-ghost sm"
          href={issueUrl(project, hand.title, issueBody(hand))}
          target="_blank"
          rel="noopener noreferrer nofollow"
        >
          <GitHubMark />
          Open a GitHub issue
        </a>
      </div>
      <Said doing={action.doing} />
    </section>
  );
}

/**
 * FR-QUEUE-12: how long the question has taken, for the maintainer only, amber after 8 minutes.
 * Counted on the server's clock, read every second while it shows.
 */
function Timer({ takenAt }: { readonly takenAt: string | null }) {
  const timer = questionTimer(takenAt, useServerNow(1000));
  if (!timer.label) return null;
  return (
    <span
      className={timer.isLong ? "q-timer is-long" : "q-timer"}
      title={
        timer.isLong ? "This question has taken over 8 minutes" : undefined
      }
    >
      <span className="sr-only">On this question for </span>
      {timer.label}
    </span>
  );
}
