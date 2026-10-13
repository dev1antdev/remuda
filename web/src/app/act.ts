import { errMsg } from "@/lib/text";
import { actions } from "./store";

// rosterRefresh is set by the provider, so an action anywhere can ask for the roster again
let rosterRefresh: () => void = () => {};
export const setRosterRefresh = (fn: () => void) => {
  rosterRefresh = fn;
};

// act runs a change to the roster: the roster is read again on success, the reason shown on failure.
export const act = (p: Promise<unknown>): Promise<void> => p.then(() => rosterRefresh(), (e) => actions.notify(errMsg(e), { error: true }));
