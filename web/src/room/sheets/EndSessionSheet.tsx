import { useEffect } from "react";

import type { QueueSession } from "../../queue/types";

type Props = {
  readonly session: QueueSession;
  /** How many hands are still waiting: what the carry-over choice is about. */
  readonly waitingCount: number;
  readonly onClose: () => void;
};

/**
 * "End this session?" with the carry-over choice, ticked by default (FR-SESS-4, FR-SESS-7). The
 * props are a contract (notes/phase-2-plan.md); the sheet is the sheets-and-head part of the
 * build (worker 6). Until then it closes itself.
 */
export function EndSessionSheet({ onClose }: Props) {
  useEffect(onClose, [onClose]);
  return null;
}
