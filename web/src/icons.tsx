// The UI's icons: a small set of 24×24 stroke paths drawn inline, so they
// take the text colour and scale with the font. No emoji, no glyphs from
// whatever font the platform falls back to.

import type { SVGProps } from "react";

const PATHS = {
  menu: "M4 6h16M4 12h16M4 18h16",
  bell: "M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10.3 21a1.94 1.94 0 0 0 3.4 0",
  sliders: "M21 7h-6M9 7H3M21 17h-10M5 17H3M15 4v6M7 14v6",
  plus: "M5 12h14M12 5v14",
  x: "M18 6 6 18M6 6l12 12",
  "chevron-down": "m6 9 6 6 6-6",
  "chevron-right": "m9 18 6-6-6-6",
  "chevron-left": "m15 18-6-6 6-6",
  "chevron-up": "m18 15-6-6-6 6",
  more: "M5 12h.01M12 12h.01M19 12h.01",
  search: "M21 21l-4.3-4.3M19 11a8 8 0 1 1-16 0 8 8 0 0 1 16 0z",
  send: "m22 2-7 20-4-9-9-4Zm0 0L11 13",
  square: "M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z",
  paperclip: "m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48",
  keyboard: "M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2zM6 8h.01M10 8h.01M14 8h.01M18 8h.01M8 12h.01M12 12h.01M16 12h.01M7 16h10",
  "text-smaller": "M4 7V4h12v3M10 4v14M7 18h6M17 14h5",
  "text-larger": "M4 7V4h12v3M10 4v14M7 18h6M17 14h5M19.5 11.5v5",
  maximize: "M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7",
  minimize: "M4 14h6v6M20 10h-6V4M14 10l7-7M3 21l7-7",
  "move-horizontal": "m18 8 4 4-4 4M6 8l-4 4 4 4M2 12h20",
  "text-select": "M5 3a2 2 0 0 0-2 2M19 3a2 2 0 0 1 2 2M21 19a2 2 0 0 1-2 2M5 21a2 2 0 0 1-2-2M9 3h1M9 21h1M14 3h1M14 21h1M3 9v1M21 9v1M3 14v1M21 14v1",
  sun: "M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0z",
  calendar: "M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z",
  "circle-check": "M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0zm-13 0 2 2 4-4",
  filter: "M22 3H2l8 9.46V19l4 2v-8.54L22 3z",
  "external-link": "M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6",
  trash: "M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M10 11v6M14 11v6",
  check: "M20 6 9 17l-5-5",
  "arrow-up-down": "m21 16-4 4-4-4M17 20V4M3 8l4-4 4 4M7 4v16",
  list: "M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01",
  layers: "m12 2 10 5-10 5L2 7l10-5zM2 17l10 5 10-5M2 12l10 5 10-5",
  monitor: "M4 3h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM8 21h8M12 17v4",
  "corner-down-left": "m9 10-5 5 5 5M20 4v7a4 4 0 0 1-4 4H4",
  inbox: "M22 12h-6l-2 3h-4l-2-3H2M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z",
  "log-out": "M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9",
  file: "M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2zM14 2v6h6",
  "alert-triangle": "m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3zM12 9v4M12 17h.01",
  terminal: "m4 17 6-6-6-6M12 19h8",
  sparkles: "m12 3 1.9 5.6L19.5 10.5l-5.6 1.9L12 18l-1.9-5.6L4.5 10.5l5.6-1.9L12 3zM19 17l.7 2.3L22 20l-2.3.7L19 23l-.7-2.3L16 20l2.3-.7L19 17z",
  zap: "M13 2 3 14h9l-1 8 10-12h-9l1-8z",
  folder: "M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z",
} as const satisfies Record<string, string>;

export type IconName = keyof typeof PATHS;

export default function Icon({ name, size = 16, strokeWidth = 1.75, ...rest }: { name: IconName; size?: number; strokeWidth?: number } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      className="icon"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...rest}
    >
      <path d={PATHS[name]} />
    </svg>
  );
}
