// useFocused follows whether this window has the focus: the status bar reads its machine quicker while it does.

import { useSyncExternalStore } from "react";

function subscribe(notify: () => void): () => void {
  window.addEventListener("focus", notify);
  window.addEventListener("blur", notify);
  return () => {
    window.removeEventListener("focus", notify);
    window.removeEventListener("blur", notify);
  };
}

export function useFocused(): boolean {
  return useSyncExternalStore(subscribe, () => document.hasFocus(), () => true);
}
