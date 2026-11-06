// The body a menu or a tooltip opens in: the pop-out's own for a piece drawn in that window, else the page's. Without it a
// tile's menu in the pop-out opened in the main window, out of sight, and its tooltips appeared there at random places.

import { createContext, useContext } from "react";

export const PortalHost = createContext<HTMLElement | null>(null);

// usePortalHost is the container Radix's portals take; undefined lets them open in the page's own body
export const usePortalHost = (): HTMLElement | undefined => useContext(PortalHost) ?? undefined;
