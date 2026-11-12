import { useMemo, useSyncExternalStore } from "react";

/** A page of the web app. The FAQ pages are not here: the Go server renders those. */
export type Route =
  | { readonly name: "landing" }
  | { readonly name: "dashboard" }
  | { readonly name: "room"; readonly owner: string; readonly repo: string }
  | { readonly name: "notFound" };

// The same patterns as the server (NFR-SEC-3). Here they only spare a request that would be refused.
const OWNER = /^[A-Za-z0-9-]{1,39}$/;
const REPO = /^[A-Za-z0-9._-]{1,100}$/;

/** Whether owner/repo can be a GitHub repository. "." and ".." fit the pattern but are none. */
export function isRepoName(owner: string, repo: string): boolean {
  return OWNER.test(owner) && REPO.test(repo) && repo !== "." && repo !== "..";
}

export function matchRoute(pathname: string): Route {
  const path = pathname.length > 1 ? pathname.replace(/\/$/, "") : pathname;
  if (path === "/") return { name: "landing" };
  if (path === "/dashboard") return { name: "dashboard" };

  const parts = path.split("/");
  const [, prefix, owner, repo] = parts;
  const isProject =
    parts.length === 4 && prefix === "p" && isRepoName(owner, repo);
  return isProject ? { name: "room", owner, repo } : { name: "notFound" };
}

const listeners = new Set<() => void>();

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  window.addEventListener("popstate", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("popstate", listener);
  };
}

/** Goes to another page of the app without a reload. */
export function navigate(to: string): void {
  window.history.pushState(null, "", to);
  window.scrollTo(0, 0);
  listeners.forEach((listener) => listener());
}

export function useRoute(): Route {
  const pathname = useSyncExternalStore(
    subscribe,
    () => window.location.pathname,
  );
  return useMemo(() => matchRoute(pathname), [pathname]);
}
