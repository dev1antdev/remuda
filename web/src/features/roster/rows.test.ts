import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeAll, describe, expect, it, vi } from "vitest";
import type { Machine, Pane, Session, Tab } from "./api";
import { DEFAULT_RADAR, type RadarPrefs } from "./radar";
import type { Sharing } from "./useSharing";

// the prefs read as their fallbacks: a server render has no browser storage to follow
vi.mock("@/lib/prefs", async (original) => ({ ...(await original<typeof import("@/lib/prefs")>()), usePref: <T,>(_key: string, fallback: T) => [fallback, () => {}] }));

let Rows: typeof import("./rows").Rows;
beforeAll(async () => {
  vi.stubGlobal("location", { search: "", pathname: "/", hash: "" }); // the page's store reads the address when it loads
  ({ Rows } = await import("./rows"));
});

// sharing is the screen-sharing mode hiding the folders named, every other one shown
const sharing = (hidden: string[] = []): Sharing => ({
  pref: { on: hidden.length > 0, all: false, folders: [], machines: [] },
  on: hidden.length > 0,
  machineHidden: () => false,
  folderHidden: (_m, folder) => hidden.includes(folder),
  sessionHidden: (_m, s) => hidden.includes(s.folder || s.dir || ""),
  machineLabel: (m) => m.label,
  folderName: (_m, folder, real) => (hidden.includes(folder) ? "Project A" : real),
  redact: (text) => text,
});

const pane = (over: Partial<Pane>): Pane => ({ id: "w1:p1", title: "", agent: "claude", cwd: "/w/app", focused: true, status: "idle", cols: 80, rows: 24, ...over }) as Pane;
const tab = (panes: Pane[]): Tab => ({ id: "t1", number: 1, label: "1", named: false, status: "idle", panes }) as unknown as Tab;
const session = (panes: Pane[], over: Partial<Session> = {}): Session => ({ id: "w1", name: "app", status: "idle", active_tab_id: "t1", dir: "/w/app", folder: "/w/app", branch: "main", tabs: [tab(panes)], ...over }) as Session;
const machine = (sessions: Session[]): Machine => ({ id: "local", label: "Mac", state: "connected", sessions }) as unknown as Machine;

const draw = (m: Machine, share = sharing(), prefs: RadarPrefs = DEFAULT_RADAR) =>
  renderToStaticMarkup(createElement(Rows, { machine: m, current: null, prefs, step: 0, now: Date.now(), onSelect: () => {}, folded: {}, onFold: () => {}, share, pins: [], subFolded: {}, onFoldSubs: () => {} }));

const secondLines = (html: string) => html.split('class="ws-sub"').length - 1;

describe("a workspace card", () => {
  it("keeps its second line whether its agent works or rests, so the cards under it never move", () => {
    // the main checkout, named as its folder, goes by its branch: no branch under it, and a working agent says nothing
    const working = draw(machine([session([pane({ radar: { state: "working" } })])]));
    const done = draw(machine([session([pane({ radar: { state: "done" }, said: "All green." })])]));
    expect(secondLines(working)).toBe(1);
    expect(secondLines(done)).toBe(1);
  });

  it("shows the Jira issue its work names while its agent works", () => {
    const issue = { key: "PROJ-12", summary: "Cache", status: "In Progress", category: "indeterminate", url: "https://example.atlassian.net/browse/PROJ-12" };
    const html = draw(machine([session([pane({ radar: { state: "working" } })], { branch: "PROJ-12-cache", jira: [issue] as Session["jira"] })]));
    expect(html).toContain("jira-chip");
    expect(html).toContain("PROJ-12");
  });
});

describe("the order by ticket prefix", () => {
  const prefixes: RadarPrefs = { ...DEFAULT_RADAR, order: "prefix" };
  const hiddenOne = session([pane({ id: "w1:p1", radar: { state: "idle" } })], { id: "w1", name: "ACME-12 login", dir: "/w/acme", folder: "/w/acme", branch: "feature/ACME-12-login" });
  const shownOne = session([pane({ id: "w2:p1", radar: { state: "idle" } })], { id: "w2", name: "PR-7 docs", dir: "/w/docs", folder: "/w/docs", branch: "PR-7-docs" });

  it("gathers a workspace the screen-sharing mode hides under no prefix, so no heading reads its name", () => {
    const html = draw(machine([hiddenOne, shownOne]), sharing(["/w/acme"]), prefixes);
    expect(html).not.toContain("ACME");
    expect(html).toContain("PR-");
    expect(html).toContain("No prefix");
  });

  it("names every prefix while nothing is hidden", () => {
    const html = draw(machine([hiddenOne, shownOne]), sharing(), prefixes);
    expect(html).toContain("ACME-");
    expect(html).not.toContain("No prefix");
  });
});
