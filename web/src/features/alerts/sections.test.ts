import { describe, expect, it } from "vitest";
import { alertSections } from "./sections";

const known = { rosterKnown: true };

describe("alertSections", () => {
  it("offers a browser everything", () => {
    expect(alertSections({ native: false, viewOnly: false, ...known })).toEqual({ push: true, kinds: true, webhooks: true });
  });
  it("leaves Web Push out of the Mac app, which brings its own notifications", () => {
    expect(alertSections({ native: true, viewOnly: false, ...known })).toEqual({ push: false, kinds: true, webhooks: true });
  });
  it("leaves a viewer screen only what stays on it, in a browser or in the app", () => {
    expect(alertSections({ native: false, viewOnly: true, ...known })).toEqual({ push: false, kinds: false, webhooks: false });
    expect(alertSections({ native: true, viewOnly: true, ...known })).toEqual({ push: false, kinds: false, webhooks: false });
  });
  it("waits for the roster before offering what a viewer could not use", () => {
    expect(alertSections({ native: false, viewOnly: false, rosterKnown: false })).toEqual({ push: false, kinds: false, webhooks: false });
  });
});
