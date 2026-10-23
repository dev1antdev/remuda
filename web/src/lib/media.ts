import { useCallback, useSyncExternalStore } from "react";

// useMedia follows a media query, for a piece the stylesheet hides that would otherwise go on polling unseen.
export function useMedia(query: string): boolean {
  const subscribe = useCallback(
    (notify: () => void) => {
      const list = matchMedia(query);
      list.addEventListener("change", notify);
      return () => list.removeEventListener("change", notify);
    },
    [query],
  );
  return useSyncExternalStore(subscribe, () => matchMedia(query).matches, () => false);
}
