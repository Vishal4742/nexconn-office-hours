import { deleteFaq, faqPath } from "../../faq";
import type { Project } from "../../projects";
import { FATE } from "../../queue/place";
import { refreshQueue } from "../../queue/store";
import type { Hand } from "../../queue/types";
import { AskOnce } from "./AskOnce";
import { Said } from "./Said";
import { useAction } from "./useAction";

type Props = {
  readonly project: Project;
  /** The session's other hands, the latest first. */
  readonly done: readonly Hand[];
  readonly isMaintainer: boolean;
  /** Open at first: when the session ended, this list is what is left to read. */
  readonly isOpen: boolean;
};

/**
 * "Done this session": what became of each hand (plan decision 11). An answered hand links to its
 * public FAQ page; a removed one has no words left to show (FR-MOD-1).
 */
export function DoneList({ project, done, isMaintainer, isOpen }: Props) {
  if (done.length === 0) return null;
  return (
    <details className="q-done" open={isOpen}>
      <summary>
        <span>Done this session</span>
        <span className="q-count">{done.length}</span>
      </summary>
      <ul>
        {done.map((hand) => (
          <DoneItem
            key={hand.id}
            project={project}
            hand={hand}
            isMaintainer={isMaintainer}
          />
        ))}
      </ul>
    </details>
  );
}

type ItemProps = {
  readonly project: Project;
  readonly hand: Hand;
  readonly isMaintainer: boolean;
};

function DoneItem({ project, hand, isMaintainer }: ItemProps) {
  const action = useAction();
  // FR-FAQ-6, AT-8: the entry's page says "removed" from then on, and the queue forgets the link.
  const removeAnswer = () =>
    void action.run(async () => {
      await deleteFaq(hand.faqId);
      refreshQueue();
    });

  return (
    <li className="q-done-item">
      <span className="q-done-text">
        <span className="q-login">@{hand.author.login}</span>{" "}
        {hand.faqSlug ? (
          // The Go server renders the FAQ pages: a plain link, in a tab of its own so the room stays.
          <a
            href={faqPath(project, hand.faqSlug)}
            target="_blank"
            rel="noopener"
          >
            {hand.title}
          </a>
        ) : (
          hand.title
        )}
      </span>
      <span className={`q-fate q-fate-${hand.status}`}>
        {FATE[hand.status]}
      </span>
      {isMaintainer && hand.faqId && (
        <AskOnce
          label="Remove from the FAQ"
          question="Remove this answer? Its page will say it was removed."
          confirm="Remove answer"
          isBusy={action.isBusy}
          onConfirm={removeAnswer}
        />
      )}
      <Said doing={action.doing} />
    </li>
  );
}
