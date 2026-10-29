// The Russian dictionary: the English string the page is written in → its Russian, or the three forms of a counted
// phrase (one, few, many). Kept by area so each file stays readable; this file joins them.

import type { Dictionary } from "../i18n";
import { agents } from "./ru/agents";
import { chat } from "./ru/chat";
import { panels } from "./ru/panels";
import { pieces } from "./ru/pieces";
import { review } from "./ru/review";
import { settings } from "./ru/settings";
import { shell } from "./ru/shell";
import { tasks } from "./ru/tasks";
import { terminal } from "./ru/terminal";
import { pr } from "./ru/pr";

export const ru: Dictionary = { ...pieces, ...shell, ...agents, ...chat, ...terminal, ...review, ...tasks, ...settings, ...panels, ...pr };
