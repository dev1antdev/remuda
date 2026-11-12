// The display checks scripts/e2e.sh runs after the smoke: every screen of the page drawn at a desk's width and a
// phone's, in English and in Russian, against a remuda of its own on a throwaway herdr. A few workspaces are made
// through the API — a repository with a linked worktree, a second tab, a plain folder — and the browser's copy of
// the roster is dressed with agents (Fetch interception, nothing changes on the server) so the cards, the tiles, the
// zoo and the plan limits have something to draw. Each screen is opened the way a person opens it (the palette by its
// row's id, a click, a key), screenshotted, and checked: no sideways scroll, every menu, popover, dialog, toast and
// drawer inside the screen, no text squeezed to nothing, the content clear of the rail and the header, its landmarks
// there, no console error. Every failure is collected and named by pass, screen and element; any one fails the run.
import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";

const base = process.env.E2E_URL;
const cdp = process.env.E2E_CDP;
const token = process.env.E2E_TOKEN;
const out = process.env.E2E_OUT;
const tmp = process.env.E2E_TMP;
for (const [name, value] of Object.entries({ E2E_URL: base, E2E_CDP: cdp, E2E_TOKEN: token, E2E_OUT: out, E2E_TMP: tmp })) {
  if (!value) throw new Error(`${name} is not set`);
}

// the desks first: once Chrome has emulated a touch screen, (pointer: coarse) keeps matching after it stops
const ALL_PASSES = [
  { name: "desk-en", width: 1440, height: 900, phone: false, lang: "en" },
  { name: "desk-ru", width: 1440, height: 900, phone: false, lang: "ru" },
  { name: "phone-en", width: 390, height: 844, phone: true, lang: "en" },
  { name: "phone-ru", width: 390, height: 844, phone: true, lang: "ru" },
];
// E2E_DISPLAY_PASSES=desk-en,phone-ru runs only those
const PASSES = process.env.E2E_DISPLAY_PASSES ? ALL_PASSES.filter((p) => process.env.E2E_DISPLAY_PASSES.split(",").includes(p.name)) : ALL_PASSES;
const OVERLAYS = ".menu, .popover, .row-menu, .modal, .toast, .sel-pop, .float-term, .palette, .fv";
const CONTENT = ".pane-area, .wall, .tasks-main, .code-mode, .queue, .float-term";

const say = (line) => console.log(`e2e display: ${line}`);
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const failures = [];
let where = "setup";
const failed = (problem) => failures.push(`${where}: ${problem}`);

async function api(method, path, body) {
  const response = await fetch(base + path, {
    signal: AbortSignal.timeout(90_000), // a request the server never answers fails its screen, never holds the run
    method,
    headers: { Authorization: `Bearer ${token}`, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${(await response.text()).trim()}`);
  return response.headers.get("content-type")?.includes("json") ? response.json() : response.text();
}

async function until(check, seconds) {
  for (let i = 0; i < seconds * 5; i++) {
    const value = await check();
    if (value) return value;
    await sleep(200);
  }
  return null;
}

// ---- the browser ----

const target = await (await fetch(`${cdp}/json/new?about:blank`, { method: "PUT" })).json();
const socket = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  socket.onopen = resolve;
  socket.onerror = reject;
});
let nextId = 0;
const waiting = new Map();
let consoleErrors = [];
socket.onmessage = (message) => {
  const msg = JSON.parse(message.data);
  if (msg.id && waiting.has(msg.id)) {
    waiting.get(msg.id)(msg);
    waiting.delete(msg.id);
  }
  if (msg.method === "Fetch.requestPaused") {
    const url = msg.params.request?.url ?? "";
    if (url.includes("/api/opinion")) void fakeOpinion(msg.params);
    else if (url.includes("/api/scorecard")) void fakeScorecard(msg.params);
    else if (url.includes("/api/review/secrets")) void fakeSecrets(msg.params);
    else if (/\/api\/feed/.test(url)) void dressFeed(msg.params);
    else if (/\/api\/chat\?/.test(url)) void dressChat(msg.params);
    else void dressRoster(msg.params);
  }
  if (msg.method === "Runtime.exceptionThrown") consoleErrors.push(msg.params.exceptionDetails?.exception?.description ?? msg.params.exceptionDetails?.text ?? "exception");
  if (msg.method === "Runtime.consoleAPICalled" && msg.params.type === "error") consoleErrors.push(msg.params.args.map((arg) => arg.value ?? arg.description).join(" "));
};
// send is one DevTools call; one the browser never answers fails the screen it belongs to instead of holding the run
const send = (method, params = {}) =>
  new Promise((resolve, reject) => {
    const id = ++nextId;
    const timer = setTimeout(() => {
      waiting.delete(id);
      reject(new Error(`the browser did not answer ${method} in 30 s`));
    }, 30_000);
    waiting.set(id, (msg) => {
      clearTimeout(timer);
      resolve(msg);
    });
    socket.send(JSON.stringify({ id, method, params }));
  });
const evaluate = async (expression) => {
  const reply = await send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
  if (reply.result?.exceptionDetails) throw new Error(`in the page: ${reply.result.exceptionDetails.text} ${reply.result.exceptionDetails.exception?.description ?? ""}`);
  return reply.result?.result?.value;
};
const present = (selector) =>
  evaluate(`(() => { const el = [...document.querySelectorAll(${JSON.stringify(selector)})].find((e) => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0 && getComputedStyle(e).visibility !== "hidden"; }); return !!el; })()`);
const centre = (selector) =>
  evaluate(`(() => { const el = [...document.querySelectorAll(${JSON.stringify(selector)})].find((e) => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0; }); if (!el) return null; el.scrollIntoView({ block: "nearest", inline: "nearest" }); const r = el.getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; })()`);

let pass = PASSES[0];

// press puts the pointer on a point the way the pass's device does: a mouse at a desk, a finger on a phone
async function press([x, y]) {
  if (pass.phone) {
    await send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
    await send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    return;
  }
  await send("Input.dispatchMouseEvent", { type: "mouseMoved", x, y });
  await send("Input.dispatchMouseEvent", { type: "mousePressed", x, y, button: "left", clickCount: 1 });
  await send("Input.dispatchMouseEvent", { type: "mouseReleased", x, y, button: "left", clickCount: 1 });
}
async function hover(selector) {
  const at = await centre(selector);
  if (at && !pass.phone) await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: at[0], y: at[1] });
  return !!at;
}
async function tap(selector) {
  const at = await centre(selector);
  if (!at) throw new Error(`nothing to press at ${selector}`);
  await press(at);
}
const key = async (name, code, modifiers = 0) => {
  const virtualKey = name === "Escape" ? 27 : name === "Enter" ? 13 : name === "`" ? 192 : name.length === 1 ? name.toUpperCase().charCodeAt(0) : undefined;
  await send("Input.dispatchKeyEvent", { type: "rawKeyDown", key: name, code, modifiers, windowsVirtualKeyCode: virtualKey });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: name, code, modifiers, windowsVirtualKeyCode: virtualKey });
};
const mac = await evaluate(`/mac/i.test(navigator.platform)`);
const CMD = mac ? 4 : 2;

// palette runs one of the palette's rows by its entry id, so a pass in any language finds it
async function palette(id) {
  // a frame with the focus hears the keys itself, and off a Mac Ctrl+K in the terminal is the shell's
  await evaluate(`(() => { const el = document.activeElement; if (el instanceof HTMLIFrameElement || el?.closest?.(".xterm")) el.blur(); })()`);
  await key("k", "KeyK", CMD);
  if (!(await until(() => present(".palette-row"), 4))) throw new Error("the palette did not open");
  const ran = await evaluate(`(() => { const row = document.querySelector('.palette-row[data-entry=${JSON.stringify(id)}]'); if (!row) return false; row.scrollIntoView({ block: "nearest" }); row.click(); return true; })()`);
  if (!ran) {
    await key("Escape", "Escape");
    throw new Error(`the palette has no row ${id}`);
  }
  await sleep(250);
}

// closeAll puts the page back to no overlay: Esc until nothing is open, the sidebar drawer shut
async function closeAll() {
  for (let i = 0; i < 4; i++) {
    const open = await evaluate(`[...document.querySelectorAll(${JSON.stringify(".menu, .row-menu, .modal, .palette")})].some((e) => e.getBoundingClientRect().width > 0)`);
    if (!open) break;
    await key("Escape", "Escape");
    await sleep(150);
  }
  if (await evaluate(`[...document.querySelectorAll(".row-menu")].some((e) => e.getBoundingClientRect().width > 0)`)) await press([Math.round(pass.width / 2), 20]);
  if (await present(".sidebar.open")) {
    await evaluate(`document.querySelector('.scrim')?.click()`);
    await sleep(350); // the drawer slides out over the burger a moment longer
  }
  if (!pass.phone) await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: pass.width - 5, y: pass.height - 5 });
}

// ---- what is checked on every screen ----

const CHECKS = `(() => {
  const W = innerWidth, H = innerHeight, problems = [];
  const shown = (el) => {
    if (!el.isConnected) return false;
    const s = getComputedStyle(el);
    if (s.display === "none" || s.visibility === "hidden" || Number(s.opacity) === 0) return false;
    if (el.closest("[hidden], [inert]")) return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  const name = (el) => {
    const cls = typeof el.className === "string" ? el.className.trim().split(/\\s+/).filter(Boolean).slice(0, 3).join(".") : "";
    const label = el.getAttribute("aria-label") || (el.innerText || "").trim().split("\\n")[0].slice(0, 40);
    return el.tagName.toLowerCase() + (cls ? "." + cls : "") + (label ? " «" + label + "»" : "");
  };
  const page = document.documentElement;
  if (page.scrollWidth > W + 1) problems.push("the page scrolls sideways: " + page.scrollWidth + " px of " + W);
  for (const el of document.querySelectorAll(${JSON.stringify(OVERLAYS)})) {
    if (!shown(el)) continue;
    const r = el.getBoundingClientRect();
    if (r.left < -1 || r.top < -1 || r.right > W + 1 || r.bottom > H + 1)
      problems.push(name(el) + " runs off the screen: " + [r.left, r.top, r.right, r.bottom].map(Math.round).join(",") + " in " + W + "×" + H);
  }
  const rail = document.querySelector(".rail");
  const top = document.querySelector(".topbar");
  const blocks = [rail, top].filter((el) => el && shown(el)).map((el) => [el, el.getBoundingClientRect()]);
  for (const el of document.querySelectorAll(${JSON.stringify(CONTENT)})) {
    if (!shown(el)) continue;
    const r = el.getBoundingClientRect();
    for (const [block, b] of blocks) {
      const w = Math.min(r.right, b.right) - Math.max(r.left, b.left);
      const h = Math.min(r.bottom, b.bottom) - Math.max(r.top, b.top);
      if (w > 2 && h > 2) problems.push(name(el) + " lies over " + name(block) + " by " + Math.round(w) + "×" + Math.round(h));
    }
  }
  for (const el of document.querySelectorAll(".rail-label")) {
    if (shown(el) && el.scrollWidth > el.clientWidth + 1) problems.push(name(el.parentElement) + " has its name cut: " + el.scrollWidth + " px in " + el.clientWidth);
  }
  for (const el of document.querySelectorAll(".lens-switch > button, .mode-switch button, .fv-tabs button, .tasks-view button")) {
    if (shown(el) && el.scrollWidth > el.clientWidth + 1) problems.push(name(el) + " spills out of its button: " + el.scrollWidth + " px in " + el.clientWidth);
  }
  // a one-line label whose box clips its text: overflow hidden with a line-height under the font's own height cuts
  // the descenders (g, y, р, у) off at the foot; a text node's range is its glyphs' box, whatever the line-height
  const clipped = [];
  for (const el of document.querySelectorAll("body *")) {
    // an aria-hidden label is still drawn — the rail's names are, the button saying its name to a reader — so it counts
    if (el.closest("svg, .xterm, canvas, .sr-only, [hidden], [inert]")) continue;
    const s = getComputedStyle(el);
    if ((s.overflowY !== "hidden" && s.overflowY !== "clip") || s.whiteSpace !== "nowrap" || s.visibility === "hidden") continue;
    const text = [...el.childNodes].find((node) => node.nodeType === 3 && /[gjpqyрудцщ]/i.test(node.textContent));
    if (!text) continue;
    const box = el.getBoundingClientRect();
    if (box.width < 1 || box.height < 1 || box.bottom < 0 || box.top > H) continue;
    const range = document.createRange();
    range.selectNodeContents(text);
    const glyphs = range.getBoundingClientRect();
    const foot = box.bottom - parseFloat(s.borderBottomWidth) + 0.5; // half a pixel of rounding either way is no clipping
    const head = box.top + parseFloat(s.borderTopWidth) - 0.5;
    if (glyphs.height > 0 && (glyphs.bottom > foot || glyphs.top < head)) clipped.push(name(el) + " (" + Math.round(Math.max(glyphs.bottom - foot, head - glyphs.top) * 10) / 10 + " px)");
  }
  if (clipped.length) problems.push("text clipped by its box: " + clipped.slice(0, 4).join(", ") + (clipped.length > 4 ? " and " + (clipped.length - 4) + " more" : ""));
  const squeezed = [];
  for (const el of document.querySelectorAll("body *")) {
    if (el.closest("svg, .xterm, canvas, [aria-hidden=true], .sr-only")) continue;
    let text = "";
    for (const node of el.childNodes) if (node.nodeType === 3) text += node.textContent;
    if (!text.trim()) continue;
    const s = getComputedStyle(el);
    if (s.display === "none" || s.visibility === "hidden" || el.closest("[hidden]")) continue;
    if (el.offsetParent === null && s.position !== "fixed") continue;
    const r = el.getBoundingClientRect();
    if (r.height > 0 && r.width < 1 && r.right > 0 && r.left < W && r.bottom > 0 && r.top < H) squeezed.push(name(el));
  }
  if (squeezed.length) problems.push("text squeezed to nothing: " + squeezed.slice(0, 4).join(", ") + (squeezed.length > 4 ? " and " + (squeezed.length - 4) + " more" : ""));
  return problems;
})()`;

let shots = 0;
// screen opens one screen, waits for its landmarks, checks it, shoots it and closes it again; a failure is noted, never thrown
async function screen(name, { open, expect = [], close = closeAll, settle = 350 }) {
  where = `${pass.name} › ${name}`;
  consoleErrors = [];
  const began = Date.now();
  try {
    await open();
    for (const selector of expect) if (!(await until(() => present(selector), 6))) failed(`${selector} is not on the screen`);
    await sleep(settle);
    for (const problem of await evaluate(CHECKS)) failed(problem);
    const shot = await send("Page.captureScreenshot", { format: "jpeg", quality: 60 });
    writeFileSync(`${out}/display/${pass.name}-${String(++shots).padStart(3, "0")}-${name}.jpg`, Buffer.from(shot.result.data, "base64"));
  } catch (error) {
    failed(error.message);
  }
  for (const error of consoleErrors) failed(`console error: ${error.slice(0, 300)}`);
  try {
    await close();
  } catch (error) {
    failed(`closing: ${error.message}`);
  }
  const mine = failures.filter((f) => f.startsWith(where + ":"));
  say(`${where} (${((Date.now() - began) / 1000).toFixed(1)} s)${mine.length ? ` — FAILED\n${mine.map((f) => `    ${f.slice(where.length + 2)}`).join("\n")}` : ""}`);
}

// ---- the workspaces and the dressed roster ----

const git = (cwd, ...args) => execFileSync("git", ["-c", "user.email=e2e@remuda.invalid", "-c", "user.name=e2e", ...args], { cwd, stdio: "pipe" });
const appDir = `${tmp}/display/app`;
const notesDir = `${tmp}/display/notes`;
mkdirSync(appDir, { recursive: true });
mkdirSync(notesDir, { recursive: true });
git(appDir, "init", "-q", "-b", "main");
// the repository holds work of every kind for the Changes lens: a file edited in two places far apart, a staged
// edit, a deleted file and a new one
const goLines = (greeting, answer) =>
  [
    "package main",
    "",
    'import "fmt"',
    "",
    "func main() {",
    `\tfmt.Println("${greeting}")`,
    ...Array.from({ length: 40 }, (_, i) => `\t// step ${i + 1} of the long walk`),
    "}",
    "",
    "func answer() int {",
    `\treturn ${answer}`,
    "}",
    "",
  ].join("\n");
mkdirSync(`${appDir}/src`, { recursive: true });
mkdirSync(`${appDir}/docs`, { recursive: true });
writeFileSync(`${appDir}/README.md`, "# app\n");
writeFileSync(`${appDir}/src/app.go`, goLines("hello", 41));
writeFileSync(`${appDir}/docs/notes.md`, "# Notes\n\nThe first note.\n");
writeFileSync(`${appDir}/old.txt`, "an old file\n");
// Claude Code's own approvals committed by mistake, one of them carrying a token: the lens head warns of it. The token
// is made here, its letters in another order, so no secret sits in this file
const shuffledKey = (n) => Array.from({ length: n }, (_, i) => "Zq8vK2mWp4Lr7Tb9Xs3Nd6Hc1JyQe5Gu0"[(i * 7 + 3) % 33]).join("");
mkdirSync(`${appDir}/.claude`, { recursive: true });
writeFileSync(`${appDir}/.claude/settings.local.json`, `${JSON.stringify({ permissions: { allow: ["Bash(npm test)", `Bash(curl -H 'Authorization: Bearer ${shuffledKey(40)}' https://deploy.internal)`] } }, null, 2)}\n`);
git(appDir, "add", "-A");
git(appDir, "add", "-f", ".claude/settings.local.json"); // past an excludes file that ignores it, as Claude Code sets one up
git(appDir, "commit", "-q", "-m", "init");
writeFileSync(`${appDir}/src/app.go`, goLines("hello, world", 42));
writeFileSync(`${appDir}/docs/notes.md`, "# Notes\n\nThe first note.\nA second note, staged.\n");
git(appDir, "add", "docs/notes.md");
git(appDir, "rm", "-q", "old.txt");
writeFileSync(`${appDir}/src/new.ts`, 'export const fresh = "a new file";\n');
const CHANGED = ["docs/notes.md", "old.txt", "src/app.go", "src/new.ts"];
// the "dev server" an agent would start in the repository, for the previews: python3's http.server on a free port
const freePort = () =>
  new Promise((resolve) => {
    const probe = createServer();
    probe.listen(0, "127.0.0.1", () => {
      const { port } = probe.address();
      probe.close(() => resolve(port));
    });
  });
const devPort = await freePort();
const devServer = spawn("python3", ["-m", "http.server", String(devPort), "--bind", "127.0.0.1"], { cwd: appDir, stdio: "ignore" });
mkdirSync(`${out}/display`, { recursive: true });

const made = [];
const now = Date.now();
const iso = (ms) => new Date(ms).toISOString();
const sessions = async () => (await api("GET", "/api/roster")).machines.find((m) => m.id === "local")?.sessions ?? [];
let app, tree, notes, inbox, drawTask, paneA1, paneN;
let DRESS = {};

// seed makes the workspaces the screens draw — the repository with two tabs and a linked worktree, a plain folder —
// a task, a Jira site nobody answers at, and the agents the browser's roster will wear
async function seed() {
  app = await api("POST", "/api/sessions", { machine: "local", dir: appDir, name: "app", agent: "shell" });
  made.push(app.id);
  await api("POST", "/api/tabs", { machine: "local", workspace: app.id, name: "", agent: "shell" });
  tree = await api("POST", "/api/worktrees", { machine: "local", workspace: app.id, branch: "feature-x", base: "main", name: "feature-x", agent: "shell" });
  made.push(tree.id);
  notes = await api("POST", "/api/sessions", { machine: "local", dir: notesDir, name: "notes", agent: "shell" });
  made.push(notes.id);
  await api("PUT", "/api/jira", { site: "http://127.0.0.1:9", account: "", token: "e2e" }).catch((error) => failed(`a Jira site: ${error.message}`));
  // a backup of this instance's own state dir onto this machine, so the Settings section has a run and a copy to show
  await api("PUT", "/api/backups", { machine: "local", at: "03:30", keep: 3 });
  const taken = await api("POST", "/api/backups/run");
  if (!taken.path?.startsWith(`${tmp}/`) || !existsSync(taken.path) || !(taken.size > 0) || !(taken.files > 0)) failed(`a backup now answered ${JSON.stringify(taken)}`);
  const projects = await api("GET", "/api/tasks/projects");
  inbox = (projects.find?.((p) => /inbox/i.test(p.title)) ?? projects[0])?.id;
  drawTask = (await api("POST", "/api/tasks", { project_id: inbox, title: "Draw every screen", markdown: "Every **screen** at two widths.\n\n- [ ] the desk\n- [ ] the phone" })).id;
  // the Inbox's agents set up: a Queued column, kept empty so the queue (on) starts nothing, and a two-stage pipeline
  const inboxProject = projects.find((p) => p.id === inbox);
  const columns = inboxProject.statuses.flatMap((s) => (s.on_new ? [s, { id: 0, project_id: inbox, title: "Queued", hex_color: "f1c40f", kind: "open", position: 0, on_queue: true }] : [s]));
  const withQueue = await api("PUT", `/api/tasks/projects/${inbox}/statuses`, { statuses: columns, moves: {} });
  const testing = withQueue.statuses.find((s) => s.on_finish);
  await api("PUT", `/api/tasks/projects/${inbox}/automation`, {
    automation: {
      machine: "local",
      dir: appDir,
      agent: "claude",
      model: "claude-opus-4-8", // fast mode on Opus 4.8 goes on 30 Nov 2026: the settings say so, and so does the machine's ⓘ
      args: `--settings '{"fastMode":true}'`,
      queue: true,
      max: 2,
      stages: [
        { name: "Tests", agent: "codex", prompt: "Run the tests on {branch} and fix what fails.", status_id: testing?.id ?? 0 },
        { name: "Pull request", agent: "claude", prompt: "", pr: true },
      ],
    },
  }).catch((error) => failed(`the Inbox's agents: ${error.message}`));
  const ready = await until(async () => {
    const list = await sessions();
    const a = list.find((s) => s.id === app.id);
    const b = list.find((s) => s.id === tree.id);
    const n = list.find((s) => s.id === notes.id);
    return a?.tabs.length >= 2 && b && n?.tabs[0]?.panes[0] ? { a, b, n } : null;
  }, 20);
  if (!ready) throw new Error("the seeded workspaces did not reach the roster");
  paneA1 = ready.a.tabs[0].panes[0].id;
  paneN = ready.n.tabs[0].panes[0].id;
  const paneA2 = ready.a.tabs[1].panes[0].id;
  const paneB = ready.b.tabs[0].panes[0].id;
  DRESS = {
    [paneA1]: {
      agent: "claude",
      status: "run",
      title: "Draw every screen of the page at two widths",
      radar: { state: "working", since: now - 3 * 60e3, last_active: now },
      usage: { model: "claude-opus-4-5", name: "Opus 4.5", context: 124000, window: 200000, cost: 4.2, turns: 12, tokens: { input: 1200, output: 3400, cache_read: 98000, cache_write: 2100 } },
      pr: { url: "https://github.com/example/app/pull/15", label: "PR #15", status: { state: "OPEN", draft: false, mergeable: "MERGEABLE", review: "REVIEW_REQUIRED", checks: { total: 3, pass: 2, fail: 1, pending: 0, skipped: 0 }, at: iso(now) } },
    },
    [paneA2]: { agent: "codex", status: "input", title: "Check the build", radar: { state: "blocked", since: now - 60e3, last_active: now - 60e3 } },
    [paneB]: {
      agent: "claude",
      status: "done",
      title: "Rename the flag everywhere it is read",
      radar: { state: "done", last_active: now - 10 * 60e3 },
      said: "The flag is renamed in all twelve places, the tests pass and the release notes say so.",
    },
  };
  say(`seeded: app ${app.id} (two tabs), its worktree ${tree.id}, notes ${notes.id}, the Inbox ${inbox}`);
}
// SERVER_HOST is the host of a remuda serving the page only, as rp-orange's vitals read it: a Raspberry Pi 4, warm, its
// firmware's word unreadable without root, a day of samples for the sparklines
const SERVER_HOST = {
  id: "local",
  label: "This machine · rp-orange",
  health: {
    at: iso(now - 60e3),
    os: "Linux 6.17.0-1018-raspi",
    arch: "arm64",
    model: "Raspberry Pi 4 Model B Rev 1.5",
    cores: 4,
    load1: 0.42,
    uptime: "28 days, 9:03",
    disk_free_gb: 107,
    disk_used_pct: 4,
    mem_total_mb: 7798,
    mem_avail_mb: 6657,
    temp_c: 77.4,
    temp_crit_c: 110,
    freq_mhz: 1300,
    freq_max_mhz: 1800,
    throttle: { now: [] },
    history: Array.from({ length: 24 }, (_, i) => ({ at: iso(now - (24 - i) * 15 * 60e3), load: 0.3 + (i % 5) / 10, disk_free: 96, mem: 14 + (i % 3), temp: 60 + (i % 8) * 2 })),
  },
};

// dressRoster paints the agents on the e2e's own shells in the browser's copy of the roster; the server keeps its own
// fakeOpinion answers the page's /api/opinion calls for the repository with a second opinion of its own, so the lens has
// comments to draw with no agent asked: on src/app.go's two changed lines, on a line its diff folds away, on a file the
// lens does not list; a dismissal is kept until the next pass, any other folder has none, and asking is refused
let fakeOpinionState = null;
const freshOpinion = () => ({
  machine: "local",
  dir: appDir,
  agent: "codex",
  tab: "w1:t9",
  pane: "w1:p9",
  asked: iso(now),
  answered: iso(now),
  state: "done",
  summary: "Two real problems in src/app.go.",
  close: true,
  comments: [
    { id: 1, path: "src/app.go", line: 6, severity: "high", text: "the greeting is printed twice when -quiet is set" },
    { id: 2, path: "src/app.go", line: 50, severity: "low", text: "answer() changed without a test" },
    { id: 3, path: "src/app.go", line: 30, text: "this step comment is stale" },
    { id: 4, path: "README.md", line: 1, text: "the README says nothing of the change" },
  ],
});
async function fakeOpinion({ requestId, request }) {
  const url = new URL(request.url);
  const sent = request.postData ? JSON.parse(request.postData) : {};
  const ours = (url.searchParams.get("dir") ?? sent.dir ?? "").replace(/\/+$/, "").endsWith("/display/app");
  let code = 200;
  let body = { opinion: null };
  if (request.method === "POST" && !url.pathname.endsWith("/dismiss")) [code, body] = [400, { error: "the e2e asks no agent" }];
  else if (ours) {
    fakeOpinionState ??= freshOpinion();
    if (request.method === "POST") {
      const ids = sent.ids ?? [];
      fakeOpinionState = { ...fakeOpinionState, comments: fakeOpinionState.comments.map((c) => (ids.length === 0 || ids.includes(c.id) ? { ...c, dismissed: true } : c)) };
    }
    body = request.method === "DELETE" ? { ok: true } : { opinion: fakeOpinionState };
    if (request.method === "DELETE") fakeOpinionState = null;
  }
  await send("Fetch.fulfillRequest", { requestId, responseCode: code, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// fakeScorecard answers the Models tab with four models: two that compare (one best of a column each, a tie), one on too
// few runs, one whose model was never read; the first two offer the seeded task to try them on, in the app repository
async function fakeScorecard({ requestId }) {
  const share = (count, of) => ({ count, of, ...(of >= 5 ? { rate: count / of } : {}) });
  const mean = (value, n) => ({ ...(value === undefined ? {} : { value }), n, ...(n < 5 ? { few: true } : {}) });
  const tryOn = (lanes) => ({ task_id: drawTask, identifier: `#${drawTask}`, title: "Draw every screen", project_id: inbox, machine: "local", dir: `${tmp}/.herdr/worktrees/app/task-${drawTask}`, ran: iso(now - 86400e3), lanes });
  const opus = { agent: "claude", model: "Opus 4.5", model_id: "claude-opus-4-5" };
  const fable = { agent: "claude", model: "Fable 5.1", model_id: "claude-fable-5-1" };
  const body = {
    since: iso(now - 30 * 86400e3),
    days: 30,
    min_sample: 5,
    verdicts: 12,
    unplaced: 2,
    rows: [
      { ...opus, runs: 7, finished: share(6, 7), first_approved: share(4, 5), rejected: 1, approved: 5, merged: share(2, 3), kept: share(1, 2), cost_per_run: mean(2.4, 7), cost_per_approved: mean(3.1, 5), minutes_per_run: mean(18.2, 6), last: iso(now - 3600e3), try: tryOn([fable, opus]) },
      { ...fable, runs: 5, finished: share(5, 5), first_approved: share(4, 5), rejected: 1, approved: 4, merged: share(3, 5), kept: share(2, 2), cost_per_run: mean(3.9, 5), cost_per_approved: mean(4.6, 4), minutes_per_run: mean(11, 5), last: iso(now - 600e3), try: tryOn([opus, fable]) },
      { agent: "codex", model: "gpt-6.1-sol", model_id: "gpt-6.1-sol", runs: 2, finished: share(2, 2), first_approved: share(1, 2), rejected: 1, approved: 1, merged: share(0, 0), kept: share(1, 2), cost_per_run: mean(undefined, 0), cost_per_approved: mean(undefined, 1), minutes_per_run: mean(7, 2), last: iso(now - 7200e3) },
      { agent: "claude", model: "", runs: 1, finished: share(0, 1), first_approved: share(0, 0), rejected: 0, approved: 0, merged: share(0, 0), kept: share(0, 0), cost_per_run: mean(0.4, 1), cost_per_approved: mean(undefined, 0), minutes_per_run: mean(undefined, 0), last: iso(now - 86400e3) },
    ],
  };
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// fakeSecrets answers the secret check for the repository with what it would find in work holding secrets: a token on
// src/app.go's changed greeting, which the lens can open at its row, and a keystore it cannot; other folders go through
async function fakeSecrets({ requestId, request }) {
  if (!(new URL(request.url).searchParams.get("dir") ?? "").replace(/\/+$/, "").endsWith("/display/app")) {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const body = {
    findings: [
      { file: "src/app.go", line: 6, rule: "github-token", excerpt: 'fmt.Println("ghp_••••••••a1B2")' },
      { file: "certs/release.p12", line: 0, rule: "key-file", excerpt: "release.p12" },
    ],
    skipped: [{ path: "assets/logo.png", why: "binary" }],
    files: 4,
    at: iso(now),
  };
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

async function dressRoster({ requestId, responseStatusCode, responseHeaders = [] }) {
  const reply = await send("Fetch.getResponseBody", { requestId });
  if (responseStatusCode !== 200 || !reply.result) {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const body = reply.result.base64Encoded ? Buffer.from(reply.result.body, "base64").toString("utf8") : reply.result.body;
  let roster;
  try {
    roster = JSON.parse(body);
  } catch {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  for (const machine of roster.machines ?? []) {
    if (machine.id !== "local") continue;
    // Claude Code's sign-in there ends in a day and a half: a red concern with a shell to sign in again from
    machine.health = { ...(machine.health ?? { at: iso(now), disk_free_gb: 120, disk_used_pct: 30, load1: 0.4 }), claude_login: { expires: iso(now + 36 * 3600e3), at: iso(now) } };
    machine.limits = {
      wrapped: true,
      windows: [
        { agent: "claude", name: "5h", used: 62, resets_at: iso(now + 2 * 3600e3), at: iso(now) },
        { agent: "claude", name: "week", used: 86, resets_at: iso(now + 3 * 86400e3), at: iso(now) },
        { agent: "codex", name: "week", used: 31, resets_at: iso(now + 5 * 86400e3), at: iso(now) },
      ],
    };
    for (const session of machine.sessions ?? []) {
      if (session.id === app.id || session.id === tree.id) session.icon = { emoji: "🦊" }; // the icon is the folder's, the worktree's too
      if (session.id === app.id) {
        session.jira = [{ key: "APP-12", summary: "Draw every screen", status: "In Review", category: "indeterminate", url: "https://example.atlassian.net/browse/APP-12" }];
      }
      for (const tab of session.tabs ?? []) for (const pane of tab.panes ?? []) Object.assign(pane, DRESS[pane.id] ?? {});
    }
  }
  roster.feed = FEED.id; // the activity's newest id as this run sets it: a page that saw an older one reads the feed
  roster.server = SERVER_HOST; // the Server place a remuda -server draws: its host's vitals, the readings a Raspberry Pi gives
  const headers = responseHeaders.filter((h) => !/^(etag|content-length|content-encoding)$/i.test(h.name));
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: headers, body: Buffer.from(JSON.stringify(roster)).toString("base64") });
}

// DENIED is what the dressed Claude Code pane's conversation was denied without asking, for the chat lens's notice
const DENIED = [
  { tool: "Bash", what: "npm install --save-dev vitest", at: iso(now - 20 * 60e3), tool_id: "toolu_e2e_1" },
  { tool: "Write", what: `${tmp}/display/app/src/new-screen.tsx`, at: iso(now - 18 * 60e3), tool_id: "toolu_e2e_2" },
];

// dressChat adds DENIED to the lens's answer for the dressed agent pane (a tile's tail, last=N, carries none)
async function dressChat({ requestId, request, responseStatusCode, responseHeaders = [] }) {
  const url = new URL(request.url);
  const reply = responseStatusCode === 200 && url.searchParams.get("pane") === paneA1 && !url.searchParams.has("last") ? await send("Fetch.getResponseBody", { requestId }) : null;
  if (!reply?.result) {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const chat = JSON.parse(reply.result.base64Encoded ? Buffer.from(reply.result.body, "base64").toString("utf8") : reply.result.body);
  chat.denied = DENIED;
  const headers = responseHeaders.filter((h) => !/^(etag|content-length|content-encoding)$/i.test(h.name));
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: headers, body: Buffer.from(JSON.stringify(chat)).toString("base64") });
}

// FEED is the activity the browser is shown: an id far past the server's own, and the alert a screen puts in it
const FEED = { id: 900000, entries: [] };

// dressFeed answers the page's read of the activity with FEED's entries, so an alert can be made to arrive
async function dressFeed({ requestId, responseHeaders = [] }) {
  if (FEED.entries.length === 0) {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const headers = responseHeaders.filter((h) => !/^(etag|content-length|content-encoding)$/i.test(h.name));
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: headers, body: Buffer.from(JSON.stringify({ entries: FEED.entries })).toString("base64") });
}

// ---- the screens ----

// alertScreens: an alert the activity records shows on the page while it is in front — the feed's newest id moved, a
// roster event (a mute set and lifted on the server) makes the page read it — and the alerts panel says what this
// browser is, and its test puts a toast up on the page itself
async function alertScreens() {
  const toastWith = (words) => `[...document.querySelectorAll('.toast')].find((t) => t.textContent.includes(${JSON.stringify(words)}))`;
  await screen("alert-toast", {
    open: async () => {
      FEED.id += 1;
      const asking = Object.keys(DRESS).find((id) => DRESS[id].status === "input"); // a question's toast stands only while its pane asks
      FEED.entries = [{ id: FEED.id, at: iso(Date.now()), kind: "input", machine: "local", pane: asking, title: "app needs you", body: "codex is waiting for an answer" }];
      await api("PUT", "/api/mutes", { machine: "local", dir: appDir, until: iso(Date.now() + 3600e3) });
      await api("DELETE", "/api/mutes", { machine: "local", dir: appDir });
      if (!(await until(() => evaluate(`!!${toastWith("app needs you")}`), 20))) failed(`an alert the activity recorded did not show on the page (the page: ${await evaluate(`document.visibilityState + ", focus " + document.hasFocus()`)})`);
      check(await evaluate(`!!${toastWith("app needs you")}?.querySelector('.btn.primary')`), "the alert on the page has no Open");
    },
    expect: [".toast"],
    close: async () => {
      FEED.entries = [];
      await evaluate(`${toastWith("app needs you")}?.querySelector('.icon-btn')?.click()`);
      await sleep(150);
    },
  });
  await screen("alerts-device", {
    open: async () => palette("alerts"),
    expect: [".alerts-device .alerts-device-lines li"],
    close: async () => {},
  });
  await screen("alerts-test", {
    open: async () => {
      await tap(".alerts-test");
      if (!(await until(() => evaluate(`!!${toastWith("remuda ·")}`), 4))) failed("the panel's test put no toast up on the page");
    },
    expect: [".toast"],
    close: async () => {
      await evaluate(`${toastWith("remuda ·")}?.querySelector('.icon-btn')?.click()`);
      await closeAll();
    },
  });
}

const sidebarOpen = async () => {
  if (pass.phone && !(await present(".sidebar.open"))) {
    await tap(".burger");
    if (!(await until(() => present(".sidebar.open"), 1.5))) await tap(".burger");
    if (!(await until(() => present(".sidebar.open"), 3))) throw new Error("the sidebar drawer did not open");
    await sleep(300);
  }
};
// tapUntil presses a button and, when what it opens is not there yet, once more: a phone's drawer still sliding in
// under load can take the first tap on its way
const tapUntil = async (selector, opened) => {
  await tap(selector);
  if (await until(() => present(opened), 2)) return;
  await tap(selector);
  await until(() => present(opened), 3);
};
const folderHead = (name) => `[...document.querySelectorAll('.ws-folder-head')].find((h) => h.querySelector('.ws-folder-name')?.textContent === ${JSON.stringify(name)})`;
const firstCardOf = (folder) => `${folderHead(folder)}?.parentElement.querySelector('.ws-item')`;
const mark = async (expression, attr) => {
  const found = await evaluate(`(() => { document.querySelectorAll('[data-e2e]').forEach((e) => e.removeAttribute('data-e2e')); const el = ${expression}; if (!el) return false; el.setAttribute('data-e2e', ${JSON.stringify(attr)}); return true; })()`);
  if (!found) throw new Error(`no ${attr} on the screen`);
  return `[data-e2e="${attr}"]`;
};
// onAgents comes back to the Agents mode on the seeded agent's pane
const onAgents = async () => {
  if (await present(".tabstrip")) return;
  await palette("mode:herdr");
  if (!(await until(() => present(".tabstrip"), 6))) throw new Error("the Agents mode did not come back");
};

// ---- the Changes lens on the repository's real work ----

const check = (ok, problem) => ok || failed(problem);
const count = (selector) => evaluate(`document.querySelectorAll(${JSON.stringify(selector)}).length`);
const LENS = ".lens-pane:not([hidden]) .review";
const fileHead = (path) => `[...document.querySelectorAll('${LENS} .review-file')].find((f) => f.querySelector('.review-path')?.textContent === ${JSON.stringify(path)})?.querySelector('.review-file-head')`;
const shownPaths = () => evaluate(`[...document.querySelectorAll('${LENS} .review-file .review-path')].map((p) => p.textContent).sort()`);
const diffRows = () => count(`${LENS} .review-diff .fv-line:not(.hunk)`);

// changesScreens draws the lens on every pass: the files, a diff with its words marked and coloured, the other
// column layout, an unfold, a comment in the draft, the verdict dialog — nothing sent, nothing marked
async function changesScreens() {
  await onAgents();
  await screen("changes-lens", {
    open: async () => {
      await palette("lens:changes");
      if (!(await until(async () => (await count(`${LENS} .review-file`)) === CHANGED.length, 8))) failed(`the lens lists ${await count(`${LENS} .review-file`)} files, not ${CHANGED.length}`);
      const paths = await shownPaths();
      check(JSON.stringify(paths) === JSON.stringify(CHANGED), `the lens lists ${paths.join(", ")}`);
      check((await count(`${LENS} .review-file.fresh`)) === CHANGED.length, "a file never looked at is not marked fresh");
      check(await evaluate(`(() => { const sub = document.querySelector('${LENS} .review-sub'); return !!sub && sub.scrollWidth <= sub.clientWidth + 1 && /4/.test(sub.textContent); })()`), "the line under the branch does not say the 4 files whole");
    },
    expect: [`${LENS} .review-head`],
    close: async () => {},
  });
  await screen("changes-diff", {
    open: async () => {
      await tap(await mark(fileHead("src/app.go"), "app-go"));
      if (!(await until(async () => (await count(`${LENS} .review-hunk`)) >= 2, 6))) failed(`src/app.go shows ${await count(`${LENS} .review-hunk`)} hunks, not 2`);
      check((await count(`${LENS} .review-diff .fv-mark`)) > 0, "no changed words are marked");
      check((await count(`${LENS} .review-diff [class*="hl-"]`)) > 0, "the Go code is not coloured");
      check((await count(`${LENS} .review-diff.columns`)) === (pass.phone ? 0 : 1), pass.phone ? "a phone opens the diff in two columns" : "a desk opens the diff in one column");
    },
    expect: [`${LENS} .review-hunk`],
    close: async () => {},
  });
  await screen("changes-columns", {
    open: async () => {
      await tap(`${LENS} .review-head .icon-btn[aria-pressed]`);
      if (!(await until(async () => (await count(`${LENS} .review-diff.columns`)) === (pass.phone ? 1 : 0), 3))) failed("the columns switch did not switch");
    },
    expect: [`${LENS} .review-hunk`],
    close: async () => {
      await tap(`${LENS} .review-head .icon-btn[aria-pressed]`);
      await sleep(200);
    },
  });
  await screen("changes-unfold", {
    open: async () => {
      const before = await diffRows();
      await tap(await mark(`document.querySelectorAll('${LENS} .review-hunk')[1]?.querySelector('.hunk-unfold button')`, "unfold"));
      if (!(await until(async () => (await diffRows()) > before, 5))) failed(`no line unfolded above the second hunk: ${before} rows still`);
    },
    expect: [`${LENS} .review-hunk`],
    close: async () => {},
  });
  await screen("changes-comment", {
    open: async () => {
      await tap(await mark(`document.querySelector('${LENS} .review-diff .fv-line.commentable.add')`, "added-line"));
      if (!(await until(() => present(`${LENS} .review-comment textarea`), 3))) throw new Error("no comment box under the line");
      await send("Input.insertText", { text: "Say hello to the world in one place only." });
      await tap(`${LENS} .review-comment button[type=submit]`);
      if (!(await until(() => present(`${LENS} .review-note`), 3))) failed("the comment did not join the draft");
      check((await evaluate(`document.querySelector('${LENS} .review-tag-comments')?.textContent`)) === "1", "the file's row does not count its comment");
      check(await present(`${LENS} .review-draft-bar .review-draft-count svg`), "the draft bar does not say there is a comment");
    },
    expect: [`${LENS} .review-draft-bar`],
    close: async () => {
      await tap(`${LENS} .review-note .icon-btn`);
      await sleep(200);
    },
  });
  await screen("changes-opinion", {
    open: async () => {
      if (!(await until(() => present(`${LENS} .opinion-block`), 6))) throw new Error("no second opinion block");
      check((await count(`${LENS} .opinion-apart .review-opinion`)) === 1, "the comment on a file the lens does not list is not listed apart");
      check((await count(`${LENS} .review-diff .review-opinion`)) === 3, `src/app.go carries ${await count(`${LENS} .review-diff .review-opinion`)} of its 3 comments`);
      check((await count(`${LENS} .review-orphans .review-opinion`)) === 1, "the comment on a line the diff does not show is not listed under it");
      check((await evaluate(`document.querySelector('${LENS} .review-tag-opinion')?.textContent?.trim()`)) === "3", "the file's row does not count the second opinion's comments");
      await tap(await mark(`document.querySelector('${LENS} .review-hunk .review-opinion .btn')`, "adopt"));
      if (!(await until(async () => (await evaluate(`document.querySelector('${LENS} .review-tag-comments')?.textContent`)) === "1", 3))) failed("an adopted comment did not join the draft");
      if (!(await until(async () => (await count(`${LENS} .review-diff .review-opinion`)) === 2, 3))) failed("an adopted comment stayed among the second opinion's");
      check(await present(`${LENS} .review-hunk .review-note`), "the adopted comment does not sit under its row as a draft note");
    },
    expect: [`${LENS} .opinion-block`],
    close: async () => {
      await tap(`${LENS} .review-note .icon-btn`);
      await sleep(200);
    },
  });
  await screen("changes-opinion-dialog", {
    open: async () => tap(`${LENS} .review-ask-opinion`),
    expect: [".modal .sel", ".modal .deny-switch"],
  });
  await screen("changes-verdict", {
    open: async () => tap(`${LENS} .review-verdicts .btn:not(.primary)`),
    expect: [".modal textarea"],
  });
  await screen("changes-secrets", {
    open: async () => {
      check(await present(`${LENS} .review-settings .notice.warn`), "the lens head does not warn of the token in .claude/settings.local.json");
      await tap(`${LENS} .review-verdicts .btn.primary`);
      if (!(await until(() => present(".modal .verdict-ask"), 3))) throw new Error("the approval offers no commit");
      await tap(await mark(`document.querySelectorAll('.modal .verdict-ask')[1]`, "ask-commit"));
      if (!(await until(() => present(".modal .secret-check.is-found"), 6))) throw new Error("the secret check did not report its findings");
      check((await count(".modal .secret-finding")) === 2, `the dialog lists ${await count(".modal .secret-finding")} findings, not 2`);
      check(await evaluate(`document.querySelector('.modal button[type=submit]')?.disabled === true`), "Approve & commit is not held back by the findings");
      check(await present(".modal .review-send-anyway"), "no Send anyway beside the held button");
      check(await evaluate(`document.querySelectorAll('.modal .secret-finding:disabled').length === 1`), "the keystore, which has no row, is offered as a jump");
      const inside = `(() => { const box = document.querySelector('.modal').getBoundingClientRect(); return [...document.querySelectorAll('.modal .modal-actions .btn')].every((b) => { const r = b.getBoundingClientRect(); return r.left >= box.left - 1 && r.right <= box.right + 1; }); })()`;
      check(await evaluate(inside), "Cancel, Send anyway and the held approval run past the dialog's edge");
    },
    expect: [".modal .secret-check"],
    close: async () => {},
  });
  // Send anyway asks first, naming how many: the confirm is only looked at, never answered — nothing is sent
  await screen("changes-send-anyway", {
    open: async () => {
      await tap(".modal .review-send-anyway");
      if (!(await until(() => present(".confirm-text"), 3))) throw new Error("Send anyway went without a confirm");
      check(/\b2\b/.test(await evaluate(`document.querySelector('.confirm-text')?.textContent ?? ""`)), "the confirm does not say how many findings");
    },
    expect: [".confirm-text"],
    close: async () => {
      await tap(await mark(`[...document.querySelectorAll('.modal')].pop()?.querySelector('.modal-actions .btn:not(.danger)')`, "confirm-cancel"));
      if (!(await until(async () => !(await present(".confirm-text")), 3))) failed("Cancel did not close the confirm");
    },
  });
  await screen("changes-secret-row", {
    open: async () => {
      await tap(".modal .secret-finding:not(:disabled)");
      if (!(await until(async () => !(await present(".modal")), 3))) failed("a finding's click did not give the dialog's place to its row");
      if (!(await until(() => present(`${LENS} .review-file.open .fv-line.secret`), 6))) failed("the finding's row is not marked in the diff");
      check(await present(`${LENS} .review-tag-secret`), "the file's row does not count its secret");
    },
    expect: [`${LENS} .fv-line.secret`],
  });
  await screen("changes-file", {
    open: async () => {
      await tap(await mark(`${fileHead("src/app.go")}?.parentElement.querySelector('.icon-btn')`, "open-file"));
      if (!(await until(async () => (await count(".fv .fv-code:not(.fv-diff) .hl-keyword")) > 0, 5))) failed("the file viewer does not colour the Go file");
    },
    expect: [".fv .fv-head"],
    settle: 600,
    close: async () => {},
  });
  await screen("changes-file-diff", {
    open: async () => {
      await tap(".fv .fv-tabs button:last-child");
      if (!(await until(async () => (await count(".fv .fv-diff .fv-line.add .hl-string")) > 0, 5))) failed("the file viewer's Changes tab does not colour the added line");
    },
    expect: [".fv .fv-diff"],
    close: async () => {
      await key("Escape", "Escape");
      if (!(await until(async () => !(await present(".fv")), 3))) failed("Esc did not close the file viewer");
    },
  });
  await palette("lens:terminal");
}

// changesFlow runs once, after the passes: the Review mode lists the repository, Mark reviewed moves the baseline,
// an edit after the look is fresh again, a commit is listed, and a commit of reviewed work reads as reviewed
async function changesFlow() {
  where = `${pass.name} › changes-flow`;
  const lens = async () => {
    await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=changes` });
    if (!(await until(() => present(`${LENS} .review-head`), 15))) throw new Error("the Changes lens did not come up");
  };
  const lookAgain = async () => {
    await tap(await mark(`document.querySelectorAll('${LENS} .review-head .icon-btn')[1]`, "look-again"));
    await sleep(400);
  };
  const markReviewed = async () => {
    await tap(`${LENS} .review-seen`);
    await sleep(400);
  };
  const state = async () => evaluate(`({ files: document.querySelectorAll('${LENS} .review-file').length, fresh: [...document.querySelectorAll('${LENS} .review-file.fresh .review-path')].map((p) => p.textContent), commits: document.querySelectorAll('${LENS} .review-commit').length, seen: document.querySelectorAll('${LENS} .review-commit.seen').length, folded: !!document.querySelector('${LENS} .review-fold') })`);
  const settles = async (want, what) => {
    const ok = await until(async () => {
      const now = await state();
      return Object.entries(want).every(([key, value]) => JSON.stringify(now[key]) === JSON.stringify(value));
    }, 8);
    if (!ok) failed(`${what}: ${JSON.stringify(await state())}, wanted ${JSON.stringify(want)}`);
  };

  await palette("mode:review");
  if (!(await until(() => evaluate(`[...document.querySelectorAll('.queue-item')].some((i) => i.textContent.includes("app"))`), 75))) failed("the Review mode never listed the repository with its changes");
  else {
    await tap(await mark(`[...document.querySelectorAll('.queue-item')].find((i) => i.textContent.includes("app"))`, "queue-app"));
    if (!(await until(async () => (await count(".queue-main .review .review-file")) === CHANGED.length, 8))) failed("the queue's pick does not show the repository's files");
  }
  await lens();
  await settles({ files: CHANGED.length, fresh: CHANGED, commits: 0 }, "before any look");
  await markReviewed();
  await settles({ files: 0, fresh: [], folded: true }, "after Mark reviewed: every file folded as unchanged since the look");
  writeFileSync(`${appDir}/src/app.go`, goLines("hello, world", 43));
  await lookAgain();
  await settles({ fresh: ["src/app.go"], folded: true }, "an edit after the look");
  git(appDir, "add", "-A");
  git(appDir, "commit", "-q", "-m", "the work");
  await lookAgain();
  await settles({ files: 0, commits: 1, seen: 0 }, "the work committed with an edit nobody looked at");
  await markReviewed();
  await settles({ files: 0, commits: 0 }, "the commit reviewed");
  writeFileSync(`${appDir}/docs/notes.md`, "# Notes\n\nThe first note.\nA second note, staged.\nA third, reviewed before its commit.\n");
  await lookAgain();
  await settles({ fresh: ["docs/notes.md"] }, "a new edit");
  await markReviewed();
  git(appDir, "commit", "-q", "-am", "the reviewed note");
  await lookAgain();
  await settles({ files: 0, commits: 1, seen: 1 }, "a commit of work already reviewed");
  say("changes flow: the queue, a look, an edit after it, a commit, a commit of reviewed work");
}

// worktreeFlow runs once, after the Changes flow: a clean worktree's workspace closed takes its folder and branch with
// it, one with an uncommitted file keeps its folder, the leftovers dialog lists that one, and once clean removes it
// previewFlow runs once: the dev server in the repository is listed for its workspace, remuda's own ports are not;
// Open serves it on a port of the cockpit's (another number, since the server holds its own on this host) to the
// signed-in caller alone, a port nothing listens on is refused, and Close shuts it
async function previewFlow() {
  where = `${pass.name} › preview-flow`;
  const listed = await api("GET", "/api/previews?machine=local");
  const mine = listed.listeners.find((l) => l.port === devPort);
  check(mine?.workspace === app.id && mine?.label === "http.server", `the dev server is listed as ${JSON.stringify(mine)}`);
  check(!listed.listeners.some((l) => ["remuda", "herdr"].includes(l.command) || l.port === Number(new URL(base).port)), "remuda's or herdr's own port is offered as a preview");
  const opened = await api("POST", "/api/previews/open", { machine: "local", port: devPort });
  check(opened.local !== devPort && opened.url.endsWith(`:${opened.local}`), `the preview opened as ${JSON.stringify(opened)}`);
  const served = await fetch(`${opened.url}/`, { headers: { Authorization: `Bearer ${token}`, Cookie: "remuda_token=never-forwarded" } });
  check(served.status === 200 && (await served.text()).includes("Directory listing"), `the preview answered ${served.status}`);
  const stranger = await fetch(`${opened.url}/`);
  check(stranger.status === 401 && !(await stranger.text()).includes("Directory listing"), `a caller with no sign-in got ${stranger.status}`);
  const again = await api("POST", "/api/previews/open", { machine: "local", port: devPort });
  check(again.url === opened.url, "a second Open made another preview");
  const nothing = await fetch(`${base}/api/previews/open`, { method: "POST", headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" }, body: JSON.stringify({ machine: "local", port: 22 }) });
  check(nothing.status === 404, `a port nothing of the person's listens on answered ${nothing.status}`);
  const shut = await api("DELETE", "/api/previews", { machine: "local", port: devPort });
  check(shut.closed === true, "Close did not shut the preview");
  check(await fetch(`${opened.url}/`).then(() => false, () => true), "a shut preview still answers");
  say(`preview flow: port ${devPort} listed in its workspace, opened on ${opened.local} for the signed-in caller only, shut`);
}

async function worktreeFlow() {
  where = `${pass.name} › worktree-flow`;
  const worktreeOf = async (id) => (await sessions()).find((s) => s.id === id)?.worktree?.checkout_path ?? "";
  const branches = () => git(appDir, "branch", "--format=%(refname:short)").toString();
  const clean = await worktreeOf(tree.id);
  if (!clean || !existsSync(clean)) throw new Error(`the feature-x worktree is not on disk: ${clean}`);
  const listed = await api("GET", "/api/machines/local/leftovers");
  check(!listed.leftovers.some((left) => left.path === clean), "a worktree whose workspace is open is listed as left over");
  const closed = await api("DELETE", `/api/sessions/${encodeURIComponent(tree.id)}?machine=local`);
  made.splice(made.indexOf(tree.id), 1);
  check(closed.worktree?.removed === true && closed.worktree?.branch_deleted === true, `the clean worktree's close answered ${JSON.stringify(closed.worktree)}`);
  check(!existsSync(clean), "the clean worktree's folder stayed after its workspace closed");
  check(!branches().includes("feature-x"), "the merged branch feature-x stayed");

  const dirty = await api("POST", "/api/worktrees", { machine: "local", workspace: app.id, branch: "dirty-x", base: "main", name: "dirty-x", agent: "shell" });
  made.push(dirty.id);
  let folder = "";
  if (!(await until(async () => (folder = await worktreeOf(dirty.id)), 20))) throw new Error("the dirty-x worktree did not reach the roster");
  writeFileSync(`${folder}/draft.txt`, "not committed\n");
  const kept = await api("DELETE", `/api/sessions/${encodeURIComponent(dirty.id)}?machine=local`);
  made.splice(made.indexOf(dirty.id), 1);
  check(kept.worktree?.removed === false && /uncommitted/.test(kept.worktree?.kept ?? ""), `the dirty worktree's close answered ${JSON.stringify(kept.worktree)}`);
  check(existsSync(`${folder}/draft.txt`), "the uncommitted file went with the close");
  await screen("leftovers", {
    open: async () => {
      await palette("leftovers:local");
      if (!(await until(async () => (await count(".modal .pick-row")) === 1, 15))) failed(`the leftovers dialog lists ${await count(".modal .pick-row")} worktrees, not 1`);
      check((await count(".modal .pick-row input:disabled")) === 1, "a worktree with an uncommitted file can be ticked for removal");
    },
    expect: [".modal .pick-row"],
  });
  rmSync(`${folder}/draft.txt`);
  const listedAgain = await api("GET", "/api/machines/local/leftovers");
  const left = listedAgain.leftovers.find((l) => l.path === folder);
  check(left && !left.dirty && !left.broken, `the cleaned worktree is listed as ${JSON.stringify(left)}`);
  const removed = await api("POST", "/api/machines/local/leftovers/remove", { paths: [folder, "/not/a/leftover"] });
  check(removed.outcomes[0]?.removed === true && removed.outcomes[1]?.removed === false, `the removal answered ${JSON.stringify(removed.outcomes)}`);
  check(!existsSync(folder), "the cleaned leftover's folder stayed");
  say("worktree flow: a clean close takes its worktree, a dirty one keeps it, the leftover goes once clean");
}

async function runPass() {
  await send("Emulation.setDeviceMetricsOverride", { width: pass.width, height: pass.height, deviceScaleFactor: 1, mobile: pass.phone });
  await send("Emulation.setTouchEmulationEnabled", { enabled: pass.phone, maxTouchPoints: pass.phone ? 5 : 0 });
  fakeOpinionState = null;
  const prefs = { lang: pass.lang, "tasks.sel": { kind: "project", id: inbox }, "tasks.view": "board" };
  await evaluate(`(() => { localStorage.clear(); for (const [k, v] of Object.entries(${JSON.stringify(prefs)})) localStorage.setItem("remuda." + k, JSON.stringify(v)); })()`);
  await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=terminal` });
  if (!(await until(() => present(".pane-area .tab"), 15))) failed("the Agents page did not come up");
  const spoken = await evaluate("document.documentElement.lang");
  if (spoken !== pass.lang) failed(`the page speaks ${spoken}, not ${pass.lang}`);
  if ((await evaluate(`matchMedia("(pointer: coarse)").matches`)) !== pass.phone) failed(`the browser does not take itself for a ${pass.phone ? "phone" : "desk"}: (pointer: coarse) says otherwise`);
  where = pass.name;

  await screen("sidebar", { open: sidebarOpen, expect: [".ws-folder-head", ".ws-item", ".machine-head"], close: async () => {} });
  await screen("folder-folded", {
    open: async () => {
      await sidebarOpen();
      await tap(await mark(folderHead("notes"), "folder"));
      if (!(await until(() => evaluate(`${folderHead("notes")}?.getAttribute("aria-expanded") === "false"`), 3))) failed("the folder did not fold");
    },
    expect: [".ws-folder.folded"],
    close: async () => {
      await tap(await mark(folderHead("notes"), "folder"));
    },
  });
  await screen("card-menu", {
    open: async () => {
      await sidebarOpen();
      await hover(await mark(firstCardOf("app") + "?.querySelector('.ws-main')", "card"));
      await sleep(150);
      await tap(await mark(firstCardOf("app") + "?.querySelector('.ws-menu-btn')", "card-menu"));
    },
    expect: ["[role=menu].menu"],
  });
  await screen("folder-menu", {
    open: async () => {
      await sidebarOpen();
      await hover(await mark(folderHead("app"), "folder"));
      await sleep(150);
      await tap(await mark(folderHead("app") + "?.querySelector('.ws-folder-menu')", "folder-menu"));
    },
    expect: ["[role=menu].menu"],
  });
  await screen("folder-new-workspace", {
    open: async () => {
      await sidebarOpen();
      await hover(await mark(folderHead("app"), "folder"));
      await sleep(150);
      await tap(await mark(folderHead("app") + "?.querySelector('.ws-folder-menu')", "folder-menu"));
      if (!(await until(() => present("[role=menu] [role=menuitem]"), 3))) throw new Error("the folder's menu did not open");
      await tap("[role=menu] [role=menuitem]");
      if (!(await until(() => evaluate(`[...document.querySelectorAll(".modal input")].some((i) => i.value.startsWith("worktree/"))`), 4))) failed("a repository folder's new workspace is not a worktree on a branch of its own");
    },
    expect: [".modal"],
  });
  await screen("machine-menu", {
    open: async () => {
      await sidebarOpen();
      await tapUntil('[data-machine-menu="local"]', "[role=menu].menu");
    },
    expect: ["[role=menu].menu"],
  });
  await screen("vitals", {
    open: async () => {
      await sidebarOpen();
      await tapUntil('[data-health-btn="local"]', ".health-pop");
      if (!(await until(() => evaluate(`[...document.querySelectorAll(".health-pop .health-list li")].some((li) => li.textContent.includes("claude auth login"))`), 6))) failed("the vitals say nothing of the sign-in ending");
      if (!(await until(() => evaluate(`[...document.querySelectorAll(".health-pop .health-list li")].some((li) => li.textContent.includes("Opus 4.8"))`), 6))) failed("the vitals say nothing of the project's agents on fast Opus 4.8");
    },
    expect: [".health-pop", ".health-pop .health-list li.bad"],
  });
  await screen("tab-strip", { open: () => onAgents(), expect: [".tabstrip .tab", ".lens-pane:not([hidden]) .xterm-screen"] });
  await screen("shell-tab", {
    open: async () => {
      await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneN)}&lens=chat` });
      if (!(await until(() => present(".lens-pane:not([hidden]) .xterm-screen"), 15))) failed("a ?lens=chat link to a shell pane did not land on its terminal");
      check(!(await present(".lens-pane:not([hidden]) .chat-log, .lens-pane:not([hidden]) .review")), "a shell pane shows a chat or the changes");
      check(await evaluate(`(() => { const nav = document.querySelector(".lens-switch"); return !!nav && nav.classList.contains("is-vacant") && getComputedStyle(nav).visibility === "hidden" && nav.getBoundingClientRect().width > 0; })()`), "a shell pane's lens switch is drawn, or gave up its room");
      if (!(await until(() => evaluate(`new URLSearchParams(location.search).get("lens") === "terminal"`), 3))) failed(`a shell pane's address says lens=${await evaluate(`new URLSearchParams(location.search).get("lens")`)}`);
      await key("k", "KeyK", CMD);
      if (!(await until(() => present(".palette-row"), 4))) throw new Error("the palette did not open");
      check(!(await evaluate(`!!document.querySelector('.palette-row[data-entry="lens:chat"], .palette-row[data-entry="lens:changes"], .palette-row[data-entry="split"]')`)), "the palette offers a shell pane its chat, its changes or the split");
      await key("Escape", "Escape");
    },
    expect: [".tabstrip .tab"],
    close: async () => {
      await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=terminal` });
      if (!(await until(() => present(".pane-area .tab"), 15))) failed("the Agents page did not come back");
    },
  });
  // an agent appearing in a pane: started at a shell's prompt, the terminal stays; an agent's pane a restart passed
  // through a shell goes back to its chat — herdr's agent field played by the browser's dressed roster (desks only)
  if (!pass.phone)
    await screen("agent-in-a-shell", {
      open: async () => {
        const lensShown = () => evaluate(`document.querySelector(".lens-pane:not([hidden]) .chat-log, .lens-pane:not([hidden]) .composer") ? "chat" : document.querySelector(".lens-pane:not([hidden]) .xterm-screen") ? "terminal" : ""`);
        const vacant = () => evaluate(`!!document.querySelector(".lens-switch.is-vacant")`);
        await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneN)}&lens=chat` });
        if (!(await until(async () => (await lensShown()) === "terminal", 15))) throw new Error("the shell pane did not come up on its terminal");
        DRESS[paneN] = { agent: "claude", status: "run", title: "claude, typed at the prompt", radar: { state: "working", since: now, last_active: now } };
        if (!(await until(async () => !(await vacant()), 30))) throw new Error("the page never saw the agent start in the shell");
        await sleep(600);
        check((await lensShown()) === "terminal", "an agent started at a shell's prompt turned the pane to its chat");
        check(await evaluate(`new URLSearchParams(location.search).get("lens") === "terminal"`), "the address of a shell an agent started in does not say its terminal");
        delete DRESS[paneN];
        const agentPane = DRESS[paneA1];
        await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=chat` });
        if (!(await until(async () => (await lensShown()) === "chat", 15))) throw new Error("the agent's pane did not come up on its chat");
        DRESS[paneA1] = { ...agentPane, agent: "" };
        if (!(await until(vacant, 30))) throw new Error("the page never saw the agent leave for its restart");
        DRESS[paneA1] = agentPane;
        if (!(await until(async () => !(await vacant()), 30))) throw new Error("the page never saw the agent come back");
        await sleep(600);
        check((await lensShown()) === "chat", "an agent's pane a restart passed through a shell did not go back to its chat");
      },
      expect: [".tabstrip .tab"],
      close: async () => {
        delete DRESS[paneN];
        await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=terminal` });
        if (!(await until(() => present(".pane-area .tab"), 15))) failed("the Agents page did not come back");
      },
    });
  await screen("new-tab-menu", { open: async () => tap(".tab-add"), expect: [".menu"] });
  await screen("view-menu", { open: async () => tap(".view-btn"), expect: [".menu"] });
  await screen("chat-lens", { open: async () => palette("lens:chat"), expect: [".lens-pane:not([hidden]) .chat-log, .lens-pane:not([hidden]) .composer"], close: async () => palette("lens:terminal") });
  // the calls the agent was denied without asking, over the composer, with the way to its terminal
  await screen("chat-denied", {
    open: async () => palette("lens:chat"),
    expect: [".lens-pane:not([hidden]) .chat-denied .chat-denied-call", ".lens-pane:not([hidden]) .chat-denied .linkish"],
    close: async () => palette("lens:terminal"),
  });
  if (!pass.phone)
    await screen("split-view", {
      open: async () => palette("split"),
      expect: [".split-chat", ".split-terminal"],
      close: async () => palette("split"),
    });
  await changesScreens();
  await screen("previews", {
    open: async () => {
      await onAgents();
      await palette("previews");
      if (!(await until(() => evaluate(`[...document.querySelectorAll(".modal .preview-row")].some((r) => r.textContent.includes("${devPort}"))`), 20))) failed(`the previews dialog does not list the dev server's port ${devPort}`);
    },
    expect: [".modal .preview-row"],
  });
  await screen("float-terminal", {
    open: async () => {
      await key("`", "Backquote", 2);
    },
    expect: [".float-term"],
    settle: 1200,
    close: async () => {
      await key("`", "Backquote", 2);
      await sleep(300);
    },
  });
  await screen("sharing", {
    open: async () => {
      await palette("sharing");
      await tap(".sharing-chip");
    },
    expect: [".sharing-chip", ".menu"],
    close: async () => {
      await closeAll();
      await palette("sharing");
    },
  });
  await screen("icon-dialog", { open: async () => palette("folder-icon"), expect: [".icon-dialog"], settle: 800, close: async () => {} });
  for (const [i, tab] of ["automatic", "emoji", "site", "picture"].entries())
    await screen(`icon-dialog-${tab}`, {
      open: async () => {
        await evaluate(`[...document.querySelectorAll('.icon-dialog .fv-tabs button')][${i}]?.click()`);
      },
      expect: [".icon-dialog"],
      settle: 500,
      close: async () => (i === 3 ? closeAll() : undefined),
    });

  await screen("wall-chat", { open: async () => palette("mode:wall"), expect: [".wall-bar", ".tile"] });
  await screen("wall-screen", { open: async () => palette("wall-view"), expect: [".wall-bar", ".tile"] });
  await screen("wall-zoo", { open: async () => palette("wall-zoo"), expect: [".zoo-grid .zoo-creature"] });
  if (!pass.phone) await screen("wall-office", { open: async () => palette("wall-office"), expect: [".wall-office"], close: async () => palette("wall-office") });

  await screen("tasks-board", { open: async () => palette("mode:tasks"), expect: [".board", ".board-col-note"] });
  await screen("project-settings", { open: async () => tap(".tasks-settings-btn"), expect: [".project-settings .ps-queue"], settle: 500, close: async () => {} });
  await screen("project-settings-agents", {
    open: async () => evaluate(`document.querySelector('.project-settings .ps-queue')?.scrollIntoView({ block: "center" })`),
    expect: [".project-settings .ps-queue input[type=checkbox]:checked", ".project-settings .retired-notice"],
    close: async () => {},
  });
  // the Inbox's agents were set up after the switch came, so their Claude Code does not wait for permissions
  await screen("project-settings-deny", {
    open: async () => evaluate(`document.querySelector('.project-settings .ps-deny')?.scrollIntoView({ block: "center" })`),
    expect: [".project-settings .ps-deny input[type=checkbox]:checked"],
    close: async () => {},
  });
  await screen("project-settings-pipeline", {
    open: async () => evaluate(`[...document.querySelectorAll('.project-settings .ps-stage')].pop()?.scrollIntoView({ block: "center" })`),
    expect: [".project-settings .ps-stage textarea"],
  });
  await screen("tasks-list", { open: async () => evaluate(`document.querySelector('.tasks-view button')?.click()`), expect: [".tasks-list .tasks-row"] });
  await screen("task-detail", { open: async () => tap(".tasks-row"), expect: [".tasks-detail"] });
  await screen("review", { open: async () => palette("mode:review"), expect: [".queue-head"] });
  await screen("code", { open: async () => palette("mode:code"), expect: [".code-mode"] });

  for (const panel of ["standup", "usage", "schedules", "activity", "alerts", "arenas"]) await screen(`panel-${panel}`, { open: async () => palette(panel), expect: [".modal"], settle: 600 });
  // a new agent schedule: Claude Code, its switch on, the words under it
  await screen("schedule-agent", {
    open: async () => {
      await palette("schedules");
      await tap(".modal.schedules .modal-head-controls .btn");
      if (!(await until(() => present(".schedule-editor .schedule-kind button"), 4))) throw new Error("the schedule editor did not open");
      await evaluate(`[...document.querySelectorAll('.schedule-editor .schedule-kind button')][2]?.click()`);
      await sleep(200);
      await evaluate(`document.querySelector('.schedule-editor .deny-switch')?.scrollIntoView({ block: "center" })`);
    },
    expect: [".schedule-editor .deny-switch input[type=checkbox]:checked", ".schedule-editor .deny-switch .schedule-note"],
    settle: 500,
  });
  await screen("usage-scorecard", {
    open: async () => {
      await palette("scorecard");
      if (!(await until(async () => (await count(".scorecard .score-row:not(.score-head)")) === 4, 6))) failed(`the Models tab draws ${await count(".scorecard .score-row:not(.score-head)")} rows, not the four models`);
      check((await count(".scorecard .score-cell.is-best")) >= 4, "the Models tab marks no best figure of a column");
      check((await count(".scorecard .score-cell.is-few")) >= 2, "a figure from too few runs is not told apart");
      check(await present(".usage-tabs button.on"), "the Usage panel's tabs say none is on");
    },
    expect: [".scorecard"],
    settle: 500,
    close: async () => {},
  });
  await screen("usage-scorecard-try", {
    open: async () => {
      await tap(".scorecard .score-try .icon-btn");
      if (!(await until(() => present(".arena-new"), 6))) return failed("Try it on a past task opens no arena");
      check((await count(".arena-new .arena-lane-row")) === 2, "the arena does not start with the two models side by side");
      check((await evaluate(`document.querySelector('.arena-new textarea')?.value ?? ""`)).includes("Draw every screen"), "the arena does not carry the past task's prompt");
    },
    expect: [".arena-new"],
    settle: 500,
  });
  await alertScreens();
  await screen("server-place", {
    open: async () => {
      await palette("server");
      if (!(await until(() => evaluate(`!!document.querySelector(".server-panel .health-rows") && document.querySelector(".server-panel").textContent.includes("77.4 °C")`), 4))) failed("the Server place does not show the host's temperature");
      check((await count(".server-panel .spark")) >= 4, `the Server place draws ${await count(".server-panel .spark")} sparklines, not the load, disk, memory and temperature`);
      check((await count(".server-concerns li.is-warn")) === 1, "a warm host is not said to want a look");
      if (!pass.phone) check(await present(".rail-btn.has-warn"), "the Server place's rail icon wears no amber dot for a warm host");
    },
    expect: [".server-panel"],
  });
  await screen("update-everywhere", { open: async () => palette("update-everywhere"), expect: [".modal.everywhere, .everywhere"], settle: 800 });
  await screen("settings", { open: async () => palette("settings"), expect: [".settings-dialog .settings-section"], close: async () => {} });
  const sections = (await evaluate(`document.querySelectorAll('.settings-dialog .settings-section').length`)) ?? 0;
  for (let i = 1; i < sections; i++)
    await screen(`settings-section-${i + 1}`, {
      open: async () => evaluate(`document.querySelectorAll('.settings-dialog .settings-section')[${i}].scrollIntoView({ block: "start" })`),
      expect: [".settings-dialog"],
      close: async () => {},
    });
  await screen("settings-backups", {
    open: async () => {
      await evaluate(`document.querySelector('.settings-dialog .backup-settings')?.scrollIntoView({ block: "start" })`);
      if (!(await until(async () => (await count(".backup-settings .backup-files li")) >= 1, 8))) failed("the Backups section lists no copy on the machine");
      check(await present(".backup-settings .notice"), "the Backups section does not say how the last run went");
    },
    expect: [".backup-settings"],
    close: async () => {},
  });
  await screen("settings-herdr", {
    open: async () => {
      await evaluate(`document.querySelector('.settings-dialog .mirror-settings')?.scrollIntoView({ block: "start" })`);
      check(await evaluate(`[...document.querySelectorAll('.mirror-settings .settings-row input[type=checkbox]')].map((box) => box.checked).join()`) === "true,false", "Inside herdr does not start with the figures on and the toasts off");
      check(await evaluate(`document.querySelector('.mirror-settings .config-snippet')?.textContent.includes("$remuda_cost")`), "Inside herdr shows no snippet naming remuda's tokens");
      await tap(".mirror-settings .backup-actions .btn:not(.sm)");
      if (!(await until(async () => (await count(".mirror-settings .mirror-outcomes li")) >= 1, 8))) failed("the test toast says nothing of how it went on the machine");
      check(await evaluate(`document.querySelector('.mirror-settings .mirror-outcomes')?.textContent.includes("ui.toast.delivery")`), "the test toast does not say that the throwaway herdr keeps its toasts off");
    },
    expect: [".mirror-settings .config-snippet"],
    close: async () => {},
  });
  await closeAll();
  await screen("palette", {
    open: async () => {
      await key("k", "KeyK", CMD);
    },
    expect: [".palette .palette-row"],
  });
  await screen("keys-sheet", { open: async () => palette("keys"), expect: [".modal"] });

  await onAgents();
  for (const [name, id] of [
    ["new-workspace", "workspace:local"],
    ["import-issue", "import-issue"],
    ["import-jira", "import-jira"],
    ["arena", "arena-new"],
    ["teleport", "teleport"],
    ["reopen", "reopen"],
    ["recipes", "recipes"],
    ["budget", "budgets"],
    ["broadcast", "broadcast"],
  ])
    await screen(`dialog-${name}`, { open: async () => palette(id), expect: [".modal"], settle: 700 });
}

let code = 0;
try {
  await seed();
  await send("Page.enable");
  await send("Runtime.enable");
  await send("Page.bringToFront");
  await send("Emulation.setFocusEmulationEnabled", { enabled: true }); // the page is the window in front, as at a desk
  await send("Fetch.enable", {
    patterns: [
      { urlPattern: "*/api/roster*", requestStage: "Response" },
      { urlPattern: "*/api/feed*", requestStage: "Response" },
      { urlPattern: "*/api/chat?*", requestStage: "Response" },
      { urlPattern: "*/api/opinion*", requestStage: "Request" },
      { urlPattern: "*/api/scorecard*", requestStage: "Request" },
      { urlPattern: "*/api/review/secrets*", requestStage: "Request" },
    ],
  });
  await send("Page.navigate", { url: `${base}/?token=${encodeURIComponent(token)}&machine=local&pane=${encodeURIComponent(paneA1)}&lens=terminal` });
  if (!(await until(async () => !(await evaluate("location.search")).includes("token=") && (await present(".topbar")), 15))) throw new Error("the page did not sign in");
  const started = Date.now();
  for (pass of PASSES) await runPass();
  await changesFlow().catch((error) => failed(`stopped: ${error.message}`));
  await worktreeFlow().catch((error) => failed(`stopped: ${error.message}`));
  await previewFlow().catch((error) => failed(`stopped: ${error.message}`));
  say(`${shots} screens in ${Math.round((Date.now() - started) / 1000)} s, screenshots in ${out}/display`);
} catch (error) {
  failed(`stopped: ${error.message}`);
} finally {
  await send("Page.navigate", { url: "about:blank" }).catch(() => {});
  await api("DELETE", "/api/scratch", { machine: "local" }).catch(() => {});
  for (const id of made.reverse()) await api("DELETE", `/api/sessions/${encodeURIComponent(id)}?machine=local`).catch(() => {});
  devServer.kill();
  socket.close();
}
if (failures.length) {
  console.error(`e2e display: ${failures.length} failure(s):`);
  for (const failure of failures) console.error(`e2e display: FAIL ${failure}`);
  code = 1;
} else say("every screen drawn and in its place");
process.exit(code);
