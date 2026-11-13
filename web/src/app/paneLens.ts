// A pane's lenses: an agent's conversation, its terminal, its folder's changes and its checkout's history (Git). A shell
// — a pane with no agent — has its terminal alone: there is no conversation to read, and its folder's changes and
// history are an agent tab's or the Review mode's. The store keeps the lens asked for, so an agent's tab comes back on it after a shell's.

import type { Lens } from "./store";

const ALL: Lens[] = ["chat", "terminal", "changes", "git"];

// lensesOf lists the lenses a pane has, by its agent ("" for a shell)
export const lensesOf = (agent: string): Lens[] => (agent ? ALL : ["terminal"]);

// lensOn is the lens a pane shows for the one asked: that one when the pane has it, else its terminal
export const lensOn = (asked: Lens, agent: string): Lens => (lensesOf(agent).includes(asked) ? asked : "terminal");

// keepsTerminal says whether a pane keeps its terminal as an agent starts in it: one that came on screen as a shell —
// the person typed `claude` at its prompt and is looking at it start — and not an agent's pane that was a shell for a
// moment (a restart leaves its agent and resumes it), which goes back to the lens asked
export const keepsTerminal = (openedAsShell: boolean, agent: string): boolean => openedAsShell && !!agent;
