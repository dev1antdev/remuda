// The notifications taken back: the roster lists, for a day, the tags of the alerts that no longer stand — a question
// answered at the desk or on another device, a finish looked at, a pane moved on — and this page closes them in the
// browser it runs in, wherever the system still shows them. Opened the morning after, it clears the night's.

// ShownNotification is what the browser hands back of a notification it shows
export type ShownNotification = { tag: string; close: () => void };

// staleOf are the notifications shown under a tag taken back
export function staleOf<T extends Pick<ShownNotification, "tag">>(shown: readonly T[], withdrawn: readonly string[]): T[] {
  if (withdrawn.length === 0) return [];
  const gone = new Set(withdrawn);
  return shown.filter((notification) => notification.tag !== "" && gone.has(notification.tag));
}

// closeWithdrawn closes them through the service worker's registration; nothing where the browser has none
export async function closeWithdrawn(withdrawn: readonly string[]): Promise<number> {
  if (withdrawn.length === 0 || typeof navigator === "undefined" || !("serviceWorker" in navigator)) return 0;
  try {
    const registration = await navigator.serviceWorker.getRegistration();
    if (!registration?.getNotifications) return 0;
    const stale = staleOf(await registration.getNotifications(), withdrawn);
    for (const notification of stale) notification.close();
    return stale.length;
  } catch {
    return 0;
  }
}
