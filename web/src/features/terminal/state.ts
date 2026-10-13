// What the terminal lens, its tools in the tab strip and its key bar share: one terminal is on screen at a time.

import { createStore } from "@/lib/store";
import type { Modifier, TermApi } from "./engine/types";

export const terminal = createStore<{
  api: TermApi | null; // the live terminal's input path, for the key bar
  modifier: Modifier; // a one-shot Ctrl or Alt from the key bar
  selectMode: boolean; // the ⬚ layer whose selection is the browser's own
}>({ api: null, modifier: null, selectMode: false });
