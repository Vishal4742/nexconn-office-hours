import { withdrawHand } from "../../queue/api";
import { placeLabel } from "../../queue/place";
import { actOnQueue } from "../../queue/store";
import type { Hand } from "../../queue/types";
import { AskOnce } from "./AskOnce";
import { StarIcon } from "./icons";
import { Said } from "./Said";
import { useAction } from "./useAction";

type Props = {
  /** My waiting hand. */
  readonly hand: Hand;
  /** How many wait before it. */
  readonly ahead: number;
  /** FR-QUEUE-10: "about 10 min", or "" while there is no median yet. */
  readonly wait: string;
};

/**
 * Where I am, in the selection box of the landing's "once." (docs/UI.md S3): "3 ahead of you" or
 * "You're next" (FR-QUEUE-5), and Withdraw (FR-HAND-4), which asks once: a hand raised again
 * waits a minute (FR-HAND-6) and goes to the end.
 */
export function MyPlace({ hand, ahead, wait }: Props) {
  const action = useAction();
  const withdraw = () =>
    void action.run(() => actOnQueue(() => withdrawHand(hand.id)));

  return (
    <section className="q-mine" aria-label="Your place">
      <p className="q-mine-pos">
        <StarIcon />
        {placeLabel(ahead)}
      </p>
      {wait && <p className="q-mine-wait">{wait}</p>}
      {hand.awayCount > 0 && (
        <p className="q-mine-note">
          You missed your turn, so you moved back. When it comes, press
          &ldquo;I&rsquo;m here&rdquo;.
        </p>
      )}
      <div className="q-btns">
        <AskOnce
          label="Withdraw"
          question="Withdraw your hand? You lose your place."
          confirm="Withdraw"
          isBusy={action.isBusy}
          onConfirm={withdraw}
        />
      </div>
      <Said doing={action.doing} />
    </section>
  );
}
