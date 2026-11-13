import { describe, expect, it } from "vitest";
import { heldWords, outcomeWords, pushAnywayAsk, ranWords, runWords } from "./outcome";
import { FETCH_AT_ONCE, pooled } from "./pool";
import type { Outcome } from "./syncApi";

const said = (o: Partial<Outcome> & Pick<Outcome, "action" | "state">) => outcomeWords(o as Outcome, "app");

describe("outcomeWords", () => {
  it("says what was done", () => {
    expect(said({ action: "fetch", state: "done", repo: { path: "/a", name: "app", parent: "~", ahead: 0, behind: 3, dirty: 0, remote: true } })).toEqual({ text: "Fetched app: 3 not pulled", error: false });
    expect(said({ action: "pull", state: "done", count: 2 })).toEqual({ text: "Pulled 2 commits into app", error: false });
    expect(said({ action: "pull", state: "done", reason: "up-to-date" }).text).toBe("app is up to date");
    expect(said({ action: "push", state: "done", count: 1, branch: "main", remote: "origin" }).text).toBe("Pushed 1 commit of main to origin");
    expect(said({ action: "push", state: "done", reason: "new-branch", branch: "x", remote: "origin" }).text).toBe("Pushed x to origin, its upstream from now on");
  });

  it("says why nothing was done, as an error", () => {
    const busy = said({ action: "pull", state: "refused", reason: "busy", agent: "claude", doing: "working" });
    expect(busy.error).toBe(true);
    expect(busy.text).toContain("Claude Code is at work in app");
    expect(said({ action: "pull", state: "refused", reason: "busy", agent: "codex", doing: "asking" }).text).toContain("Codex is asking you something");
    expect(said({ action: "pull", state: "refused", reason: "diverged", ahead: 1, behind: 2 }).text).toContain("1 commit of its own, 2 commits not pulled");
    expect(said({ action: "fetch", state: "refused", reason: "away", output: "GOG is away: no route" }).text).toBe("GOG is away: no route");
    expect(said({ action: "push", state: "failed", output: "! [rejected] main -> main (fetch first)" }).text).toBe("The push of app failed: ! [rejected] main -> main (fetch first)");
  });

  it("words a held push and its confirm", () => {
    const found: Outcome = { action: "push", state: "held", reason: "secrets", scan: { findings: [{ file: "a.go", line: 2, rule: "aws-access-key", excerpt: "AKIA••••" }], files: 1, at: "" } };
    expect(heldWords(found, "app")).toContain("found 1 thing");
    expect(pushAnywayAsk(found, "app")).toContain("with 1 finding?");
    const failed: Outcome = { action: "push", state: "held", reason: "check-failed", output: "it took longer than 45s" };
    expect(heldWords(failed, "app")).toContain("it took longer than 45s");
    expect(pushAnywayAsk(failed, "app")).toContain("no secret check");
  });
});

describe("Fetch all", () => {
  it("counts as it goes and names each failure", () => {
    expect(runWords({ done: 3, total: 12, failed: [] })).toBe("Fetching 3/12…");
    expect(ranWords({ done: 4, total: 4, failed: [] })).toEqual({ text: "Fetched 4 repositories", error: false });
    const ran = ranWords({ done: 4, total: 4, failed: [{ name: "api", why: "fatal: could not read Username for 'https://github.com'" }] });
    expect(ran.error).toBe(true);
    expect(ran.text).toBe("Fetched 3 of 4 repositories. api: fatal: could not read Username for 'https://github.com'");
  });

  it("runs at most so many at once, every one of them", async () => {
    let running = 0;
    let most = 0;
    const done: number[] = [];
    await pooled(
      Array.from({ length: 20 }, (_, i) => i),
      FETCH_AT_ONCE,
      async (i) => {
        running++;
        most = Math.max(most, running);
        await new Promise((resolve) => setTimeout(resolve, 1));
        done.push(i);
        running--;
      },
    );
    expect(most).toBe(FETCH_AT_ONCE);
    expect(done.sort((a, b) => a - b)).toEqual(Array.from({ length: 20 }, (_, i) => i));
    await pooled([], FETCH_AT_ONCE, async () => {});
  });
});
