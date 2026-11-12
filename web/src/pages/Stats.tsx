import type { RepoName } from "../projects";

type Props = {
  /** One project's numbers, or those of every project together when null. */
  readonly repo: RepoName | null;
};

/**
 * S6, /stats and /p/owner/repo/stats: sessions held, hands raised, hands answered, FAQ entries
 * and participants (FR-MET-1), from loadStats (stats.ts). Public. The props are a contract
 * (notes/phase-2-plan.md); the page is the cards-gate-and-stats part of the build (worker 9).
 */
export function Stats({ repo }: Props) {
  const what = repo ? `${repo.owner}/${repo.repo}` : "Office Hours";
  return (
    <div className="wrap page">
      <h1>{what} in numbers</h1>
      <p className="empty">The numbers arrive with the next update.</p>
    </div>
  );
}
