// FR-NOTIF-2: "You're next" and "It's your turn" as a system notification, for a page that is not
// in front, as Meet and Zoom tell people. In front, the banner and the tab title say it already.
import type { TurnAlert } from "../queue/turnAlert";

export type NotifyState = "unsupported" | NotificationPermission;

/** One tag for both notices, so "It's your turn" replaces "You're next" instead of stacking. */
const TAG = "office-hours-turn";

/** Whether this browser can show notifications to this page, and whether it may. */
export function notifyState(): NotifyState {
  return typeof Notification === "undefined"
    ? "unsupported"
    : Notification.permission;
}

/** Asks the browser for permission. Call it from a click: some browsers refuse otherwise. */
export async function askToNotify(): Promise<NotifyState> {
  if (typeof Notification === "undefined") return "unsupported";
  try {
    return await Notification.requestPermission();
  } catch {
    return notifyState();
  }
}

/** Shows the notice for my hand, if this page is in the background and may notify. */
export function showTurnNotice(alert: TurnAlert, room: string): void {
  if (notifyState() !== "granted") return;
  if (typeof document === "undefined" || document.visibilityState === "visible")
    return;
  const { hand } = alert;
  const title =
    alert.kind === "turn"
      ? `It's your turn in ${room}`
      : `You're next in ${room}`;
  const body =
    alert.kind === "turn" && hand.takenBy
      ? `@${hand.takenBy.login} is answering: ${hand.title}`
      : hand.title;
  try {
    const notice = new Notification(title, { body, tag: TAG });
    notice.onclick = () => {
      window.focus();
      notice.close();
    };
  } catch {
    // Chrome on Android lets only a service worker show notifications. The tab title and the
    // banner still say it, as they do everywhere.
  }
}
