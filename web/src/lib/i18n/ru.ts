// The Russian dictionary: the English string the page is written in → its Russian, or the three forms of a counted
// phrase (one, few, many). Kept by area so each file stays readable; this file joins them.

import type { Dictionary } from "../i18n";
import { agents } from "./ru/agents";
import { chat } from "./ru/chat";
import { panels } from "./ru/panels";
import { pieces } from "./ru/pieces";
import { recover } from "./ru/recover";
import { review } from "./ru/review";
import { settings } from "./ru/settings";
import { shell } from "./ru/shell";
import { tasks } from "./ru/tasks";
import { terminal } from "./ru/terminal";
import { webhooks } from "./ru/webhooks";
import { awake } from "./ru/awake";
import { pr } from "./ru/pr";
import { complete } from "./ru/complete";
import { macshell } from "./ru/macshell";
import { tablecopy } from "./ru/tablecopy";
import { machineaway } from "./ru/machineaway";
import { modechip } from "./ru/modechip";
import { mute } from "./ru/mute";
import { bulkstart } from "./ru/bulkstart";
import { wake } from "./ru/wake";
import { activity } from "./ru/activity";
import { viewonly } from "./ru/viewonly";
import { splitview } from "./ru/splitview";
import { shellwords } from "./ru/shellwords";
import { sidebar } from "./ru/sidebar";
import { worktreeinclude } from "./ru/worktreeinclude";
import { quote } from "./ru/quote";
import { reopen } from "./ru/reopen";
import { forkchat } from "./ru/forkchat";
import { autoicon } from "./ru/autoicon";
import { gitlab } from "./ru/gitlab";
import { teleport } from "./ru/teleport";
import { arena } from "./ru/arena";
import { limits } from "./ru/limits";
import { floatterm } from "./ru/floatterm";
import { screenshare } from "./ru/screenshare";
import { jira } from "./ru/jira";
import { mermaid } from "./ru/mermaid";
import { zoo } from "./ru/zoo";
import { auditchat } from "./ru/auditchat";

export const ru: Dictionary = { ...pieces, ...shell, ...agents, ...chat, ...terminal, ...review, ...tasks, ...settings, ...panels, ...webhooks, ...awake, ...pr, ...complete, ...recover, ...macshell, ...tablecopy, ...machineaway, ...modechip, ...mute, ...bulkstart, ...wake, ...activity, ...viewonly, ...splitview, ...shellwords, ...sidebar, ...worktreeinclude, ...quote, ...reopen, ...forkchat, ...autoicon, ...gitlab, ...teleport, ...arena, ...limits, ...floatterm, ...screenshare, ...jira, ...mermaid, ...zoo, ...auditchat };
