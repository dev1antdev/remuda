import { describe, expect, it } from "vitest";
import { resultTone, resultWords } from "./runs";

describe("a schedule's last run", () => {
  it("says a skip in the page's words, else the server's result", () => {
    expect(resultWords({ skipped: "working", last_result: "skipped: its previous agent (w5:p2) is still working" })).toBe("skipped: the agent is still working");
    expect(resultWords({ skipped: "asking" })).toBe("skipped: the agent waits for an answer");
    expect(resultWords({ skipped: "running" })).toBe("skipped: the previous run is still on its way");
    expect(resultWords({ skipped: "limit", last_result: "skipped: Codex's 5 h window is at 97 % until Nov 12 16:50" })).toBe("skipped: its agent's plan window is nearly spent");
    expect(resultWords({ last_result: "sent to api · w3:p1" })).toBe("sent to api · w3:p1");
    expect(resultWords({})).toBe("");
  });
  it("colours a skip quietly, a run that went green, a failure red", () => {
    expect(resultTone({ skipped: "starting", last_ok: true })).toBe("skip");
    expect(resultTone({ last_ok: true })).toBe("ok");
    expect(resultTone({ last_ok: false })).toBe("bad");
  });
});
