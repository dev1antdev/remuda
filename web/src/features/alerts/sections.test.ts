import { describe, expect, it } from "vitest";
import { alertSections } from "./sections";

const known = { rosterKnown: true };
const none = { push: false, kinds: false, webhooks: false, sounds: false };

describe("alertSections", () => {
  it("offers a browser everything", () => {
    expect(alertSections({ native: false, viewOnly: false, ...known })).toEqual({ push: true, kinds: true, webhooks: true, sounds: true });
  });
  it("leaves Web Push and the page's sounds out of the Mac app, which brings its own notifications", () => {
    expect(alertSections({ native: true, viewOnly: false, ...known })).toEqual({ push: false, kinds: true, webhooks: true, sounds: false });
  });
  it("leaves a viewer screen only what stays on it, in a browser or in the app", () => {
    expect(alertSections({ native: false, viewOnly: true, ...known })).toEqual(none);
    expect(alertSections({ native: true, viewOnly: true, ...known })).toEqual(none);
  });
  it("waits for the roster before offering what a viewer could not use", () => {
    expect(alertSections({ native: false, viewOnly: false, rosterKnown: false })).toEqual(none);
  });
});
