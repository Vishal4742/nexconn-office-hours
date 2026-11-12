import { useEffect } from "react";

import type { Project } from "../../projects";

type Props = {
  readonly project: Project;
  readonly onClose: () => void;
};

/**
 * A maintainer starts a session: a title and an optional length (FR-SESS-1). On success it gives
 * the answer to acceptQueue and closes. The props are a contract (notes/phase-2-plan.md); the
 * sheet is the sheets-and-head part of the build (worker 6). Until then it closes itself.
 */
export function StartSessionSheet({ onClose }: Props) {
  useEffect(onClose, [onClose]);
  return null;
}
