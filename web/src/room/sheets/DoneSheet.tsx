import { useEffect } from "react";

import type { Project } from "../../projects";
import type { Hand } from "../../queue/types";

type Props = {
  readonly project: Project;
  /** The hand being answered. */
  readonly hand: Hand;
  readonly onClose: () => void;
};

/**
 * A maintainer finishes the hand being answered: an answer summary and optional links, which
 * become a FAQ entry (FR-FAQ-1). The props are a contract (notes/phase-2-plan.md); the sheet is
 * the sheets-and-head part of the build (worker 6). Until then it closes itself.
 */
export function DoneSheet({ onClose }: Props) {
  useEffect(onClose, [onClose]);
  return null;
}
