// useBadge keeps the browser tab in step with the roster: the title counts the agents that need the person and the favicon
// wears a dot in the most pressing one's colour, taken from the page's own tokens so it follows the theme. The icon is
// loaded once and redrawn only when the count, the tone or the theme changes; a tab with nobody waiting gets the plain icon.

import { useEffect } from "react";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { badgeOf, titleFor, type BadgeTone } from "./badge";
import { useTheme } from "./theme";

export const ICON_HREF = "/icons/icon-192.png";
const SIZE = 64;
const FALLBACK: Record<BadgeTone, string> = { red: "#e78284", amber: "#e5c890", green: "#a6d189" };

let icon: Promise<HTMLImageElement> | null = null;

function loadIcon(): Promise<HTMLImageElement> {
  icon ??= new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("the icon did not load"));
    image.src = ICON_HREF;
  });
  return icon;
}

function iconLink(): HTMLLinkElement {
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    document.head.appendChild(link);
  }
  return link;
}

// token reads a colour off the root, so the dot is the theme's own red, amber, green or background
function token(name: string, fallback: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

// drawBadge is the icon with a filled dot in the bottom-right corner, ringed in the page's background so it reads on either theme
function drawBadge(image: HTMLImageElement, tone: BadgeTone): string | null {
  const canvas = document.createElement("canvas");
  canvas.width = SIZE;
  canvas.height = SIZE;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.drawImage(image, 0, 0, SIZE, SIZE);
  const radius = SIZE / 4;
  const x = SIZE - radius - 2;
  const y = SIZE - radius - 2;
  ctx.beginPath();
  ctx.arc(x, y, radius + 3, 0, Math.PI * 2);
  ctx.fillStyle = token("--bg", "#303446");
  ctx.fill();
  ctx.beginPath();
  ctx.arc(x, y, radius, 0, Math.PI * 2);
  ctx.fillStyle = token(`--${tone}`, FALLBACK[tone]);
  ctx.fill();
  return canvas.toDataURL("image/png");
}

export function useBadge(): void {
  const { machines } = useRoster();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const theme = useTheme();
  const { count, tone } = badgeOf(machines, radar, Date.now());

  useEffect(() => {
    document.title = titleFor(count);
  }, [count]);

  useEffect(() => {
    if (!tone) {
      iconLink().href = ICON_HREF;
      return;
    }
    let stale = false;
    void loadIcon()
      .then((image) => {
        if (stale) return;
        const url = drawBadge(image, tone);
        if (url) iconLink().href = url;
      })
      .catch(() => {});
    return () => {
      stale = true;
    };
  }, [tone, theme]);
}
