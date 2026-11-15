import { describe, expect, it } from "vitest";
import { chatQuery } from "./api";

describe("a chat read", () => {
  const read = { machineId: "gog", paneId: "w1:p1", sessionId: null, hidden: false };

  it("is the person's look while the page is seen", () => {
    expect(chatQuery(read).toString()).toBe("machine=gog&pane=w1%3Ap1");
  });

  it("says it is none from a page whose document is hidden, the Mac app's closed window among them", () => {
    expect(chatQuery({ ...read, hidden: true }).get("look")).toBe("0");
  });

  it("keeps the rest of the query as it was", () => {
    const q = chatQuery({ ...read, sessionId: "abc", last: 12, subagent: "agent-1", hidden: true });
    expect([q.get("session"), q.get("last"), q.get("subagent"), q.get("look")]).toEqual(["abc", "12", "agent-1", "0"]);
    expect(chatQuery({ machineId: "gog", paneId: "", sessionId: "abc", dir: "/work/api", hidden: true }).toString()).toBe("machine=gog&dir=%2Fwork%2Fapi&session=abc");
  });
});
