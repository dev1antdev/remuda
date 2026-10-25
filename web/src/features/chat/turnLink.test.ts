import { describe, expect, it } from "vitest";
import { readTurnLink, turnLink } from "./turnLink";

describe("turnLink", () => {
  it("names the pane, the chat lens, the session and the turn", () => {
    expect(turnLink("http://127.0.0.1:7317", { machineId: "local", paneId: "w12:p1", session: "492634a3-456d-44b4-86e5-8515ba11581d", turn: "abc-1" })).toBe(
      "http://127.0.0.1:7317/?machine=local&pane=w12%3Ap1&lens=chat&session=492634a3-456d-44b4-86e5-8515ba11581d&turn=abc-1",
    );
  });
  it("leaves the session out when there is none", () => {
    expect(turnLink("https://mac.tail.ts.net", { machineId: "gog", paneId: "w3:p2", session: "", turn: "u-9" })).toBe("https://mac.tail.ts.net/?machine=gog&pane=w3%3Ap2&lens=chat&turn=u-9");
  });
});

describe("readTurnLink", () => {
  it("reads back what turnLink wrote", () => {
    const target = { machineId: "gog", paneId: "w3:p2", session: "s-1", turn: "u-9" };
    const url = new URL(turnLink("http://x", target));
    expect(readTurnLink(url.search)).toEqual(target);
  });
  it("defaults the machine to local and the session to none", () => {
    expect(readTurnLink("?pane=w1%3Ap1&turn=t")).toEqual({ machineId: "local", paneId: "w1:p1", session: "", turn: "t" });
  });
  it("is nothing without a pane or a turn", () => {
    expect(readTurnLink("?pane=w1%3Ap1")).toBeNull();
    expect(readTurnLink("?turn=t")).toBeNull();
    expect(readTurnLink("")).toBeNull();
  });
});
