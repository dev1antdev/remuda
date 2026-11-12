// Inside herdr's server calls: the two switches and the toasts' quiet hours, the snippet for herdr's config, a test toast.

import type { Quiet } from "@/features/alerts/push";
import { getJSON, sendJSON } from "@/lib/http";

export type MirrorSettings = { figures: boolean; toasts: boolean; quiet?: Quiet };
export type ToastOutcome = { machine: string; label: string; at: string; title: string; shown: boolean; reason?: string; error?: string };
export type Yield = { machine: string; label: string; writer: string };
export type MirrorStatus = MirrorSettings & { closed?: string; tokens: string[]; snippet: string; toast_snippet: string; last?: ToastOutcome[]; elsewhere?: Yield[] };

export const fetchMirror = (): Promise<MirrorStatus> => getJSON("/api/mirror");
export const saveMirror = (settings: MirrorSettings): Promise<MirrorStatus> => sendJSON("PUT", "/api/mirror", settings);
export const testMirror = (): Promise<ToastOutcome[]> => sendJSON("POST", "/api/mirror/test");
