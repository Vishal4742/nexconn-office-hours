import { useEffect, useState } from "react";

import { errorMessage } from "../api";
import { Link } from "../components/Link";
import { faqPath } from "../faq";
import { roomPath } from "../projects";
import type { RepoName } from "../projects";
import { loadStats } from "../stats";
import type { Stats as Counts } from "../stats";
import { hasNothingYet, statTiles } from "../stats/numbers";

type Props = {
  /** One project's numbers, or those of every project together when null. */
  readonly repo: RepoName | null;
};

type Load =
  | { readonly status: "loading" }
  | { readonly status: "ready"; readonly counts: Counts }
  | { readonly status: "failed"; readonly message: string };

const TILES = 5;

/**
 * S6, /stats and /p/owner/repo/stats: sessions held, hands raised, hands answered, FAQ entries
 * and participants (FR-MET-1), from loadStats (stats.ts). Public. The props are a contract
 * (notes/phase-2-plan.md).
 */
export function Stats({ repo }: Props) {
  const [load, setLoad] = useState<Load>({ status: "loading" });
  const [loads, setLoads] = useState(0);
  // App hands over a new object on every render: the effect follows the names, not the object.
  const owner = repo?.owner ?? "";
  const name = repo?.repo ?? "";

  useEffect(() => {
    let isCurrent = true;
    loadStats(owner ? { owner, repo: name } : null).then(
      (counts) => {
        if (isCurrent) setLoad({ status: "ready", counts });
      },
      (failure: unknown) => {
        if (isCurrent) {
          setLoad({ status: "failed", message: errorMessage(failure) });
        }
      },
    );
    return () => {
      isCurrent = false;
    };
  }, [owner, name, loads]);

  function handleRetry() {
    setLoad({ status: "loading" });
    setLoads((count) => count + 1);
  }

  // With nothing counted yet the room is the one place to go: its FAQ is still empty.
  const isEmpty = load.status === "ready" && hasNothingYet(load.counts);
  return (
    <div className="wrap page">
      <h1 className="stats-title">
        {repo ? `${repo.owner}/${repo.repo}` : "Office Hours"} in numbers
      </h1>
      <p className="lead">
        {repo
          ? "Counted from every session held in this room."
          : "Counted from every room on Office Hours."}
      </p>
      {repo && !isEmpty && (
        <div className="actions">
          <Link href={roomPath(repo)} className="btn ghost">
            Open the room
          </Link>
          <a href={faqPath(repo)} className="btn ghost">
            Read the FAQ
          </a>
        </div>
      )}
      <StatsBody load={load} repo={repo} onRetry={handleRetry} />
    </div>
  );
}

type StatsBodyProps = {
  readonly load: Load;
  readonly repo: RepoName | null;
  readonly onRetry: () => void;
};

function StatsBody({ load, repo, onRetry }: StatsBodyProps) {
  if (load.status === "loading") {
    return (
      <div className="stats-grid" role="status">
        <span className="sr-only">Loading the numbers…</span>
        {Array.from({ length: TILES }, (_, index) => (
          <div key={index} className="skel" />
        ))}
      </div>
    );
  }
  if (load.status === "failed") {
    return (
      <p className="notice" role="alert">
        <span>{load.message}</span>
        <button type="button" className="btn ghost sm" onClick={onRetry}>
          Try again
        </button>
      </p>
    );
  }
  if (hasNothingYet(load.counts)) {
    return (
      <div className="empty">
        <p>
          {repo
            ? "No session has been held in this room yet. Its numbers start with the first one."
            : "No session has been held on Office Hours yet. The numbers start with the first one."}
        </p>
        {repo && (
          <div className="actions">
            <Link href={roomPath(repo)} className="btn primary">
              Open the room
            </Link>
          </div>
        )}
      </div>
    );
  }
  return (
    <ul className="stats-grid">
      {statTiles(load.counts).map((tile) => (
        <li key={tile.key} className="stats-tile">
          <span className="stats-num">{tile.value}</span>{" "}
          <span className="stats-name">{tile.name}</span>
        </li>
      ))}
    </ul>
  );
}
