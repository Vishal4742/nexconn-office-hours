// After a deployment, a page that was already open asks for script files that are gone: their
// names carry a hash of the old code, and the server answers with the app's page instead. Vite
// reports that with a "vite:preloadError" event. The cure is the new page, so the page reloads,
// once per tab: the same event comes for a download that keeps failing (a bad network, code an old
// browser can't run), and reloading for that loops the room and wipes what was being typed.
const REMEMBERED_AS = "oh_reloaded_at";

/** Whether the page may reload now: not if this tab reloaded for this already, nor offline. */
export function mayReload(
  remembered: string | null,
  isOnline: boolean,
): boolean {
  return isOnline && remembered === null;
}

export function reloadWhenCodeIsGone(): void {
  window.addEventListener("vite:preloadError", () => {
    try {
      if (!mayReload(sessionStorage.getItem(REMEMBERED_AS), navigator.onLine))
        return;
      sessionStorage.setItem(REMEMBERED_AS, String(Date.now()));
    } catch {
      return; // no storage to remember the reload in: don't risk a loop
    }
    window.location.reload();
  });
}
