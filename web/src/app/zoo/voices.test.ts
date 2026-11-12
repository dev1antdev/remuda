import { afterAll, describe, expect, it } from "vitest";
import { armVoices } from "./voices";

// FakeContext stands in for the browser's AudioContext: suspended until resumed, as one made outside a gesture is
const made: FakeContext[] = [];
class FakeContext {
  state: "suspended" | "running" = "suspended";
  constructor() {
    made.push(this);
  }
  resume() {
    this.state = "running";
    return Promise.resolve();
  }
  suspend() {
    this.state = "suspended";
    return Promise.resolve();
  }
}

const browsers = globalThis.AudioContext;
globalThis.AudioContext = FakeContext as unknown as typeof AudioContext;
afterAll(() => {
  globalThis.AudioContext = browsers;
});

describe("armVoices", () => {
  it("wakes the voices at the first click after a reload, lets the device go once disarmed, and wakes them again", () => {
    const page = new EventTarget();
    const disarm = armVoices(page, false);
    expect(made).toHaveLength(0); // no gesture yet: a context made now could not play
    page.dispatchEvent(new Event("pointerdown"));
    expect(made).toHaveLength(1);
    const context = made[0]!;
    expect(context.state).toBe("running");

    disarm();
    expect(context.state).toBe("suspended");
    page.dispatchEvent(new Event("keydown"));
    expect(context.state).toBe("suspended"); // switched off, a key wakes nothing

    const again = armVoices(page, true);
    expect(context.state).toBe("running"); // a page already used wakes at once, on the same context
    expect(made).toHaveLength(1);
    again();
    expect(context.state).toBe("suspended");
  });
});
