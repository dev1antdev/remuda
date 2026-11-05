// What the alerts panel says of this device, truthfully: the browser and the system, whether the browser lets the site
// notify and where to change it, where the system itself may hide what the browser shows, the last push this cockpit
// handed to the browser's push service and the last one the browser received. A push can be sent and taken, received,
// and still never appear: those three lines tell where it stopped.

import { ago } from "@/lib/time";
import { t } from "@/lib/i18n";

export type Browser = "Firefox" | "Edge" | "Chrome" | "Safari" | "browser";
export type System = "macOS" | "Windows" | "Linux" | "iOS" | "Android" | "system";

// Delivery is what the server last handed this browser's push service, and its answer ("" taken)
export type Delivery = { at: string; kind: string; title: string; error?: string };

// Received is the last push the service worker saw arrive, and where it was shown
export type Received = { at: number; title: string; kind: string; shown: "os" | "page" };

// browserOf reads the browser off the user agent, Edge and Chrome before Safari, whose name both carry
export function browserOf(ua: string): Browser {
  if (/Edg\//.test(ua)) return "Edge";
  if (/Firefox\//.test(ua)) return "Firefox";
  if (/Chrome\/|Chromium\//.test(ua)) return "Chrome";
  if (/Safari\//.test(ua)) return "Safari";
  return "browser";
}

// systemOf reads the system; an iPad that says it is a Mac gives itself away by its touch points
export function systemOf(ua: string, touchPoints = 0): System {
  if (/iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && touchPoints > 1)) return "iOS";
  if (/Android/.test(ua)) return "Android";
  if (/Macintosh|Mac OS X/.test(ua)) return "macOS";
  if (/Windows/.test(ua)) return "Windows";
  if (/Linux|X11/.test(ua)) return "Linux";
  return "system";
}

// blockedHelp is where the browser lets the site notify again once the person said no
export function blockedHelp(browser: Browser): string {
  switch (browser) {
    case "Firefox":
      return t("Firefox blocks notifications for this site: click the icon left of the address, then Permissions → Send notifications → Allow, and reload.");
    case "Safari":
      return t("Safari blocks notifications for this site: Safari → Settings → Websites → Notifications → this site → Allow, and reload.");
    case "Chrome":
    case "Edge":
      return t("{browser} blocks notifications for this site: click the icon left of the address → Site settings → Notifications → Allow, and reload.", { browser });
    default:
      return t("The browser blocks notifications for this site: allow them in its site settings, and reload.");
  }
}

// systemHelp is where the system may keep what the browser shows from appearing, even with everything allowed here
export function systemHelp(browser: Browser, system: System): string {
  const name = browser === "browser" ? t("the browser") : browser;
  switch (system) {
    case "macOS":
      return t("Nothing appears? System Settings → Notifications → {browser}: Allow notifications, as Banners or Alerts — and Focus off.", { browser: name });
    case "Windows":
      return t("Nothing appears? Settings → System → Notifications: {browser} on — and Do not disturb off.", { browser: name });
    case "Linux":
      return t("Nothing appears? Your desktop's notification settings must let {browser} show them, with Do not disturb off.", { browser: name });
    case "iOS":
      return t("On iPhone and iPad notifications come only to remuda added to the Home Screen; Settings → Notifications → remuda lets them through.");
    case "Android":
      return t("Nothing appears? Settings → Apps → {browser} → Notifications must be on.", { browser: name });
    default:
      return t("Nothing appears? The system's notification settings must let {browser} show them.", { browser: name });
  }
}

// deliveryWords is the last push this cockpit handed the browser's push service, and what the service said
export function deliveryWords(last: Delivery | null | undefined, now = Date.now()): string {
  if (!last) return t("No push sent to this browser yet.");
  const when = ago(last.at, now);
  return last.error ? t("Last push sent {when}: {title} — refused: {error}", { when, title: last.title, error: last.error }) : t("Last push sent {when}: {title} — the push service took it.", { when, title: last.title });
}

// receivedWords is the last push the browser received, and whether the system or the page showed it
export function receivedWords(last: Received | null | undefined, now = Date.now()): string {
  if (!last) return t("No push received here yet.");
  const when = ago(last.at, now);
  return last.shown === "page" ? t("Last push received here {when}: {title} — shown on the page, which had the focus.", { when, title: last.title }) : t("Last push received here {when}: {title} — handed to the system to show.", { when, title: last.title });
}
