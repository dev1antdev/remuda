// The workspace whose card details a touch screen asked for — its ⋯ menu's Details, a tap on its pull request — by
// "machine|workspace": the card opens them as a popover beside itself, since a finger has no hover.

import { createStore } from "@/lib/store";

const asked = createStore<{ key: string | null }>({ key: null });

const detailsKey = (machineId: string, sessionId: string) => `${machineId}|${sessionId}`;

export const askDetails = (machineId: string, sessionId: string): void => asked.set({ key: detailsKey(machineId, sessionId) });

export const dropDetails = (): void => asked.set({ key: null });

export const useDetailsAsked = (machineId: string, sessionId: string): boolean => asked.use((s) => s.key === detailsKey(machineId, sessionId));
