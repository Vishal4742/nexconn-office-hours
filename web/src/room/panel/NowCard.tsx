import { Avatar } from "../../components/Avatar";
import { Check, GitHubMark } from "../../components/Icons";
import { issueUrl } from "../../issueLink";
import type { Project } from "../../projects";
import { markAway, skipHand } from "../../queue/api";
import { actOnQueue } from "../../queue/store";
import type { Hand } from "../../queue/types";
import { Details } from "./Details";
import { Said } from "./Said";
import { useAction } from "./useAction";

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

  return (
    <section className="q-now" aria-label="Now answering">
      <div className="q-now-top">
        <span className="q-eyebrow">Now answering</span>
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
            >
              <Check />
              Done
            </button>
            <button
              type="button"
              className="btn q-on-blue-ghost sm"
              disabled={action.isBusy}
              onClick={skip}
            >
              Skip
            </button>
            {!hand.ackedAt && (
              <button
                type="button"
                className="btn q-on-blue-ghost sm"
                title="No reply: they move back in line"
                disabled={action.isBusy}
                onClick={away}
              >
                Away
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
