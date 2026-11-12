import { useRef, useState } from "react";
import type { KeyboardEvent, ReactNode } from "react";

import { Avatar } from "../../components/Avatar";
import type { Project } from "../../projects";
import {
  mergeHand,
  muteMember,
  removeHand,
  skipHand,
  togglePlus,
} from "../../queue/api";
import { actOnQueue } from "../../queue/store";
import type { Hand, Queue } from "../../queue/types";
import { AskOnce } from "./AskOnce";
import { DotsIcon } from "./icons";
import { Said } from "./Said";
import { useAction } from "./useAction";

/** FR-MOD-1: how long a maintainer's mute lasts. */
const MUTE_MINUTES = 10;

type Props = {
  readonly project: Project;
  /** The waiting hands, in the server's order. */
  readonly waiting: readonly Hand[];
  readonly isMaintainer: boolean;
  /** Whether a hand is being answered: a waiting hand can be merged into it. */
  readonly canMerge: boolean;
  /** Last call: "Hands closed, N left" (FR-SESS-6). */
  readonly closedLeft: number | null;
  /** The done list, at the card's foot. */
  readonly children?: ReactNode;
};

/**
 * "Waiting [ N in line ]": numbered rows in the server's order (FR-QUEUE-1), each with the +1
 * count. A member can +1 someone else's hand and take it back (FR-HAND-7); a maintainer gets a
 * menu on each row instead. One row's menu is open at a time.
 */
export function WaitingCard({
  project,
  waiting,
  isMaintainer,
  canMerge,
  closedLeft,
  children,
}: Props) {
  const [openRow, setOpenRow] = useState<string | null>(null);
  return (
    <section className="qcard q-wait" aria-label="Waiting">
      <div className="q-wait-head">
        <span className="q-eyebrow">Waiting</span>
        <span className="q-count">
          {closedLeft !== null && (
            <span className="q-closed">Hands closed, {closedLeft} left · </span>
          )}
          [ {waiting.length} in line ]
        </span>
      </div>
      <ol className="q-list">
        {waiting.map((hand, at) => (
          <Row
            key={hand.id}
            project={project}
            hand={hand}
            place={at + 1}
            isMaintainer={isMaintainer}
            canMerge={canMerge}
            isOpen={openRow === hand.id}
            onToggle={() => setOpenRow(openRow === hand.id ? null : hand.id)}
            onClose={() => setOpenRow(null)}
          />
        ))}
      </ol>
      {children}
    </section>
  );
}

type RowProps = {
  readonly project: Project;
  readonly hand: Hand;
  /** 1 for the first in line. */
  readonly place: number;
  readonly isMaintainer: boolean;
  readonly canMerge: boolean;
  readonly isOpen: boolean;
  readonly onToggle: () => void;
  readonly onClose: () => void;
};

function Row({
  project,
  hand,
  place,
  isMaintainer,
  canMerge,
  isOpen,
  onToggle,
  onClose,
}: RowProps) {
  const action = useAction();
  const toggleRef = useRef<HTMLButtonElement>(null);
  const menuId = `q-menu-${hand.id}`;
  const login = `@${hand.author.login}`;

  function handleMenuKey(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "Escape") return;
    onClose();
    toggleRef.current?.focus();
  }

  return (
    <li className={hand.isMine ? "q-row q-row-mine" : "q-row"}>
      <span className="q-num" aria-hidden="true">
        {String(place).padStart(2, "0")}
      </span>
      <Avatar name={hand.author.name} url={hand.author.avatarUrl} />
      <p className="q-who">
        <span className="q-login">{login}</span>
        {hand.isMine && <span className="q-you">You</span>}
      </p>
      <span className="q-side">
        {(isMaintainer || hand.isMine) && hand.plusCount > 0 && (
          <span className="tag">+{hand.plusCount}</span>
        )}
        {isMaintainer ? (
          <button
            ref={toggleRef}
            type="button"
            className="icon-btn"
            aria-expanded={isOpen}
            aria-controls={isOpen ? menuId : undefined}
            aria-label={`Actions for ${login}'s hand`}
            onClick={onToggle}
          >
            <DotsIcon />
          </button>
        ) : (
          !hand.isMine && <PlusButton hand={hand} action={action} />
        )}
      </span>
      <p className="q-ttl">{hand.title}</p>
      {isMaintainer && isOpen && (
        <div
          id={menuId}
          className="q-acts"
          role="group"
          aria-label={`Actions for ${login}'s hand`}
          onKeyDown={handleMenuKey}
        >
          <RowActions
            project={project}
            hand={hand}
            canMerge={canMerge}
            action={action}
          />
        </div>
      )}
      <Said doing={action.doing} />
    </li>
  );
}

type Action = ReturnType<typeof useAction>;

/** FR-HAND-7: "+1 me too" on someone else's waiting hand, pressed again to take it back. */
function PlusButton({
  hand,
  action,
}: {
  readonly hand: Hand;
  readonly action: Action;
}) {
  const toggle = () =>
    void action.run(() => actOnQueue(() => togglePlus(hand.id)));
  return (
    <button
      type="button"
      className="q-plus"
      aria-pressed={hand.plusByMe}
      title={hand.plusByMe ? "Take back your +1" : "Me too"}
      disabled={action.isBusy}
      onClick={toggle}
    >
      +{hand.plusCount || 1}
      <span className="sr-only"> me too, {hand.plusCount} so far</span>
    </button>
  );
}

/** A maintainer's actions on a waiting hand (FR-QUEUE-3, FR-MOD-1, FR-HAND-8). */
function RowActions({
  project,
  hand,
  canMerge,
  action,
}: {
  readonly project: Project;
  readonly hand: Hand;
  readonly canMerge: boolean;
  readonly action: Action;
}) {
  const login = `@${hand.author.login}`;
  const queueCall = (call: (id: string) => Promise<Queue>) =>
    void action.run(() => actOnQueue(() => call(hand.id)));
  const mute = () =>
    void action.run(
      () => muteMember(project, hand.author.userId, MUTE_MINUTES),
      `${login} can't post in the room for ${MUTE_MINUTES} minutes.`,
    );
  const isBusy = action.isBusy;

  return (
    <>
      <button
        type="button"
        className="btn ghost sm"
        disabled={isBusy}
        onClick={() => queueCall(skipHand)}
      >
        Skip
      </button>
      <AskOnce
        label="Remove"
        question={`Remove ${login}'s hand?`}
        confirm="Remove"
        isBusy={isBusy}
        onConfirm={() => queueCall(removeHand)}
      />
      {canMerge && (
        <button
          type="button"
          className="btn ghost sm"
          title="The same question as the one being answered"
          disabled={isBusy}
          onClick={() => queueCall(mergeHand)}
        >
          Merge into the current question
        </button>
      )}
      <button
        type="button"
        className="btn ghost sm"
        disabled={isBusy}
        onClick={mute}
      >
        Mute {MUTE_MINUTES} min
      </button>
    </>
  );
}
