// After a deployment, a page that was already open asks for script files that are gone: their
// names carry a hash of the old code, and the server answers with the app's page instead. Vite
// reports that with a "vite:preloadError" event. The cure is the new page, so the page reloads.
const REMEMBERED_AS = "oh_reloaded_at";
const QUIET_MS = 10_000;

/** Whether the page may reload now: not if it did so a moment ago, or a file that keeps failing would loop it. */
export function mayReload(lastReloadAt: string | null, now: number): boolean {
  const last = Number(lastReloadAt);
  return !lastReloadAt || !Number.isFinite(last) || now - last > QUIET_MS;
}

export function reloadWhenCodeIsGone(): void {
  window.addEventListener("vite:preloadError", () => {
    try {
      const now = Date.now();
      if (!mayReload(sessionStorage.getItem(REMEMBERED_AS), now)) return;
      sessionStorage.setItem(REMEMBERED_AS, String(now));
    } catch {
      return; // no storage to remember the reload in: don't risk a loop
    }
    window.location.reload();
  });
}
