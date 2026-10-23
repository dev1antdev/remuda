import { describe, expect, it } from "vitest";
import { stallWords, toastDue } from "./perf";

describe("stallWords", () => {
  it("says how long and where", () => {
    expect(stallWords({ ms: 4213, where: "on the Wall (13 tiles)", at: 0 })).toBe("The page stalled 4.2 s on the Wall (13 tiles)");
    expect(stallWords({ ms: 250, where: "in Agents, terminal lens, local w12:p1", at: 0 })).toBe("The page stalled 0.3 s in Agents, terminal lens, local w12:p1");
    expect(stallWords({ ms: 51_400, where: "", at: 0 })).toBe("The page stalled 51 s");
  });
});

describe("toastDue", () => {
  it("toasts a stall the person felt, a minute apart at most", () => {
    expect(toastDue({ ms: 2000, where: "", at: 100_000 }, 0)).toBe(true);
    expect(toastDue({ ms: 1999, where: "", at: 100_000 }, 0)).toBe(false);
    expect(toastDue({ ms: 5000, where: "", at: 100_000 }, 50_000)).toBe(false);
    expect(toastDue({ ms: 5000, where: "", at: 100_000 }, 40_000)).toBe(true);
  });
});
