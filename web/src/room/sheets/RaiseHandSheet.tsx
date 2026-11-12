import { useEffect } from "react";

import type { Project } from "../../projects";
import type { QueueSession } from "../../queue/types";

type Props = {
  readonly project: Project;
  /** The live session the hand is raised in. */
  readonly session: QueueSession;
  readonly onClose: () => void;
};

/**
 * A member raises a hand: a title and optional details, with up to 3 similar FAQ entries shown
 * while they type (FR-HAND-1, FR-HAND-3). The props are a contract (notes/phase-2-plan.md); the
 * sheet is the sheets-and-head part of the build (worker 6). Until then it closes itself.
 */
export function RaiseHandSheet({ onClose }: Props) {
  useEffect(onClose, [onClose]);
  return null;
}
