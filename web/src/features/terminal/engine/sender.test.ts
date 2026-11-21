import { describe, expect, it } from "vitest";
import { createSender } from "./sender";
import type { Batch } from "./strokes";

// a send the test answers by hand, one request at a time
function harness() {
  const sent: Batch[] = [];
  const answers: { ok: () => void; fail: (e: unknown) => void }[] = [];
  const events: string[] = [];
  const sender = createSender(
    (batch) =>
      new Promise<void>((ok, fail) => {
        sent.push(batch);
        answers.push({ ok, fail });
      }),
    { sent: () => events.push("sent"), failed: (e) => events.push(`failed: ${String(e)}`) },
  );
  const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
  return { sender, sent, answers, events, tick };
}

describe("createSender", () => {
  it("sends the first stroke at once and coalesces the rest while it is out", async () => {
    const { sender, sent, answers, tick } = harness();
    sender.push({ text: "e" });
    expect(sent).toEqual([{ text: "e", paste: false, keys: [] }]);
    sender.push({ text: "ch" });
    sender.push({ text: "o" });
    sender.push({ key: "enter" });
    expect(sent.length).toBe(1); // never two in flight
    expect(sender.busy()).toBe(true);
    answers[0]!.ok();
    await tick();
    expect(sent[1]).toEqual({ text: "cho", paste: false, keys: ["enter"] });
    answers[1]!.ok();
    await tick();
    expect(sent.length).toBe(2);
    expect(sender.busy()).toBe(false);
  });
  it("drops what was queued behind a refusal, and goes on with what is typed after", async () => {
    const { sender, sent, answers, events, tick } = harness();
    sender.push({ text: "a" });
    sender.push({ text: "b" });
    answers[0]!.fail("gone");
    await tick();
    expect(events).toEqual(["failed: gone"]);
    expect(sent.length).toBe(1);
    sender.push({ key: "enter" });
    expect(sent[1]).toEqual({ text: "", paste: false, keys: ["enter"] });
  });
});
