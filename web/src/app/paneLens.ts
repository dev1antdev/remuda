// A pane's lenses: an agent's conversation, its terminal and its folder's changes. A shell — a pane with no agent —
// has its terminal alone: there is no conversation to read, and its folder's changes are an agent tab's or the Review
// mode's. The store keeps the lens asked for, so an agent's tab comes back on it after a shell's.

import type { Lens } from "./store";

const ALL: Lens[] = ["chat", "terminal", "changes"];

// lensesOf lists the lenses a pane has, by its agent ("" for a shell)
export const lensesOf = (agent: string): Lens[] => (agent ? ALL : ["terminal"]);

// lensOn is the lens a pane shows for the one asked: that one when the pane has it, else its terminal
export const lensOn = (asked: Lens, agent: string): Lens => (lensesOf(agent).includes(asked) ? asked : "terminal");
