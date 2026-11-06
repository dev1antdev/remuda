// What the alerts panel offers on this screen. Web Push and the kinds it carries belong to a signed-in browser — the Mac
// app brings its own notifications — and the webhooks to whoever may change them; a viewer link's screen keeps what
// stays on it, the voice and the lens a row opens. Until the roster has answered nobody knows which screen this is,
// so only those two show: a panel opened from the address bar must not ask a viewer's refused routes in the meantime.

export type AlertSections = { push: boolean; kinds: boolean; webhooks: boolean };

export function alertSections({ native, viewOnly, rosterKnown }: { native: boolean; viewOnly: boolean; rosterKnown: boolean }): AlertSections {
  const mayChange = rosterKnown && !viewOnly;
  return { push: mayChange && !native, kinds: mayChange, webhooks: mayChange };
}
