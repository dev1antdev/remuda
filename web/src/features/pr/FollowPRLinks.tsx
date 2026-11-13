// Over a pane's lenses a GitHub pull request's address in an agent's answer opens the pull request panel on it, in the
// pane's checkout; ⌘ or Ctrl with the click opens GitHub as before. Not for a viewer, whose page has no panel.

import { useMemo, type ReactNode } from "react";
import { actions } from "@/app/store";
import { FollowLinks, type Follow } from "@/ui/Markdown";
import { isPullURL } from "./links";

export default function FollowPRLinks({ machineId, dir, paneId, off, children }: { machineId: string; dir: string; paneId: string; off: boolean; children: ReactNode }) {
  const follow = useMemo<Follow | null>(
    () =>
      off || !dir
        ? null
        : (url) => {
            if (!isPullURL(url)) return false;
            actions.openPRPanel({ machineId, dir, paneId, url });
            return true;
          },
    [off, machineId, dir, paneId],
  );
  return <FollowLinks.Provider value={follow}>{children}</FollowLinks.Provider>;
}
