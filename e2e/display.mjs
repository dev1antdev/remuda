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
import { existsSync, mkdirSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { fakePullRequests, prScreens } from "./fake-pr.mjs";

const base = process.env.E2E_URL;
const cdp = process.env.E2E_CDP;
const token = process.env.E2E_TOKEN;
const out = process.env.E2E_OUT;
const tmp = process.env.E2E_TMP;
for (const [name, value] of Object.entries({ E2E_URL: base, E2E_CDP: cdp, E2E_TOKEN: token, E2E_OUT: out, E2E_TMP: tmp })) {
  if (!value) throw new Error(`${name} is not set`);
}

// the desks first: once Chrome has emulated a touch screen, (pointer: coarse) keeps matching after it stops
// each pass wears a theme other than Catppuccin's, so the display checks run on them: the desks a dark one, the phones a light one
const ALL_PASSES = [
  { name: "desk-en", width: 1440, height: 900, phone: false, lang: "en", look: { theme: "dark", "theme.dark": "nord" } },
  { name: "desk-ru", width: 1440, height: 900, phone: false, lang: "ru", look: { theme: "dark", "theme.dark": "tokyo-night" } },
  { name: "phone-en", width: 390, height: 844, phone: true, lang: "en", look: { theme: "light", "theme.light": "solarized-light" } },
  { name: "phone-ru", width: 390, height: 844, phone: true, lang: "ru", look: { theme: "light", "theme.light": "rose-pine-dawn" } },
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
    else if (url.includes("/api/pr/")) void fakePullRequests(msg.params, send);
    else if (url.includes("/api/concierge")) void fakeConcierge(msg.params);
    else if (url.includes("/api/scorecard")) void fakeScorecard(msg.params);
    else if (/\/api\/(scenes\/|scene-runs\/)/.test(url)) void fakeScenes(msg.params);
    else if (url.includes("/api/review/conflict")) void fakeConflictRead(msg.params);
    else if (/\/api\/review\?/.test(url)) void dressReview(msg.params);
    else if (url.includes("/api/review/secrets")) void fakeSecrets(msg.params);
    else if (url.includes("/api/timelapse/recordings") || url.includes("/api/timelapse/search")) void fakeTimelapse(msg.params);
    else if (url.includes("/api/standup")) void fakeStandup(msg.params);
    else if (url.includes("/api/recap")) void fakeRecap(msg.params);
    else if (url.includes("/api/fleet-setup")) void fakeFleet(msg.params);
    else if (url.includes("/api/undo")) void fakeUndo(msg.params);
    else if (url.includes("/api/limits/offers")) void fakeOffers(msg.params);
    else if (url.includes("/api/question")) void fakeQuestion(msg.params);
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
  // a row the palette reads from the server (a scene) comes a moment after the palette itself
  await until(() => evaluate(`!!document.querySelector('.palette-row[data-entry=${JSON.stringify(id)}]')`), 3);
  const ran = await evaluate(`(() => { const row = document.querySelector('.palette-row[data-entry=${JSON.stringify(id)}]'); if (!row) return false; row.scrollIntoView({ block: "nearest" }); row.click(); return true; })()`);
  if (!ran) {
    await key("Escape", "Escape");
    throw new Error(`the palette has no row ${id}`);
  }
  await sleep(250);
}

// conciergeHidden fails the screen when the concierge's workspace shows in the place given: its own place draws it alone
async function conciergeHidden(selector) {
  const shown = await evaluate(`[...document.querySelectorAll(${JSON.stringify(selector)})].some((el) => el.textContent.includes("concierge-e2e"))`);
  if (shown) failed(`the concierge's workspace shows in ${selector}`);
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
const deskDir = `${tmp}/display/concierge-e2e`; // the concierge's folder: a shell workspace the browser's roster marks as the concierge's
mkdirSync(appDir, { recursive: true });
mkdirSync(notesDir, { recursive: true });
mkdirSync(deskDir, { recursive: true });
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
// for the project tree: a folder git ignores, a link, a file whose icon has a light theme's own
writeFileSync(`${appDir}/.gitignore`, "build/\n");
writeFileSync(`${appDir}/app.toml`, 'name = "app"\n');
symlinkSync("README.md", `${appDir}/latest.md`);
// Claude Code's own approvals committed by mistake, one of them carrying a token: the lens head warns of it. The token
// is made here, its letters in another order, so no secret sits in this file
const shuffledKey = (n) => Array.from({ length: n }, (_, i) => "Zq8vK2mWp4Lr7Tb9Xs3Nd6Hc1JyQe5Gu0"[(i * 7 + 3) % 33]).join("");
mkdirSync(`${appDir}/.claude`, { recursive: true });
writeFileSync(`${appDir}/.claude/settings.local.json`, `${JSON.stringify({ permissions: { allow: ["Bash(npm test)", `Bash(curl -H 'Authorization: Bearer ${shuffledKey(40)}' https://deploy.internal)`] } }, null, 2)}\n`);
git(appDir, "add", "-A");
git(appDir, "add", "-f", ".claude/settings.local.json"); // past an excludes file that ignores it, as Claude Code sets one up
git(appDir, "commit", "-q", "-m", "init");
// a tag on the first commit, a bare origin it went to, and a second commit not pushed yet: the Git lens draws two
// commits, labels and a line changed with its words marked; the Changes lens has its "not on origin/main" and its scope
// against the base
git(appDir, "tag", "v0.1.0");
const originDir = `${tmp}/display/origin.git`;
git(`${tmp}/display`, "init", "-q", "--bare", originDir);
git(appDir, "remote", "add", "origin", originDir);
git(appDir, "push", "-q", "-u", "origin", "main");
writeFileSync(`${appDir}/README.md`, "# the app\n");
git(appDir, "commit", "-q", "-am", "docs: name the app");
writeFileSync(`${appDir}/src/app.go`, goLines("hello, world", 42));
writeFileSync(`${appDir}/docs/notes.md`, "# Notes\n\nThe first note.\nA second note, staged.\n");
git(appDir, "add", "docs/notes.md");
git(appDir, "rm", "-q", "old.txt");
writeFileSync(`${appDir}/src/new.ts`, 'export const fresh = "a new file";\n');
mkdirSync(`${appDir}/build`, { recursive: true });
writeFileSync(`${appDir}/build/out.txt`, "built\n");
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
let app, tree, notes, desk, inbox, drawTask, paneA1, paneN, paneDesk, morningScene, eveningScene;
let DRESS = {};
// the morning's first step lays its tab out as two panes, the agent on the left and a dev server on the right
const SCENE_LAYOUT = { agent: 0, root: { type: "split", direction: "right", ratio: 0.6, first: { type: "pane" }, second: { type: "pane", dir: "src", command: ["npm", "run", "dev"] } } };

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
  desk = await api("POST", "/api/sessions", { machine: "local", dir: deskDir, name: "concierge-e2e", agent: "shell" });
  made.push(desk.id);
  await api("PUT", "/api/jira", { site: "http://127.0.0.1:9", account: "", token: "e2e" }).catch((error) => failed(`a Jira site: ${error.message}`));
  // the person's own triggers, one everywhere and one on the repository's folder, for the triggers dialog; a regex
  // herdr could not read is refused before it reaches a machine's stream
  const rules = await api("PUT", "/api/triggers", [
    { name: "Deployed", match: "substring", pattern: "DEPLOYED", action: "push", enabled: true },
    { name: "Ready for QA", machine: "local", dir: appDir, match: "regex", pattern: "^ready for qa$", action: "task", column: "Testing", enabled: true },
  ]);
  if (rules.rules?.length !== 2) failed(`the triggers answered ${JSON.stringify(rules)}`);
  await api("PUT", "/api/triggers", [{ name: "Ahead", match: "regex", pattern: "(?=x)", action: "push", enabled: true }]).then(
    () => failed("a look-ahead herdr cannot read was kept"),
    () => {},
  );
  // a morning set-up with a captured layout and an evening tidy, saved through the real route; a run of either and the
  // tidy's plan are answered in the browser (fakeScenes), so nothing starts or closes
  const scenes = await api("PUT", "/api/scenes", [
    {
      name: "Morning",
      kind: "start",
      steps: [
        { machine: "local", dir: appDir, agent: "claude", model: "opus", prompt: "Read the overnight pull requests and sum them up", tab: "planner", layout: SCENE_LAYOUT },
        { machine: "local", dir: notesDir, agent: "shell", tab: "notes" },
      ],
    },
    { name: "Evening", kind: "tidy", tidy: { shells: true, idle: true, idle_hours: 8, compact: true, compact_at: 70, worktrees: true } },
  ]);
  [morningScene, eveningScene] = [scenes[0]?.id, scenes[1]?.id];
  if (!morningScene || !eveningScene || scenes[0].steps[0].dir !== appDir) failed(`the scenes answered ${JSON.stringify(scenes)}`);
  await api("PUT", "/api/scenes", [{ name: "Elsewhere", kind: "start", steps: [{ machine: "gog", dir: appDir, agent: "claude" }] }]).then(
    () => failed("a scene on a machine the fleet lacks was kept"),
    () => {},
  );
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
      anywhere: true, // the queue may start a card on whichever machine has room
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
    const d = list.find((s) => s.id === desk.id);
    return a?.tabs.length >= 2 && b && n?.tabs[0]?.panes[0] && d?.tabs[0]?.panes[0] ? { a, b, n, d } : null;
  }, 20);
  if (!ready) throw new Error("the seeded workspaces did not reach the roster");
  paneA1 = ready.a.tabs[0].panes[0].id;
  paneN = ready.n.tabs[0].panes[0].id;
  paneDesk = ready.d.tabs[0].panes[0].id;
  const paneA2 = ready.a.tabs[1].panes[0].id;
  const paneB = ready.b.tabs[0].panes[0].id;
  DRESS = {
    [paneA1]: {
      agent: "claude",
      status: "run",
      title: "Draw every screen of the page at two widths",
      radar: { state: "working", since: now - 3 * 60e3, last_active: now },
      // three subagents at work, the two oldest by name: the sidebar's tree under the agent's line
      subagents: { working: 3, session: "0c7a6c1e-2a43-4c1a-9a5e-3c1f0b6f8a11", list: [{ id: "agent-a1", description: "explore · cart service", doing: "Read · service.go" }, { id: "agent-a2", description: "test-runner", doing: "Bash · go test ./..." }] },
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
    [paneDesk]: { agent: "claude", status: "idle", title: "concierge", radar: { state: "idle", last_active: now - 60e3 } },
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

// PLAN_OFFER is a queued card Claude's 5-hour window holds at 96 %, offered to run now on Codex
const PLAN_OFFER = { id: "e2e-offer-1", kind: "task", project: 1, task: 12, ref: "#12", title: "Draw every screen", machine: "local", from: "claude", to: "codex", why: "Claude's 5 h window is at 96 % until 16:50", window: "5h", used: 96, resets_at: iso(now + 2 * 3600e3), at: iso(now - 60e3) };
let planOfferRuns = 0;

// fakeOffers answers the plan limits' offers: the one open, and its run, which the server would start on Codex
async function fakeOffers({ requestId, request }) {
  let body = { offers: [PLAN_OFFER] };
  if (request.method === "POST") {
    planOfferRuns++;
    body = { ok: true, words: "started #12 on this machine with Codex" };
  } else if (request.method === "DELETE") body = { ok: true };
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
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

// fakeScenes answers a scene's run and the tidy's plan as a busy evening would: the morning's agent started and its
// shell refused; four things to tidy, a machine not looked at, and their outcomes each way; a capture goes through
async function fakeScenes({ requestId, request }) {
  const path = new URL(request.url).pathname;
  if (path.endsWith("/capture")) {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const actions = [
    { id: `compact|local|${paneA1}`, kind: "compact", machine: "local", pane: paneA1, workspace: "app", agent: "claude", context: 82 },
    { id: "close-agent|local|w9:p1", kind: "close-agent", machine: "local", pane: "w9:p1", workspace: "app", tab: "review", agent: "codex", since: now - 11 * 3600e3, closes: "tab" },
    { id: `close-shell|local|${paneN}`, kind: "close-shell", machine: "local", pane: paneN, workspace: "notes", closes: "workspace" },
    { id: `worktree|local|${tmp}/.herdr/worktrees/app/old-idea`, kind: "worktree", machine: "local", path: `${tmp}/.herdr/worktrees/app/old-idea`, branch: "old-idea", kb: 18432 },
  ];
  const morning = (state) => [
    { index: 0, step: { machine: "local", dir: appDir, agent: "claude", model: "opus", tab: "planner", layout: SCENE_LAYOUT }, state: state ?? "started", pane: paneA1 },
    { index: 1, step: { machine: "local", dir: notesDir, agent: "shell", tab: "notes" }, state: state ?? "refused", note: "herdr says the folder is gone" },
  ];
  const evening = (state) => [
    { index: 0, action: actions[0], state: state ?? "done" },
    { index: 1, action: actions[1], state: state ?? "done" },
    { index: 2, action: actions[2], state: state ?? "skipped", note: "it works now" },
    { index: 3, action: actions[3], state: state ?? "refused", note: "uncommitted changes (M notes.md)" },
  ];
  const run = (scene, done) => ({ id: `run-${scene}`, scene, name: scene === eveningScene ? "Evening" : "Morning", kind: scene === eveningScene ? "tidy" : "start", at: iso(now), done, items: scene === eveningScene ? evening(done ? undefined : "waiting") : morning(done ? undefined : "waiting") });
  let body;
  if (path.endsWith("/plan")) body = { actions, unread: [{ machine: "gog", label: "GOG", why: "not connected" }] };
  else if (path.endsWith("/run")) body = run(path.split("/").at(-2), false);
  else body = run(path.split("/").at(-1).replace(/^run-/, ""), true);
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// fakeConflict is a screen asking for a conflict in the repository: dressReview adds src/clash.go, both sides edited,
// to the lens's summary of it, and fakeConflictRead answers the file with its markers; nothing is merged anywhere
let fakeConflict = false;
const isApp = (request) => (new URL(request.url).searchParams.get("dir") ?? "").replace(/\/+$/, "").endsWith("/display/app");
async function dressReview({ requestId, request, responseStatusCode }) {
  const reply = await send("Fetch.getResponseBody", { requestId });
  if (!fakeConflict || !isApp(request) || responseStatusCode !== 200 || !reply.result) {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const summary = JSON.parse(reply.result.base64Encoded ? Buffer.from(reply.result.body, "base64").toString("utf8") : reply.result.body);
  summary.files = [{ path: "src/clash.go", added: 0, removed: 0, fresh: true, conflict: "UU" }, ...(summary.files ?? [])];
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(summary)).toString("base64") });
}
async function fakeConflictRead({ requestId }) {
  const text = ["package main", "", "// greet says hello", "func greet() string {", "<<<<<<< HEAD", '\treturn "hello"', "=======", '\treturn "hello, world"', '\t// and to everyone', ">>>>>>> feature", "}", ""].join("\n");
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify({ stages: "123", text })).toString("base64") });
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

// fakeTimelapse answers the player's reads with a recording of the seeded agent's pane — forty minutes of a Claude Code
// screen at work, its spinner and its tool calls moving — and of an earlier pane of the folder; no recording is deleted
const TIMELAPSE_ID = "20261112T120000Z-e2e0aa";
function fakeRecording() {
  const start = now - 45 * 60e3;
  const rows = (i) => [
    "\x1b[38;2;215;119;87m╭" + "─".repeat(58) + "╮\x1b[0m",
    "\x1b[38;2;215;119;87m│\x1b[0m \x1b[1m✻ Welcome to Claude Code\x1b[0m" + " ".repeat(32) + " \x1b[38;2;215;119;87m│\x1b[0m",
    "\x1b[38;2;215;119;87m╰" + "─".repeat(58) + "╯\x1b[0m",
    "",
    "> Draw every screen of the page at two widths",
    ...Array.from({ length: Math.min(6, i) }, (_, n) => `\x1b[32m⏺\x1b[0m \x1b[1mEdit\x1b[0m(src/screen${n}.tsx)`),
    "",
    i < 11 ? `\x1b[38;2;215;119;87m${"✻✽✶✳"[i % 4]} Drawing… \x1b[0m\x1b[2m(${i * 4} min · esc to interrupt)\x1b[0m` : "\x1b[32m⏺\x1b[0m Every screen draws at both widths.",
  ];
  let previous = [];
  const frames = Array.from({ length: 12 }, (_, i) => {
    const screen = rows(i);
    const changed = screen.flatMap((text, line) => (previous[line] === text ? [] : [{ line, text }]));
    previous = screen;
    return { at: i * 4 * 60e3, lines: screen.length, rows: changed };
  });
  return { id: TIMELAPSE_ID, machine: "local", pane: paneA1, workspace: "app", dir: appDir, folder: appDir, agent: "claude", title: "Draw every screen", cols: 60, rows: 14, start: iso(start), end: iso(start + 44 * 60e3), count: frames.length, ended: "done", size: 4096, frames };
}
const earlierOf = (meta) => ({ ...meta, id: "20261111T090000Z-e2e0bb", pane: "w9:p1", agent: "codex", start: iso(now - 26 * 3600e3), end: iso(now - 25 * 3600e3) });
async function fakeTimelapse({ requestId, request }) {
  const recording = fakeRecording();
  const { frames, ...meta } = recording;
  const earlier = earlierOf(meta);
  const url = new URL(request.url);
  const path = url.pathname;
  const [code, body] =
    request.method !== "GET"
      ? [403, { error: "the e2e deletes no recording" }]
      : path.endsWith("/search")
        ? [200, fakeScreenSearch(url, recording)]
        : path.endsWith("/recordings")
          ? [200, { recordings: [meta, earlier], on: true }]
          : path.endsWith(TIMELAPSE_ID)
            ? [200, recording]
            : [200, { ...earlier, frames }];
  await send("Fetch.fulfillRequest", { requestId, responseCode: code, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// a recap as the server sums one up, with what the case at hand says
const recapOf = (over) => ({ prompts: 0, answers: 0, took: 0, tokens: 0, cost: 0, calls: 0, tools: {}, failed: 0, commands: [], edited: [], read: [], subagents: 0, questions: 0, asking: false, ...over });

// fakeStandup answers the standup with the dressed agents' day — the Codex asking, the Claude Code that finished with its
// files and its last answer, the one at work — so the panel, the zoo's foot and the voice have something to tell
async function fakeStandup({ requestId }) {
  const items = [
    { machine: "local", label: "This machine", workspace: "app", pane: Object.keys(DRESS).find((id) => DRESS[id].status === "input"), dir: appDir, agent: "codex", state: "blocked", recap: recapOf({ prompts: 1, answers: 1, took: 95, calls: 3, tools: { exec_command: 3 }, commands: ["go build ./..."], asking: true, cost: 0.4 }) },
    { machine: "local", label: "This machine", workspace: "feature-x", pane: Object.keys(DRESS).find((id) => DRESS[id].status === "done"), dir: appDir, agent: "claude", state: "done", recap: recapOf({ prompts: 4, answers: 4, took: 26 * 60, tokens: 182000, cost: 3.1, calls: 31, tools: { Edit: 12, Read: 14, Bash: 5 }, edited: [`${appDir}/src/flag.go`, `${appDir}/src/flag_test.go`, `${appDir}/NOTES.md`], read: [`${appDir}/src/app.go`], commands: ["go test ./...", "make web"], said: DRESS[Object.keys(DRESS).find((id) => DRESS[id].status === "done")]?.said }) },
    { machine: "local", label: "This machine", workspace: "app", pane: paneA1, dir: appDir, agent: "claude", state: "working", recap: recapOf({ prompts: 2, answers: 1, took: 12 * 60, cost: 4.2, calls: 18, tools: { Edit: 6, Bash: 4 }, edited: [`${appDir}/src/screen0.tsx`], commands: ["npm test"], said: "Every screen draws at both widths." }) },
  ];
  const body = { since: iso(new Date(new Date(now).setHours(0, 0, 0, 0)).getTime()), items, markdown: "# Standup\n" };
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// fakeRecap answers one conversation's recap, the zoo's creature telling its day among them
async function fakeRecap({ requestId }) {
  const body = { since: "", recap: recapOf({ prompts: 3, answers: 3, took: 18 * 60, cost: 2.2, calls: 12, tools: { Edit: 4, Bash: 2 }, edited: [`${appDir}/src/app.go`, `${appDir}/README.md`], commands: ["go test ./..."] }), markdown: "" };
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// FAKE_VOICE puts a stand-in voice where the browser's is: headless Chrome says nothing, so this one keeps what it is
// handed, starts each queue's first line and never ends it, and the screens are checked on what the page shows
const FAKE_VOICE = `(() => {
  if (window.__fakeVoice) return;
  const real = Object.getOwnPropertyDescriptor(window, "speechSynthesis");
  const said = [];
  const queue = [];
  const voice = {
    get speaking() { return queue.length > 0; },
    get pending() { return queue.length > 1; },
    paused: false,
    speak(line) { queue.push(line); said.push(line.text); if (queue.length === 1) setTimeout(() => queue[0] === line && line.onstart?.(), 0); },
    cancel() { for (const line of queue.splice(0)) line.onerror?.({ error: "interrupted" }); },
    getVoices() { return []; },
    pause() {}, resume() {}, addEventListener() {}, removeEventListener() {},
  };
  Object.defineProperty(window, "speechSynthesis", { configurable: true, value: voice });
  window.__fakeVoice = { said, real };
})()`;
const REAL_VOICE = `(() => {
  const fake = window.__fakeVoice;
  if (!fake) return;
  delete window.speechSynthesis;
  if (fake.real) Object.defineProperty(window, "speechSynthesis", fake.real);
  delete window.__fakeVoice;
})()`;
const sentence = () => evaluate(`document.querySelector('.listen-bar .listen-sentence')?.textContent ?? ""`);

// fakeScreenSearch answers the search over the screens as the server would for the two fake recordings, which show the
// same frames: where the query first stood, the rows around it, how long it stayed; the panel's search, over every
// recording, says older ones went unread
function fakeScreenSearch(url, recording) {
  const query = (url.searchParams.get("q") ?? "").trim().toLowerCase();
  const plain = (text) => text.replace(/\x1b\[[0-?]*[ -\/]*[@-~]/g, "").trimEnd();
  let screen = [];
  let first = null;
  let rows = [];
  recording.frames.forEach((frame, index) => {
    screen = screen.slice(0, frame.lines);
    while (screen.length < frame.lines) screen.push("");
    for (const row of frame.rows) screen[row.line] = plain(row.text);
    const line = screen.findIndex((text) => text.toLowerCase().includes(query));
    if (!first && line >= 0) [first, rows] = [{ frame: index, at: frame.at, for: 0, line }, [...screen]];
    else if (first && !first.for && line < 0) first.for = frame.at - first.at;
  });
  const unread = url.searchParams.get("pane") ? { unread: 0 } : { unread: 3, cut: "budget" };
  if (!first) return { hits: [], searched: 2, more: false, ...unread };
  if (!first.for) first.for = 44 * 60e3 - first.at;
  const { frames, ...meta } = recording;
  const hitOf = (m) => ({ recording: m.id, machine: m.machine, pane: m.pane, workspace: m.workspace, dir: m.dir, folder: m.folder, agent: m.agent, title: m.title, start: m.start, first, times: 1, before: rows.slice(Math.max(0, first.line - 2), first.line), row: rows[first.line], after: rows.slice(first.line + 1, first.line + 3), col: 0 });
  return { hits: [hitOf(meta), hitOf(earlierOf(meta))], searched: 2, more: false, ...unread };
}

// fakeFleet answers Fleet setup with four machines: this one, GOG lagging in every way the table tells — an older
// Claude Code with its update, remuda's MCP missing, a CLAUDE.md of its own, a checkout behind — a Pi alike but for its
// default model, and a box away; this machine's own Claude Code is offered its update, so Run… opens its Updates
async function fakeFleet({ requestId }) {
  const hash = (c) => c.repeat(64);
  const at = Math.round(now / 1000);
  const update = (what, title) => ({ what, title, command: `PATH=…; ${what === "claude" ? "claude update && claude --version" : '"$HOME/.local/bin/remuda" mcp install'}` });
  const body = {
    at: iso(now - 20e3),
    machines: [{ id: "local", label: "This machine · e2e" }, { id: "gog", label: "GOG" }, { id: "pi", label: "Pi" }, { id: "box", label: "Box", away: true }],
    sections: [
      {
        id: "agents",
        rows: [
          { id: "agent.version", arg: "claude", kind: "version", state: "odd", norm: "2.1.322", cells: { local: { value: "2.1.320", mark: "odd", action: update("claude", "Update Claude Code") }, gog: { value: "2.1.322" }, pi: { value: "2.1.322" } } },
          { id: "agent.via", arg: "claude", kind: "via", state: "same", norm: "native", cells: { local: { value: "native" }, gog: { value: "native" }, pi: { value: "native" } } },
          { id: "agent.version", arg: "codex", kind: "version", state: "odd", norm: "0.80.0", cells: { local: { value: "0.80.0" }, gog: { value: "0.80.0" }, pi: { value: "", mark: "odd" } }, hand: ["pi"] },
        ],
      },
      { id: "herdr", rows: [{ id: "herdr.hooks", arg: "claude", kind: "hook", state: "same", norm: "current", cells: { local: { value: "current" }, gog: { value: "current" }, pi: { value: "current" } } }] },
      { id: "remuda", rows: [{ id: "mcp", arg: "claude", kind: "flag", state: "odd", norm: "yes", cells: { local: { value: "yes" }, gog: { value: "no", mark: "odd", action: update("mcp", "Connect the agents to your tasks (MCP)") }, pi: { value: "yes" } } }] },
      { id: "claude", rows: [{ id: "claude.model", kind: "model", state: "odd", norm: "claude-opus-4-8", cells: { local: { value: "claude-opus-4-8" }, gog: { value: "claude-opus-4-8" }, pi: { value: "default", mark: "odd" } }, hand: ["pi"] }] },
      { id: "instructions", rows: [{ id: "instructions", arg: "claude", kind: "hash", state: "odd", norm: hash("a"), cells: { local: { value: hash("a"), at: at - 86400 }, gog: { value: hash("b"), mark: "odd", at: at - 7200 }, pi: { value: hash("a"), at: at - 3 * 86400 } }, hand: ["gog"] }] },
      {
        id: "repo",
        name: "owner/app",
        folders: { local: appDir, gog: "/home/someone/app" },
        rows: [
          { id: "repo.branch", kind: "text", state: "same", norm: "main", cells: { local: { value: "main" }, gog: { value: "main" } } },
          { id: "repo.behind", kind: "behind", state: "odd", norm: "0", cells: { local: { value: "0", at: at - 600, note: "origin/main" }, gog: { value: "3", mark: "odd", at: at - 5 * 3600, note: "origin/main" } }, hand: ["gog"] },
          { id: "repo.file", arg: "CLAUDE.md", kind: "hash", state: "split", cells: { local: { value: hash("c"), mark: "split", at: at - 3600 }, gog: { value: hash("d"), mark: "split", at: at - 30 * 3600 } }, hand: ["local", "gog"] },
        ],
      },
    ],
    single: [{ name: "owner/notes", machine: "pi", folder: "/home/someone/notes" }],
  };
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// fakeUndo answers the page's undo routes with a kept turn of the dressed pane that edited, added, deleted and committed,
// and a plan of it already pushed: the dialog shows every kind of file and the revert, and the e2e undoes nothing
const UNDO_ID = "20261112T100000Z-e2e0cc";
const UNDO_CHAT = [
  { role: "user", ts: iso(now - 12 * 60e3), ids: ["e2e-undo-prompt"], parts: [{ type: "text", text: "Draw the missing screens." }] },
  { role: "assistant", ts: iso(now - 11 * 60e3), ids: ["e2e-undo-answer"], parts: [{ type: "text", text: "Drew them, and committed the screens." }], took: 150 },
];
let undoChat = false;
// PROMPTS_CHAT is the dressed pane's conversation for the chat's own screens: an answer whose calls make one card — a
// read, an edit with its change, a test that failed and passed again — then a turn under way, a message the person sent
// while it worked inside it, its last call still running; the question route says what Claude Code's spinner says
const PROMPTS_SESSION = "0e2e0000-7777-4000-8000-0000000000aa";
const PROMPTS_CHAT = [
  { role: "user", ts: iso(now - 14 * 60e3), ids: ["e2e-p1"], parts: [{ type: "text", text: "Draw the **prompts list** beside the chat, newest first." }] },
  {
    role: "assistant",
    ts: iso(now - 14 * 60e3 + 3e3),
    ids: ["e2e-p1a"],
    took: 252,
    tokens: 38000,
    output: 9100,
    cost: 0.42,
    parts: [
      { type: "text", text: "I'll start with the cards." },
      { type: "tool", tool_name: "Read", tool_id: "toolu_e2e_p1", at: iso(now - 14 * 60e3 + 4e3), took: 0.2, input: { file_path: `${appDir}/src/app.go` }, output: "package main" },
      { type: "tool", tool_name: "Edit", tool_id: "toolu_e2e_p2", at: iso(now - 14 * 60e3 + 9e3), took: 0.4, input: { file_path: `${appDir}/src/app.go`, old_string: 'fmt.Println("hello")', new_string: 'fmt.Println("hello, prompts")' }, output: "The file was updated." },
      { type: "thinking", text: "The test should cover the newest-first order." },
      { type: "tool", tool_name: "Bash", tool_id: "toolu_e2e_p3", at: iso(now - 13 * 60e3), took: 3.1, input: { command: "go test ./..." }, output: "--- FAIL: TestPrompts", is_error: true },
      { type: "tool", tool_name: "Bash", tool_id: "toolu_e2e_p4", at: iso(now - 12 * 60e3), took: 2.7, input: { command: "go test ./..." }, output: "ok" },
      { type: "text", text: "Drew the list and its sheet on a phone. The tests pass now; the card opens in place and the log jumps to its turn." },
    ],
  },
  { role: "user", ts: iso(now - 6 * 60e3), ids: ["e2e-p2"], parts: [{ type: "text", text: "Now show the spinner's progress over the composer." }] },
  {
    role: "assistant",
    ts: iso(now - 6 * 60e3 + 2e3),
    ids: ["e2e-p2a", "e2e-p2m"],
    output: 2100,
    parts: [
      { type: "tool", tool_name: "Read", tool_id: "toolu_e2e_p5", at: iso(now - 5 * 60e3), took: 0.1, input: { file_path: `${appDir}/README.md` }, output: "# app" },
      { type: "queued", text: "Show it in the **wall tile** too.", at: iso(now - 4 * 60e3) },
      { type: "tool", tool_name: "Bash", tool_id: "toolu_e2e_p6", at: iso(now - 20e3), input: { command: "npm test -- progress" } },
    ],
  },
];
let promptsChat = false;
// windowsLocal plays the local machine as a Windows PC in the dressed roster, for the screen terminal's screen alone
let windowsLocal = false;
function fakeUndoTurn() {
  const file = (path, was, extra = {}) => ({ path, was, dirty_at: false, dirty_to: true, ...extra });
  return {
    id: UNDO_ID, machine: "local", dir: appDir, root: appDir, pane: paneA1, agent: "Claude Code", started: iso(now - 12 * 60e3), ended: iso(now - 9.5 * 60e3), branch: "main", size: 2048,
    files: [file("src/app.go", "changed"), file("src/screens/new-screen.tsx", "added"), file("docs/old-notes.md", "deleted"), file("web/public/logo.png", "changed", { binary: true }), file("src/screens/index.ts", "changed", { committed: true, dirty_to: false })],
    commits: [{ sha: "4f2c9a1d0b7e6f5a4c3b2a1908f7e6d5c4b3a291", subject: "Draw the missing screens", when: Math.floor((now - 10 * 60e3) / 1000) }],
  };
}
async function fakeUndo({ requestId, request }) {
  const path = new URL(request.url).pathname;
  const turn = fakeUndoTurn();
  const older = { ...turn, id: "20261112T080000Z-e2e0cd", started: iso(now - 3 * 3600e3), ended: iso(now - 2.9 * 3600e3), commits: [], undone: { at: iso(now - 2 * 3600e3), files: 2 } };
  const [code, body] =
    request.method !== "GET" ? [403, { error: "the e2e undoes nothing" }] : path.endsWith("/plan") ? [200, { turn, uncommitted: 4, pushed: ["origin/main"], left_as_is: [] }] : [200, { turns: [turn, older].map((one) => ({ ...one, files: one.files.length })), recording: true }];
  await send("Fetch.fulfillRequest", { requestId, responseCode: code, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// ASKED is what the dressed Codex asks on its screen, which the sidebar's "Needs you" and the zoo's bubble quote
const ASKED = "Bash command\ngo build ./...\nbuild the app\nDo you want to proceed?";

// fakeQuestion answers a read of a pane's dialog: while the chat's own screens are up the dressed pane's screen shows
// Claude Code's spinner; else the asking pane's dialog, none for the rest; an answer is passed on
async function fakeQuestion({ requestId, request }) {
  if (request.method !== "GET") {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const pane = new URL(request.url).searchParams.get("pane");
  const answer =
    promptsChat && pane === paneA1
      ? { question: null, mode: "acceptEdits", progress: { verb: "Refactoring", seconds: 134, tokens: 6800 } }
      : { question: DRESS[pane]?.status === "input" ? { text: ASKED, options: [{ label: "Yes" }, { label: "No" }] } : null };
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(answer)).toString("base64") });
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
    if (windowsLocal) machine.os = "windows"; // the screen terminal's screen: this machine played as a Windows PC
    // Claude Code's sign-in there ends in a day and a half at the latest, so it may end at any time: a red concern with a shell to sign in again from;
    // its settings keep fast mode on Opus 4.8 as the default, so every dialog that starts Claude Code on its default model says it goes on 30 Nov
    const health = machine.health ?? { at: iso(now), disk_free_gb: 120, disk_used_pct: 30, load1: 0.4 };
    // and it runs on a battery more than half full, which the ⓘ says
    machine.health = { ...health, claude: health.claude || "2.1.322", claude_model: "claude-opus-4-8", claude_fast: true, claude_login: { expires: iso(now + 36 * 3600e3), at: iso(now) }, battery: { percent: 64, discharging: true } };
    // Claude's 5 hours run out at this pace fifty minutes before they reset; its week lasts
    machine.limits = {
      wrapped: true,
      windows: [
        { agent: "claude", name: "5h", used: 62, resets_at: iso(now + 2 * 3600e3), at: iso(now), forecast: { runs_out_at: iso(now + 70 * 60e3), pace: 32.6 } },
        { agent: "claude", name: "week", used: 86, resets_at: iso(now + 3 * 86400e3), at: iso(now), forecast: { lasts: true, pace: 0.2 } },
        { agent: "codex", name: "week", used: 31, resets_at: iso(now + 5 * 86400e3), at: iso(now) },
      ],
    };
    for (const session of machine.sessions ?? []) {
      if (session.id === app.id || session.id === tree.id) session.icon = { emoji: "🦊" }; // the icon is the folder's, the worktree's too
      if (session.id === desk?.id) session.concierge = true; // the page shows it in the concierge's place alone
      if (session.id === app.id) {
        session.jira = [{ key: "APP-12", summary: "Draw every screen", status: "In Review", category: "indeterminate", url: "https://example.atlassian.net/browse/APP-12" }];
        session.sync = { ahead: 2, behind: 1 }; // the selected card says how far it stands from its upstream
      }
      dressCollisions(session);
      for (const tab of session.tabs ?? []) for (const pane of tab.panes ?? []) Object.assign(pane, DRESS[pane.id] ?? {});
    }
  }
  roster.feed = FEED.id; // the activity's newest id as this run sets it: a page that saw an older one reads the feed
  roster.offers = [PLAN_OFFER.id]; // a queued card Claude's plan limit holds, offered on Codex (fakeOffers answers it)
  roster.server = SERVER_HOST; // the Server place a remuda -server draws: its host's vitals, the readings a Raspberry Pi gives
  const headers = responseHeaders.filter((h) => !/^(etag|content-length|content-encoding)$/i.test(h.name));
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: headers, body: Buffer.from(JSON.stringify(roster)).toString("base64") });
}

// dressCollisions has the repository and its worktree change the same lines of src/app.go — a lightning between their
// creatures in the zoo — and the repository the same file as a clone on a machine not on the wall, a bolt on its
// creatures; each with a risk the Review mode's queue sorts and draws
function dressCollisions(session) {
  const peer = (other, name) => ({ machine: "local", workspace: other.id, name, dir: other.id === app.id ? appDir : `${appDir}-feature-x` });
  const files = [{ path: "src/app.go", lines: true, from: 6, to: 12 }, { path: "README.md", lines: false }];
  if (session.id === app.id) {
    session.collisions = [
      { with: peer(tree, "feature-x"), files, lines: true, first: true, why: "pr", main: "main" },
      { with: { machine: "gog", workspace: "w9", name: "api", dir: "/srv/app" }, files: [{ path: "go.mod", lines: false }], lines: false, first: false, why: "smaller", main: "origin/main" },
    ];
    const reasons = [
      { kind: "checks", points: 20, count: 1 },
      { kind: "collision", points: 20, same: true, with: "feature-x" },
      { kind: "migration", points: 15, count: 1, paths: ["db/migrations/0042_users.sql"] },
      { kind: "untested", points: 5, count: 1, paths: ["src/app.go"] },
    ];
    session.review = { ...(session.review ?? { changed: 4, fresh: 4 }), risk: { score: 60, level: "high", reasons } };
  }
  if (session.id === tree.id) {
    session.collisions = [{ with: peer(app, "app"), files, lines: true, first: false, why: "pr", main: "main" }];
    session.review = { ...(session.review ?? { changed: 2, fresh: 2 }), risk: { score: 20, level: "medium", reasons: [{ kind: "collision", points: 20, same: true, with: "app" }] } };
  }
}

// DENIED is what the dressed Claude Code pane's conversation was denied without asking, for the chat lens's notice
const DENIED = [
  { tool: "Bash", what: "npm install --save-dev vitest", at: iso(now - 20 * 60e3), tool_id: "toolu_e2e_1" },
  { tool: "Write", what: `${tmp}/display/app/src/new-screen.tsx`, at: iso(now - 18 * 60e3), tool_id: "toolu_e2e_2" },
];

// dressChat adds DENIED to the lens's answer for the dressed agent pane (a tile's tail, last=N, carries none)
async function dressChat({ requestId, request, responseStatusCode, responseHeaders = [] }) {
  const url = new URL(request.url);
  if (url.searchParams.get("pane") === paneDesk) {
    await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(conciergeChat())).toString("base64") });
    return;
  }
  const reply = responseStatusCode === 200 && url.searchParams.get("pane") === paneA1 && !url.searchParams.has("last") ? await send("Fetch.getResponseBody", { requestId }) : null;
  if (!reply?.result) {
    await send("Fetch.continueRequest", { requestId });
    return;
  }
  const chat = JSON.parse(reply.result.base64Encoded ? Buffer.from(reply.result.body, "base64").toString("utf8") : reply.result.body);
  chat.denied = DENIED;
  if (undoChat) chat.turns = [...(chat.turns ?? []), ...UNDO_CHAT];
  if (promptsChat) {
    chat.turns = PROMPTS_CHAT;
    chat.session_id ||= PROMPTS_SESSION;
  }
  const headers = responseHeaders.filter((h) => !/^(etag|content-length|content-encoding)$/i.test(h.name));
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: headers, body: Buffer.from(JSON.stringify(chat)).toString("base64") });
}

// fakeConcierge answers the concierge's place as if Claude Code ran in the concierge-e2e workspace on this machine, a
// Mac: where it runs, a start, a new conversation; no agent is asked anything
async function fakeConcierge({ requestId, request }) {
  const running = { machine: "local", workspace: desk.id, pane: paneDesk, dir: deskDir, started: false };
  const path = new URL(request.url).pathname;
  const view = { machine: "local", machines: [{ id: "local", label: "This Mac · e2e", connected: true, mac: true }], running };
  const body = request.method === "GET" || request.method === "PUT" ? view : request.method === "DELETE" ? { ok: true } : path.endsWith("/new") ? { ...running, started: true } : running;
  await send("Fetch.fulfillRequest", { requestId, responseCode: 200, responseHeaders: [{ name: "Content-Type", value: "application/json" }], body: Buffer.from(JSON.stringify(body)).toString("base64") });
}

// conciergeChat is the concierge's conversation: a question, the tool it looked through, the answer
const conciergeChat = () => ({
  session_id: "0e2e0000-1111-2222-3333-444455556666",
  sessions: [],
  turns: [
    { role: "user", ts: iso(now - 120e3), ids: ["e2e-c1"], parts: [{ type: "text", text: "What did the agents finish today?" }] },
    {
      role: "assistant",
      ts: iso(now - 100e3),
      ids: ["e2e-c2"],
      parts: [
        { type: "tool", tool_name: "mcp__remuda__standup", tool_id: "toolu_e2e_c1", at: iso(now - 115e3), took: 1.2, input: { since: "midnight" }, output: "# Standup" },
        { type: "text", text: "Two finished today:\n\n- **feature-x** on this Mac renamed the flag everywhere it is read; the tests pass.\n- **app** opened PR #15, one check failing.\n\nA Codex in **app** waits for your answer." },
      ],
    },
  ],
});

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
// setPref writes a pref the way another tab of the page would, so every hook of it reads it again at once
const setPref = (key, value) =>
  evaluate(`(() => { localStorage.setItem(${JSON.stringify(`remuda.${key}`)}, ${JSON.stringify(JSON.stringify(value))}); dispatchEvent(new StorageEvent("storage", { key: ${JSON.stringify(`remuda.${key}`)} })); })()`);
const folderHead = (name) => `[...document.querySelectorAll('.ws-folder-head')].find((h) => h.querySelector('.ws-folder-name')?.textContent === ${JSON.stringify(name)})`;
const firstCardOf = (folder) => `${folderHead(folder)}?.parentElement.querySelector('.ws-item')`;
const mark = async (expression, attr) => {
  const found = await evaluate(`(() => { document.querySelectorAll('[data-e2e]').forEach((e) => e.removeAttribute('data-e2e')); const el = ${expression}; if (!el) return false; el.setAttribute('data-e2e', ${JSON.stringify(attr)}); return true; })()`);
  if (!found) throw new Error(`no ${attr} on the screen`);
  return `[data-e2e="${attr}"]`;
};
// onAgents comes back to the Agents mode on the seeded agent's pane
// sceneScreens are the Scenes dialog with the seeded morning and evening, each one's form, and a run of each: what it
// will do first, then its outcome as the fake answers it
async function sceneScreens() {
  // a long line of a scene's words is cut at its column, never pushing the dialog into a sideways scroll
  const flat = async () => check(await evaluate(`[...document.querySelectorAll(".modal")].every((m) => m.scrollWidth <= m.clientWidth + 1)`), "the dialog scrolls sideways");
  const openScenes = async () => {
    await palette("scenes");
    if (!(await until(async () => (await count(".modal .scene-row")) === 2, 10))) failed(`the scenes dialog lists ${await count(".modal .scene-row")} scenes, not 2`);
    await flat();
  };
  await screen("scenes", { open: openScenes, expect: [".modal .scene-row"] });
  await screen("scene-morning", {
    open: async () => {
      await openScenes();
      await tap(".modal .scene-row:nth-child(1) > .btn:nth-of-type(2)");
      check((await count(".modal .scene-step")) === 1, "the morning's second step is not listed under its open first one");
      await flat();
    },
    expect: [".modal .scene-form", ".modal .scene-step-form", ".modal .scene-layout"],
  });
  await screen("scene-evening", {
    open: async () => {
      await openScenes();
      await tap(".modal .scene-row:nth-child(2) > .btn:nth-of-type(2)");
      await flat();
    },
    expect: [".modal .scene-tidy", ".modal .scene-check .scene-number"],
  });
  await screen("scene-run-morning", {
    open: async () => {
      await palette(`scene:${morningScene}`);
      if (!(await until(async () => (await count(".modal .scene-picks .pick-row")) === 2, 10))) failed("the morning's run does not list its two steps");
      await flat();
    },
    expect: [".modal .scene-picks", ".modal .modal-actions .btn.primary"],
  });
  await screen("scene-run-morning-outcome", {
    open: async () => {
      await palette(`scene:${morningScene}`);
      await until(async () => (await count(".modal .scene-picks .pick-row")) === 2, 10);
      await tap(".modal .modal-actions .btn.primary");
      if (!(await until(async () => (await count(".modal .scene-outcome.is-ok")) === 1 && (await count(".modal .scene-outcome.is-bad")) === 1, 10))) failed("the morning's outcome is not one started and one refused");
      await flat();
    },
    expect: [".modal .scene-outcomes", ".modal .scene-outcome .btn"],
  });
  await screen("scene-run-evening", {
    open: async () => {
      await palette(`scene:${eveningScene}`);
      if (!(await until(async () => (await count(".modal .scene-picks .pick-row")) === 4, 10))) failed("the evening's plan does not list its four actions");
      await flat();
    },
    expect: [".modal .scene-picks", ".modal .notice"],
  });
  await screen("scene-run-evening-outcome", {
    open: async () => {
      await palette(`scene:${eveningScene}`);
      await until(async () => (await count(".modal .scene-picks .pick-row")) === 4, 10);
      await tap(".modal .modal-actions .btn.primary");
      if (!(await until(async () => (await count(".modal .scene-outcome")) === 4, 10))) failed("the evening's outcome does not list its four actions");
      await flat();
    },
    expect: [".modal .scene-outcome.is-ok", ".modal .scene-outcome.is-bad", ".modal .scene-outcome.is-quiet"],
  });
}

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
      // git's groups: the staged edit and deletion, the edit not staged, the new file; the commit not on origin/main
      const groups = { staged: await inGroup("staged"), unstaged: await inGroup("unstaged"), untracked: await inGroup("untracked") };
      check(JSON.stringify(groups) === JSON.stringify({ staged: 2, unstaged: 1, untracked: 1 }), `the groups hold ${JSON.stringify(groups)}`);
      check((await count(`${LENS} .review-group.is-outgoing .review-outgoing`)) === 1, "the commit not on origin/main is not listed");
      // the agent dressed at work in the repository holds every write back, and the commit box says why
      check(await evaluate(`(() => { const acts = [...document.querySelectorAll('${LENS} .review-act')]; return acts.length > 0 && acts.every((b) => b.getAttribute("aria-disabled") === "true"); })()`), "a write is offered while the agent works there");
      check(/Claude Code/.test(await evaluate(`document.querySelector('${LENS} .commit-hint')?.textContent ?? ""`)), "the commit box does not name the agent at work");
    },
    expect: [`${LENS} .review-head`, `${LENS} .review-collision`, `${LENS} .commit-box`],
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
      await tap(`${LENS} .review-head .review-columns`);
      if (!(await until(async () => (await count(`${LENS} .review-diff.columns`)) === (pass.phone ? 1 : 0), 3))) failed("the columns switch did not switch");
    },
    expect: [`${LENS} .review-hunk`],
    close: async () => {
      await tap(`${LENS} .review-head .review-columns`);
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
  // undoing the agent's last turn: every kind of file, the commit offered as a revert, the push it already went out in
  await screen("changes-undo", {
    open: async () => {
      await tap(`${LENS} .review-undo`);
      if (!(await until(async () => (await count(".modal .undo-files .pick-row")) === 5, 6))) failed(`the undo lists ${await count(".modal .undo-files .pick-row")} files, not 5`);
      check(await present(".modal .undo-revert input:checked"), "the turn's commit is not offered as a revert");
      check(await present(".modal .notice.warn"), "the dialog does not say the commit was pushed already");
      check(await evaluate(`document.querySelector('.modal .btn.danger')?.disabled === false`), "Undo this turn is held back");
    },
    expect: [".modal .undo-files", ".modal .undo-commit-list"],
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
      await tap(await mark(`${fileHead("src/app.go")}?.parentElement.querySelector('.review-open')`, "open-file"));
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
  await stagingScreens();
  await palette("lens:terminal");
}

// a row of a group by its key, its own button for an action, and the files a group lists
const rowOf = (key) => `${LENS} .review-file[data-key=${JSON.stringify(key)}]`;
const actionOf = (key, action) => `${rowOf(key)} .review-act.is-${action}`;
const inGroup = (group) => count(`${LENS} .review-group.is-${group} .review-file`);
const gitStatus = () => execFileSync("git", ["status", "--porcelain=v1"], { cwd: appDir, encoding: "utf8" });
// pressRow presses a row's button: the pointer over the row first, where its counts give way to its buttons
async function pressRow(key, action) {
  await hover(`${rowOf(key)} .review-file-row`);
  await tap(actionOf(key, action));
}

// stagingScreens are the lens as a client of the repository: the tree, the commits not on origin/main and the scope
// against the base on the seeded work; then, with the agents in the repository dressed at rest, a file staged through
// the lens and back (the e2e's own throwaway repository, put back each pass), the commit box held by the secret check,
// and a conflict through a faked summary; the agents are dressed at work again afterwards
async function stagingScreens() {
  await screen("changes-tree", {
    open: async () => {
      await tap(`${LENS} .review-tree-toggle`);
      if (!(await until(async () => (await count(`${LENS} .review-folder`)) === 3, 3))) failed(`the tree shows ${await count(`${LENS} .review-folder`)} folders, not docs/ and src/ twice`);
      check((await count(`${LENS} .review-file`)) === CHANGED.length, "a folder of the tree hides its files");
    },
    expect: [`${LENS} .review-folder`],
    close: async () => {
      await tap(`${LENS} .review-tree-toggle`);
      await until(async () => (await count(`${LENS} .review-folder`)) === 0, 3);
    },
  });
  await screen("changes-outgoing", {
    open: async () => {
      check(/origin\/main/.test(await evaluate(`document.querySelector('${LENS} .review-group.is-outgoing .review-group-title')?.textContent ?? ""`)), "the group does not name origin/main");
      await tap(`${LENS} .review-outgoing-head`);
      if (!(await until(() => present(`${LENS} .review-outgoing.open .review-file`), 6))) throw new Error("the commit's files did not open");
      await tap(`${LENS} .review-outgoing.open .review-file-head`);
      if (!(await until(() => present(`${LENS} .review-outgoing.open .review-hunk`), 6))) failed("the commit's file did not open on its diff");
    },
    expect: [`${LENS} .review-outgoing.open .review-hunk`],
    close: async () => {
      await tap(`${LENS} .review-outgoing-head`);
      await sleep(200);
    },
  });
  await screen("changes-base", {
    open: async () => {
      await tap(`${LENS} .review-scope button:last-child`);
      if (!(await until(async () => (await count(`${LENS} .review-group.is-base .review-file`)) === CHANGED.length + 1, 8))) failed(`vs base lists ${await count(`${LENS} .review-group.is-base .review-file`)} files, not ${CHANGED.length + 1}`);
      check(/origin\/main/.test(await evaluate(`document.querySelector('${LENS} .review-base-line')?.textContent ?? ""`)), "the head does not name the base");
      check(!(await present(`${LENS} .review-act`)) && !(await present(`${LENS} .commit-box`)), "the base scope offers a write");
    },
    expect: [`${LENS} .review-base-line`],
    close: async () => {
      await tap(`${LENS} .review-scope button:first-child`);
      await until(() => present(`${LENS} .review-group.is-unstaged`), 5);
    },
  });
  const atWork = { ...DRESS };
  for (const paneId of Object.keys(DRESS)) if (DRESS[paneId].radar?.state === "working" || DRESS[paneId].radar?.state === "blocked") DRESS[paneId] = { ...DRESS[paneId], radar: { ...DRESS[paneId].radar, state: "idle" } };
  try {
    await screen("changes-stage", {
      open: async () => {
        await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=changes` });
        if (!(await until(async () => (await inGroup("unstaged")) === 1, 15))) throw new Error("the lens did not come back on the repository's work");
        if (!(await until(() => evaluate(`(() => { const button = document.querySelector(${JSON.stringify(actionOf("unstaged:src/app.go", "stage"))}); return !!button && button.getAttribute("aria-disabled") !== "true"; })()`), 6))) throw new Error("Stage is held back with no agent at work");
        await pressRow("unstaged:src/app.go", "stage");
        if (!(await until(async () => (await inGroup("staged")) === 3 && !(await present(`${LENS} .review-group.is-unstaged`)), 8))) failed("src/app.go did not move to Staged");
        check(/^M {2}src\/app\.go$/m.test(gitStatus()), `git does not have src/app.go staged: ${gitStatus()}`);
      },
      expect: [`${LENS} .review-group.is-staged`],
      close: async () => {
        await pressRow("staged:src/app.go", "unstage");
        if (!(await until(async () => (await inGroup("unstaged")) === 1, 8))) failed("Unstage did not put src/app.go back");
      },
    });
    await screen("changes-commit", {
      open: async () => {
        await evaluate(`document.querySelector('${LENS} .commit-message')?.focus()`); // the toast of the stage just undone may still lie over the box on a phone
        await send("Input.insertText", { text: "Say hello to the world" });
        if (!(await until(() => present(`${LENS} .commit-box .secret-check.is-found`), 8))) failed("the secret check does not stand before the commit");
        check(await evaluate(`document.querySelector('${LENS} .commit-go')?.disabled === true`), "Commit is not held back by the findings");
        check(await present(`${LENS} .commit-anyway`), "no Commit anyway… beside the held button");
        await evaluate(`document.querySelector('${LENS} .commit-box')?.scrollIntoView({ block: "center" })`);
      },
      expect: [`${LENS} .commit-box .secret-check`],
      close: async () => {
        // the message taken out again, so the next pass's box starts empty: React hears the field's own input event
        await evaluate(`(() => { const box = document.querySelector('${LENS} .commit-message'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value").set.call(box, ""); box.dispatchEvent(new Event("input", { bubbles: true })); })()`);
        await until(async () => !(await present(`${LENS} .commit-box .secret-check`)), 3);
      },
    });
    await screen("changes-conflict", {
      open: async () => {
        fakeConflict = true;
        await tap(`${LENS} .review-look-again`);
        if (!(await until(() => present(`${LENS} .review-group.is-conflict .review-file`), 6))) throw new Error("the faked conflict is not listed apart");
        check(await present(`${LENS} .review-group.is-conflict .review-conflict-kind`), "the conflict does not say its kind");
        check(await evaluate(`document.querySelector('${LENS} .commit-go')?.disabled === true`), "Commit is offered over a conflict");
        await tap(`${LENS} .review-group.is-conflict .review-file-head`);
        if (!(await until(() => present(`${LENS} .conflict-view .fv-line.yours`), 6))) failed("the conflict's blocks are not drawn");
        check(await present(`${LENS} .conflict-view .fv-line.marker`), "the markers are not on their band");
        check((await count(`${LENS} .conflict-view.columns`)) === (pass.phone ? 0 : 1), "yours and incoming are not side by side at a desk");
        await evaluate(`document.querySelector('${LENS} .conflict-view')?.scrollIntoView({ block: "center" })`);
      },
      expect: [`${LENS} .conflict-view`],
      close: async () => {
        fakeConflict = false;
        await tap(`${LENS} .review-look-again`);
        await until(async () => !(await present(`${LENS} .review-group.is-conflict`)), 5);
      },
    });
  } finally {
    DRESS = atWork;
    await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=changes` });
    await until(() => present(`${LENS} .review-head`), 15);
  }
}

// ---- the project's files: the files rail's tree, a row's menu, the file viewer's changed lines ----

const RAIL = ".footprint";
const treeRow = (path) => `[...document.querySelectorAll('${RAIL} .tree-open')].find((b) => b.title.split("\\n")[0] === ${JSON.stringify(path)})?.closest('.tree-row')`;
const treeMenu = async (path) => {
  await hover(await mark(`${treeRow(path)}?.querySelector('.tree-open')`, "tree-row"));
  await tap(await mark(`${treeRow(path)}?.querySelector('.tree-more')`, "tree-more"));
  if (!(await until(() => present(".menu .menu-item"), 4))) throw new Error(`the menu of ${path} did not open`);
};

// filesScreens draws the Project tab of the files rail on the repository, with git's marks, a folder opened, a row's
// menu, Send to agent (nothing sent) and the light theme's icons — the rail is a desk's — then, on every pass, the file
// viewer's changed lines with the old ones shown in place, opened from the tree at a desk and from the Changes lens on a phone
async function filesScreens() {
  await onAgents();
  if (!pass.phone) {
    await screen("files-tree", {
      open: async () => {
        await palette("lens:chat");
        await tap(`${RAIL} .footprint-tabs button:last-child`);
        if (!(await until(() => evaluate(`!!(${treeRow("src")})`), 10))) throw new Error("the Project tab lists no src folder");
        check(await evaluate(`!!${treeRow("src")}?.querySelector('.tree-dot.on')`), "src, holding changes, wears no dot");
        check(await evaluate(`!!${treeRow("old.txt")}?.classList.contains('is-gone')`), "the deleted old.txt is not listed as gone");
        check(await evaluate(`!!${treeRow("build")}?.classList.contains('is-ignored')`), "the ignored build folder is not dimmed");
        check(await evaluate(`!!${treeRow("latest.md")}?.querySelector('.tree-link')`), "the link latest.md wears no link mark");
        check((await count(`${RAIL} .file-icon[data-icon="folder-src"]`)) === 1, "src wears no src folder icon");
      },
      expect: [`${RAIL} .tree-row.tone-changed`],
      close: async () => {},
    });
    await screen("files-tree-folder", {
      open: async () => {
        await tap(await mark(`${treeRow("src")}?.querySelector('.tree-open')`, "tree-src"));
        if (!(await until(() => evaluate(`!!(${treeRow("src/app.go")})`), 6))) throw new Error("src did not open");
        check(await evaluate(`!!${treeRow("src/app.go")}?.classList.contains('tone-changed') && !!${treeRow("src/new.ts")}?.classList.contains('tone-added')`), "src's files do not wear their marks");
      },
      expect: [`${RAIL} .tree-group.nested .tree-row`],
      close: async () => {},
    });
    await screen("files-tree-menu", {
      open: async () => {
        await treeMenu("src/app.go");
        check((await count(".menu .menu-item")) === 6, `the row's menu has ${await count(".menu .menu-item")} rows, not 6`);
      },
      expect: [".menu"],
    });
    await screen("files-tree-send", {
      open: async () => {
        await treeMenu("src/app.go");
        await tap(await mark(`[...document.querySelectorAll(".menu .menu-item")].pop()`, "send-row"));
        if (!(await until(() => present(".modal .send-about"), 4))) throw new Error("Send to agent did not open its dialog");
        check(await evaluate(`document.querySelector(".modal .send-about")?.textContent.endsWith(${JSON.stringify(`${appDir}/src/app.go`)})`), "the dialog does not name the file's path");
      },
      expect: [".modal textarea", ".modal .send-about"],
    });
    await screen("files-tree-light", {
      open: async () => {
        await palette("theme-light");
        if (!(await until(() => evaluate(`document.documentElement.dataset.theme === "light"`), 4))) throw new Error("the page did not turn light");
        if (!(await until(async () => (await count(`${RAIL} .file-icon[data-icon="toml_light"]`)) === 1, 4))) failed("app.toml does not wear the light theme's icon");
      },
      expect: [`${RAIL} .tree-row`],
      close: async () => {
        await palette("theme-dark");
        if (!(await until(() => evaluate(`document.documentElement.dataset.theme === "dark"`), 4))) failed("the page did not turn dark again");
      },
    });
  }
  await screen("file-lines", {
    open: async () => {
      if (pass.phone) {
        await palette("lens:changes");
        await until(async () => (await count(`${LENS} .review-file`)) > 0, 8);
        await tap(await mark(`${fileHead("src/app.go")}?.parentElement.querySelector('.review-open')`, "open-file"));
      } else await tap(await mark(`${treeRow("src/app.go")}?.querySelector('.tree-open')`, "tree-file"));
      if (!(await until(() => present(".fv .fv-gutter .fv-line.is-changed"), 6))) throw new Error("the file viewer marks no changed line");
      await tap(".fv .fv-toggle");
      if (!(await until(() => evaluate(`[...document.querySelectorAll(".fv .fv-line.is-old")].some((l) => l.textContent.includes('"hello"'))`), 4))) failed("the old line does not show in place");
    },
    expect: [".fv .fv-line.is-old"],
    close: async () => {
      await key("Escape", "Escape");
      if (!(await until(async () => !(await present(".fv")), 3))) failed("Esc did not close the file viewer");
      if (!pass.phone) await tap(`${RAIL} .footprint-tabs button:first-child`);
      await palette("lens:terminal");
    },
  });
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
    await tap(await mark(`document.querySelector('${LENS} .review-head .review-look-again')`, "look-again"));
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

// ---- the Git lens on the repository's real history ----

const GIT = ".lens-pane:not([hidden]) .git-lens";

// menuOn opens the Git lens's menu for what is under a selector: a right-click at a desk, a tap on a label on a phone
async function menuOn(selector) {
  const at = await centre(selector);
  if (!at) throw new Error(`nothing to open a menu on at ${selector}`);
  if (pass.phone) await press(at);
  else {
    await send("Input.dispatchMouseEvent", { type: "mouseMoved", x: at[0], y: at[1] });
    await send("Input.dispatchMouseEvent", { type: "mousePressed", x: at[0], y: at[1], button: "right", clickCount: 1 });
    await send("Input.dispatchMouseEvent", { type: "mouseReleased", x: at[0], y: at[1], button: "right", clickCount: 1 });
  }
  if (!(await until(() => present(".menu .menu-item"), 4))) throw new Error(`no menu opened on ${selector}`);
}

// gitScreens draws the Git lens on every pass: the graph with its uncommitted row, labels and HEAD, a commit's details,
// a file's diff with its words marked, a branch's menu and its delete asked and cancelled — nothing is written
async function gitScreens() {
  await onAgents();
  const branchesBefore = git(appDir, "branch", "--format=%(refname:short)").toString();
  await screen("git-lens", {
    open: async () => {
      await palette("lens:git");
      if (!(await until(async () => (await count(`${GIT} .git-row[data-index]`)) === 3, 10))) failed(`the graph draws ${await count(`${GIT} .git-row[data-index]`)} rows, not the uncommitted one and two commits`);
      check(await present(`${GIT} .git-row.is-uncommitted`), "no Uncommitted changes row over HEAD");
      check(await present(`${GIT} .git-row.is-head .git-label.is-current`), "HEAD's row does not carry the checked-out branch, filled");
      check(await present(`${GIT} .git-label.is-tag`), "the tag v0.1.0 has no label");
      check(/main/.test(await evaluate(`document.querySelector('${GIT} .git-where')?.textContent ?? ""`)), "the head's chip does not say main");
      check(await evaluate(`[...document.querySelectorAll('${GIT} .git-row[data-index]')].every((r) => Math.round(r.getBoundingClientRect().height) === 24)`), "a row of the graph is not 24 px high");
      // a phone widens its page to what overflows it, so the page's own width says whether the four lenses fit
      check(await evaluate(`innerWidth === ${pass.width} && document.querySelector('.lens-switch').getBoundingClientRect().right <= innerWidth + 1`), "the header's four lenses run past the screen");
    },
    expect: [`${GIT} .git-head`, `${GIT} .git-lanes`],
    close: async () => {},
  });
  await screen("git-details", {
    open: async () => {
      await tap(await mark(`[...document.querySelectorAll('${GIT} .git-row[data-index]')].find((r) => r.textContent.includes("docs: name the app"))?.querySelector('.git-subject')`, "git-commit"));
      if (!(await until(() => present(`${GIT} .git-details .git-file`), 6))) throw new Error("the commit's files did not come");
      check(/README\.md/.test(await evaluate(`document.querySelector('${GIT} .git-files')?.textContent ?? ""`)), "the commit's files do not name README.md");
    },
    expect: [`${GIT} .git-meta`, `${GIT} .git-files`],
    close: async () => {},
  });
  await screen("git-diff", {
    open: async () => {
      await tap(`${GIT} .git-file`);
      if (!(await until(() => present(`${GIT} .git-diff .fv-line.add`), 6))) throw new Error("the file's diff did not come");
      check((await count(`${GIT} .git-diff .fv-mark`)) > 0, "the changed words are not marked");
      check((await count(`${GIT} .git-diff .fv-line.del`)) === 1, "the removed line is not drawn");
    },
    expect: [`${GIT} .git-diff`],
    close: async () => {
      await tap(`${GIT} .git-details-head .icon-btn`);
      await sleep(200);
    },
  });
  await screen("git-branch-menu", {
    open: async () => {
      await menuOn(`${GIT} .git-label-part[data-ref-name="feature-x"]`);
      const rows = await evaluate(`[...document.querySelectorAll('.menu .menu-item')].map((i) => i.textContent)`);
      check(rows.length >= 5, `the branch's menu has ${rows.length} rows`);
      check(!rows.some((r) => /push|pull|fetch/i.test(r)), "the branch's menu offers what reaches a remote");
    },
    expect: [".menu .menu-item.danger"],
    close: async () => {},
  });
  await screen("git-delete-branch", {
    open: async () => {
      await evaluate(`document.querySelector('.menu .menu-item.danger')?.click()`);
      if (!(await until(() => present(".modal.git-dialog"), 4))) throw new Error("Delete branch… asked nothing first");
      check(await present(".modal.git-dialog button[type=submit].danger"), "the delete's button is not red");
      check(/feature-x/.test(await evaluate(`document.querySelector('.git-dialog-detail')?.textContent ?? ""`)), "the dialog does not name the branch");
    },
    expect: [".modal.git-dialog .modal-actions"],
    close: async () => {
      await tap(".modal.git-dialog .modal-actions .btn:not(.danger)");
      if (!(await until(async () => !(await present(".modal.git-dialog")), 3))) failed("Cancel did not close the dialog");
      check(git(appDir, "branch", "--format=%(refname:short)").toString() === branchesBefore, "a cancelled delete changed the branches");
    },
  });
  await palette("lens:terminal");
}

async function runPass() {
  await send("Emulation.setDeviceMetricsOverride", { width: pass.width, height: pass.height, deviceScaleFactor: 1, mobile: pass.phone });
  await send("Emulation.setTouchEmulationEnabled", { enabled: pass.phone, maxTouchPoints: pass.phone ? 5 : 0 });
  fakeOpinionState = null;
  const prefs = { lang: pass.lang, "tasks.sel": { kind: "project", id: inbox }, "tasks.view": "board", ...pass.look };
  await evaluate(`(() => { localStorage.clear(); for (const [k, v] of Object.entries(${JSON.stringify(prefs)})) localStorage.setItem("remuda." + k, JSON.stringify(v)); })()`);
  await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=terminal` });
  if (!(await until(() => present(".pane-area .tab"), 15))) failed("the Agents page did not come up");
  const spoken = await evaluate("document.documentElement.lang");
  if (spoken !== pass.lang) failed(`the page speaks ${spoken}, not ${pass.lang}`);
  const worn = await evaluate(`document.documentElement.dataset.flavor + " " + document.documentElement.dataset.theme`);
  const asked = `${pass.look["theme.dark"] ?? pass.look["theme.light"]} ${pass.look.theme}`;
  if (worn !== asked) failed(`the page wears ${worn}, not ${asked}`);
  if ((await evaluate(`matchMedia("(pointer: coarse)").matches`)) !== pass.phone) failed(`the browser does not take itself for a ${pass.phone ? "phone" : "desk"}: (pointer: coarse) says otherwise`);
  where = pass.name;

  // the card Claude's plan limit holds is offered on Codex in the corner, once: dismissed, it stays gone on this device
  await screen("plan-offer-toast", {
    open: async () => {
      if (!(await until(() => present('[data-toast^="plan-offer-"] .btn.primary'), 10))) failed("no toast offers the held card on the other vendor");
      check(await evaluate(`/Codex/.test(document.querySelector('[data-toast^="plan-offer-"] .btn.primary')?.textContent ?? "")`), "the offer's button does not name Codex");
    },
    expect: ['[data-toast^="plan-offer-"]'],
    close: async () => {
      await tap('[data-toast^="plan-offer-"] .icon-btn');
      if (!(await until(async () => !(await present('[data-toast^="plan-offer-"]')), 3))) failed("the offer's toast stays once dismissed");
    },
  });
  await screen("sidebar", {
    open: async () => {
      await sidebarOpen();
      await conciergeHidden(".sidebar");
    },
    expect: [".ws-folder-head", ".ws-item", ".machine-head", ".ws-item .collide-mark.is-lines", ".needs-you .inbox-row", ".ws-item .ws-agent-line", ".sidebar-foot"],
    close: async () => {},
  });
  // the sidebar's agents after Orca: "Needs you" with what the Codex asks on its screen, a line per agent under every
  // card (two under the repository's), the subagents at work on their tree under the Claude Code, the main checkout's
  // primary tag and the selected card's distance from its upstream, the foot's link to the one waiting
  await screen("sidebar-agents", {
    open: async () => {
      await sidebarOpen();
      if (!(await until(() => evaluate(`[...document.querySelectorAll(".needs-you .inbox-ask")].some((el) => el.textContent.includes("go build ./..."))`), 15))) failed("the inbox does not say what the Codex asks");
      const lines = await evaluate(`${firstCardOf("app")}?.querySelectorAll(".ws-agent-line").length ?? 0`);
      check(lines === 2, `the repository's card has ${lines} agent lines, not its two agents'`);
      check(await evaluate(`!!${firstCardOf("app")}?.querySelector(".ws-agent-line.shape-asking .needs-pill")`), "the asking agent's line wears no Needs you pill");
      check(await evaluate(`${firstCardOf("app")}?.querySelectorAll(".sub-tree .sub-line").length === 3`), "the Claude Code's subagents are not on their tree, two named and one more");
      check(await evaluate(`!!${firstCardOf("app")}?.querySelector(".primary-tag")`), "the main checkout beside its worktree wears no primary tag");
      check(await evaluate(`${firstCardOf("app")}?.querySelector(".ws-sync")?.textContent === "↑2 ↓1"`), "the selected card does not say how far it stands from its upstream");
      check(await evaluate(`!!document.querySelector(".sidebar-foot .sidebar-foot-needs")`), "the foot does not link to the agent waiting");
    },
    expect: [".needs-you .inbox-row", ".sub-tree .sub-line", ".ws-agent-line.shape-working", ".ws-agent-line.shape-done"],
    close: async () => {},
  });
  // an agent pinned from its line's pin (under the pointer at a desk, always there on a phone) stands in the pinned
  // group; unpinned again, the group goes
  await screen("sidebar-pinned", {
    open: async () => {
      await sidebarOpen();
      const line = await mark(`${firstCardOf("app")}?.querySelector(".ws-agent-line.shape-working")`, "agent-line");
      await hover(line);
      await sleep(150);
      await tap(`${line} .agent-pin`);
      if (!(await until(() => present(".sidebar-group.pinned .ws-agent-line"), 4))) failed("the pinned agent is not in the pinned group");
    },
    expect: [".sidebar-group.pinned .ws-agent-line .agent-line-where"],
    close: async () => {
      await hover(".sidebar-group.pinned .ws-agent-line");
      await sleep(150);
      await tap(".sidebar-group.pinned .ws-agent-line .agent-pin");
      if (!(await until(async () => !(await present(".sidebar-group.pinned")), 3))) failed("the pinned group stays once its agent is unpinned");
    },
  });
  // the order by priority: sections for the ones that need you, the finished, the working and the idle, with counts
  await screen("sidebar-priority", {
    open: async () => {
      await sidebarOpen();
      await setPref("radar", { order: "priority", rowLabel: "title", freshMinutes: 15, staleMinutes: 120, shells: false });
      if (!(await until(() => present(".ws-section.is-needs .ws-section-head .badge"), 4))) failed("the order by priority draws no Needs you section");
      check(await evaluate(`!!document.querySelector(".ws-section.is-done")`), "the order by priority has no Done section for the finished worktree");
      check(await evaluate(`!!document.querySelector(".ws-section.is-needs .ws-agent-line .agent-shape.is-asking-square")`), "under the Needs you heading a question is not the ! square");
    },
    expect: [".ws-section-head", ".ws-section .ws-item"],
    close: async () => {
      await setPref("radar", { order: "active", rowLabel: "title", freshMinutes: 15, staleMinutes: 120, shells: false });
      await until(() => present(".ws-folder-head"), 3);
    },
  });
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
      check(await evaluate(`[...document.querySelectorAll(".health-pop dd")].some((dd) => dd.textContent.startsWith("64 % · "))`), "the vitals say nothing of the battery");
      check(await evaluate(`[...document.querySelectorAll(".health-pop .health-limit.is-amber")].some((row) => row.textContent.includes("≈"))`), "the vitals do not say Claude's 5 hours run out early at this pace");
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
  // a Windows PC's pane: herdr has no terminal attach there, so the lens is the pane's screen read again and typed into —
  // the notes shell played as a Windows machine's by the dressed roster, its screen and its typing the throwaway herdr's own
  await screen("screen-terminal", {
    open: async () => {
      windowsLocal = true;
      await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneN)}&lens=terminal` });
      if (!(await until(() => present(".lens-pane:not([hidden]) .screen-term .screen-line"), 15))) throw new Error("the screen terminal did not come up");
      check(!(await present(".xterm")), "a Windows machine's pane attached a live terminal");
      const word = `screen-${pass.name}`;
      await tap(".lens-pane:not([hidden]) .screen-term-screen");
      await send("Input.insertText", { text: `echo ${word}` });
      await key("Enter", "Enter");
      const typed = () => evaluate(`[...document.querySelectorAll(".lens-pane:not([hidden]) .screen-term .screen-line")].some((line) => line.textContent.trim() === ${JSON.stringify(word)})`);
      if (!(await until(typed, 10))) failed("what was typed into the screen terminal did not come back on its screen");
    },
    expect: [".lens-pane:not([hidden]) .screen-term-note"],
    close: async () => {
      windowsLocal = false;
      await send("Page.navigate", { url: `${base}/?machine=local&pane=${encodeURIComponent(paneA1)}&lens=terminal` });
      if (!(await until(() => present(".lens-pane:not([hidden]) .xterm-screen"), 15))) failed("the live terminal did not come back");
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
  // an answer whose changes the cockpit kept offers Undo this turn on its foot, which opens the undo on that very turn
  await screen("chat-undo", {
    open: async () => {
      undoChat = true;
      await palette("lens:chat");
      if (!(await until(() => present(".lens-pane:not([hidden]) .turn-undo"), 20))) throw new Error("the answer's foot offers no Undo this turn");
      await tap(".lens-pane:not([hidden]) .turn-undo");
    },
    expect: [".modal .undo-files"],
    close: async () => {
      undoChat = false;
      await closeAll();
      await palette("lens:terminal");
    },
  });
  // the chat's own: an answer's calls as one card, a message sent while the agent worked in its turn, its last call
  // still running and the spinner's progress over the composer — then the prompts list beside the log (a sheet on a
  // phone) with a card opened in place
  await screen("chat-working", {
    open: async () => {
      promptsChat = true;
      await palette("lens:chat");
      if (!(await until(() => present(".lens-pane:not([hidden]) .part-sent"), 20))) throw new Error("the message sent while the agent worked is not in its turn");
      await evaluate(`(() => { const log = document.querySelector(".lens-pane:not([hidden]) .chat-log"); if (log) log.scrollTop = log.scrollHeight; })()`);
      if (!(await until(() => evaluate(`!!document.querySelector(".lens-pane:not([hidden]) .chat-progress")?.textContent.includes("Refactoring")`), 10))) failed("the progress line does not say what the spinner says");
    },
    expect: [".lens-pane:not([hidden]) .tool-card .tool-spin", ".lens-pane:not([hidden]) .part-sent .part-sent-mark", ".lens-pane:not([hidden]) .chat-progress"],
    close: async () => {},
  });
  await screen("chat-prompts", {
    open: async () => {
      promptsChat = true;
      if (!(await present(".lens-pane:not([hidden]) .chat-log"))) await palette("lens:chat"); // the screen before left it on
      if (!(await until(() => present(".lens-pane:not([hidden]) .tool-card"), 20))) throw new Error("the answer's calls are not one card");
      await palette("prompts");
      if (!(await until(() => present(".prompts .prompt"), 10))) throw new Error("the prompts list did not open");
      await tap(".prompts .prompt:nth-child(2) .prompt-head");
      if (!(await until(() => present(".prompts .prompt.is-open .prompt-reply"), 5))) failed("a card of the prompts list did not open in place");
    },
    expect: [".lens-pane:not([hidden]) .tool-card .tool-mark.is-failed", ".prompts .prompt.is-live .prompt-working", ".prompts .prompt.is-open .prompt-reply"],
    close: async () => {
      if (await present(".prompts")) await palette("prompts");
      promptsChat = false;
      await palette("lens:terminal");
    },
  });
  if (!pass.phone)
    await screen("split-view", {
      open: async () => palette("split"),
      expect: [".split-chat", ".split-terminal"],
      close: async () => palette("split"),
    });
  await changesScreens();
  await gitScreens();
  await filesScreens();
  await screen("previews", {
    open: async () => {
      await onAgents();
      await palette("previews");
      if (!(await until(() => evaluate(`[...document.querySelectorAll(".modal .preview-row")].some((r) => r.textContent.includes("${devPort}"))`), 20))) failed(`the previews dialog does not list the dev server's port ${devPort}`);
    },
    expect: [".modal .preview-row"],
  });
  await screen("triggers", {
    open: async () => {
      await onAgents();
      await palette("triggers-here");
      if (!(await until(async () => (await count(".modal .trigger-row")) === 2, 15))) failed(`the triggers dialog lists ${await count(".modal .trigger-row")} rules for the folder, not 2`);
      await tap(".modal .trigger-row .btn");
      if (!(await until(() => present(".modal .trigger-form"), 5))) failed("a trigger's form did not open");
    },
    expect: [".modal .trigger-form", ".modal .trigger-try"],
  });
  await sceneScreens();
  await prScreens({ screen, palette, tap, present, until, evaluate, count, failed, check, onAgents, sleep });
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

  await screen("wall-chat", { open: async () => palette("mode:wall"), expect: [".wall-bar", ".tile", ".tile .collide-mark"] });
  await screen("wall-screen", { open: async () => palette("wall-view"), expect: [".wall-bar", ".tile"] });
  await screen("wall-zoo", {
    open: async () => {
      await palette("wall-zoo");
      await conciergeHidden(".wall");
    },
    expect: [".zoo-grid .zoo-creature", ".zoo-bolts path.is-lines", ".zoo-bolt"],
  });
  // a creature asked about its day says it in its bubble, in its own voice
  await screen("zoo-tell", {
    open: async () => {
      await evaluate(FAKE_VOICE);
      await tap(".zoo-grid .zoo-creature .zoo-tell");
      const telling = `(() => { const bubble = document.querySelector('.zoo-bubble.is-telling'); return !!bubble && bubble.textContent.trim().length > 3; })()`;
      if (!(await until(() => evaluate(telling), 6))) failed("a creature asked about its day shows nothing in its bubble");
      check(await evaluate(`(window.__fakeVoice?.said ?? []).some((text) => /^(Today I|Сегодня я)/.test(text))`), "the creature's voice was handed nothing about its day");
    },
    expect: [".zoo-bubble.is-telling"],
    close: async () => evaluate(REAL_VOICE),
  });
  if (!pass.phone) await screen("wall-office", { open: async () => palette("wall-office"), expect: [".wall-office"], close: async () => palette("wall-office") });

  await screen("tasks-board", { open: async () => palette("mode:tasks"), expect: [".board", ".board-col-note"] });
  await screen("project-settings", { open: async () => tap(".tasks-settings-btn"), expect: [".project-settings .ps-queue"], settle: 500, close: async () => {} });
  await screen("project-settings-agents", {
    open: async () => evaluate(`document.querySelector('.project-settings .ps-queue')?.scrollIntoView({ block: "center" })`),
    expect: [".project-settings .ps-queue input[type=checkbox]:checked", ".project-settings .retired-notice"],
    close: async () => {},
  });
  // the Inbox's queue may use any machine that has room: the choice, and the words of how it picks one
  await screen("project-settings-fleet", {
    open: async () => evaluate(`document.querySelector('.project-settings .ps-where')?.scrollIntoView({ block: "center" })`),
    expect: [".project-settings .ps-where .sel", ".project-settings .ps-where + .modal-hint"],
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
  await screen("review", { open: async () => palette("mode:review"), expect: [".queue-head", ".queue-item .risk-chip.is-high", ".queue-item .collide-mark"] });
  await screen("code", { open: async () => palette("mode:code"), expect: [".code-mode"] });

  for (const panel of ["standup", "usage", "schedules", "activity", "alerts", "arenas"]) await screen(`panel-${panel}`, { open: async () => palette(panel), expect: [".modal"], settle: 600 });
  // the day read aloud: Listen in the standup, the sentence being said under its agent, skip and pause, the lock screen
  await screen("standup-listen", {
    open: async () => {
      await palette("standup");
      if (!(await until(() => present(".standup-item"), 8))) return failed("the standup lists none of the agents it was given");
      await evaluate(FAKE_VOICE);
      await tap(".listen-start");
      if (!(await until(async () => (await sentence()).length > 3, 4))) return failed("Listen shows no sentence being said");
      const first = await sentence();
      check((await count(".listen-bar .listen-controls .icon-btn")) === 3 && (await present(".listen-bar .listen-stop")), "the Listen bar lacks its play, back, skip or stop");
      await tap(".listen-bar .listen-skip");
      if (!(await until(async () => (await sentence()) !== first && (await sentence()).length > 3, 4))) failed("skipping to the next agent moved nothing");
      check(await present(".standup-item.is-reading"), "the agent being told of is not marked in the report");
      check(await evaluate(`(() => { const title = (navigator.mediaSession?.metadata?.title ?? "").replace(/…$/, ""); return title.length > 3 && (document.querySelector('.listen-sentence')?.textContent ?? "").startsWith(title); })()`), "the lock screen does not name the sentence being said");
      await tap(".listen-bar .listen-toggle");
      check(await present(".listen-bar.is-paused"), "pause does not pause");
    },
    expect: [".modal .listen-bar .listen-sentence"],
    close: async () => {
      await evaluate(REAL_VOICE);
      await closeAll();
    },
  });
  await screen("listen-day", {
    open: async () => {
      await evaluate(FAKE_VOICE);
      await palette("listen-day");
      if (!(await until(async () => (await sentence()).length > 3, 8))) failed("Listen to the day opens the standup reading nothing");
    },
    expect: [".modal .listen-bar"],
    close: async () => {
      await evaluate(REAL_VOICE);
      await closeAll();
    },
  });

  // the concierge: its conversation in its place (its workspace in no sidebar, wall or palette row: conciergeHidden);
  // a new conversation offers the questions to start with
  await screen("concierge", {
    open: async () => palette("concierge"),
    expect: [".modal.concierge", ".concierge-chat .turn-assistant", ".concierge-chat .composer textarea"],
    settle: 600,
  });
  await screen("concierge-new", {
    open: async () => {
      await palette("concierge");
      if (!(await until(() => present(".concierge-chat .turn-assistant"), 6))) throw new Error("the concierge's conversation did not show");
      await tap(".concierge-bar .btn.sm");
    },
    expect: [".concierge-hint"],
    settle: 400,
  });

  // the plan limits seen coming: the 5 hours' run-out in amber, the week lasting, the held card's offer — and its run
  await screen("usage-plan-limits", {
    open: async () => {
      await palette("usage");
      if (!(await until(() => present(".plan-limits .limit-offer .btn.primary"), 6))) failed("the Usage panel does not offer the held card on the other vendor");
      check((await count(".plan-limits .limit-forecast")) >= 2, "the plan limits say nothing of when their windows run out");
      check(await present(".plan-limits .limit-forecast.is-amber"), "a window that runs out well before its reset is not told apart");
    },
    expect: [".plan-limits .limit-forecast"],
    close: async () => {
      const before = planOfferRuns;
      await tap(".plan-limits .limit-offer .btn.primary");
      if (!(await until(() => planOfferRuns > before, 4))) failed("the offer's button ran nothing");
      if (!(await until(() => present('[data-toast="notice"]'), 4))) failed("the offer's run says nothing");
      await closeAll();
    },
  });
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
  await screen("fleet-setup", {
    open: async () => {
      await palette("fleet-setup");
      if (!(await until(() => present(".fleet-setup .fleet-table"), 6))) return failed("Fleet setup draws no table");
      check((await count(".fleet-setup thead th")) === 5, "Fleet setup does not lay the four machines out as columns");
      check((await count(".fleet-cell.is-odd")) === 6, `Fleet setup marks ${await count(".fleet-cell.is-odd")} cells off the norm, not the six that differ`);
      check((await count(".fleet-cell.is-split")) === 2, "a file two machines hold in two copies is not marked as having no norm");
      check((await count(".fleet-setup .fleet-fix")) === 2, "Fleet setup does not offer the two updates that bring a machine in line");
      check((await count(".fleet-setup .fleet-hand li")) === 5, `Fleet setup leaves ${await count(".fleet-setup .fleet-hand li")} things to do by hand, not five`);
      check(await evaluate(`[...document.querySelectorAll(".fleet-setup tbody tr:not(.fleet-section)")].every((tr) => tr.querySelector(".is-odd, .is-split"))`), "a row in line shows before Show every fact");
    },
    expect: [".fleet-setup .fleet-table"],
    settle: 500,
  });
  await screen("fleet-setup-every-fact", {
    open: async () => {
      await palette("fleet-setup");
      if (!(await until(() => present(".fleet-setup .fleet-table"), 6))) return failed("Fleet setup draws no table");
      const shown = await count(".fleet-setup tbody tr:not(.fleet-section)");
      await tap(".fleet-setup .modal-actions .btn[aria-pressed]");
      if (!(await until(async () => (await count(".fleet-setup tbody tr:not(.fleet-section)")) === shown + 3, 4))) failed("Show every fact does not add the three rows in line");
    },
    expect: [".fleet-setup .fleet-table"],
  });
  await screen("fleet-setup-bring-in-line", {
    open: async () => {
      await palette("fleet-setup");
      if (!(await until(() => present(".fleet-setup .fleet-fix .btn:not(:disabled)"), 6))) return failed("Fleet setup offers no update to run here");
      check((await count(".fleet-setup .fleet-fix .btn:disabled")) === 1, "an update on a machine the roster does not list can be run");
      await tap(".fleet-setup .fleet-fix .btn:not(:disabled)");
      if (!(await until(() => evaluate(`!!document.querySelector(".modal.updates:not(.fleet-setup)") && !document.querySelector(".fleet-setup")`), 6))) failed("Run… does not open the machine's Updates on it");
    },
    expect: [".modal.updates"],
  });
  await screen("settings", { open: async () => palette("settings"), expect: [".settings-dialog .settings-section"], close: async () => {} });
  await screen("settings-themes", {
    open: async () => {
      await evaluate(`document.querySelector('.settings-dialog .settings-themes')?.scrollIntoView({ block: "start" })`);
      check((await count(".settings-dialog .theme-cards[role=radiogroup] .theme-card")) === 20, "Settings › Look does not offer ten dark and ten light themes");
      const outside = await evaluate(`(() => { const box = document.querySelector(".settings-dialog").getBoundingClientRect(); return [...document.querySelectorAll(".theme-card")].filter((card) => { const r = card.getBoundingClientRect(); return r.width < 60 || r.left < box.left - 1 || r.right > box.right + 1; }).map((card) => card.dataset.flavor); })()`);
      check(outside.length === 0, `theme cards that do not fit the dialog: ${outside.join(", ")}`);
      const picked = pass.look["theme.dark"] ?? pass.look["theme.light"];
      check(await present(`.theme-card.on[data-flavor="${picked}"][aria-checked="true"]`), `the ${picked} card is not the one picked`);
      const other = pass.look.theme === "dark" ? "dracula" : "github-light";
      await tap(`.theme-card[data-flavor="${other}"]`);
      if (!(await until(async () => (await evaluate("document.documentElement.dataset.flavor")) === other, 3))) failed(`a tap on the ${other} card did not dress the page in it`);
      await tap(`.theme-card[data-flavor="${picked}"]`);
      if (!(await until(async () => (await evaluate("document.documentElement.dataset.flavor")) === picked, 3))) failed(`a tap on the ${picked} card did not dress the page back`);
    },
    expect: [".settings-dialog .theme-card.on"],
    close: async () => {},
  });
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
      if (await until(() => present(".palette .palette-row"), 4)) await conciergeHidden(".palette");
    },
    expect: [".palette .palette-row"],
  });
  await screen("keys-sheet", { open: async () => palette("keys"), expect: [".modal"] });

  await onAgents();
  for (const [name, id, notice] of [
    ["new-workspace", "workspace:local", ".modal .retired-notice"],
    ["import-issue", "import-issue"],
    ["import-jira", "import-jira"],
    ["arena", "arena-new", ".arena-new .retired-notice"],
    ["teleport", "teleport"],
    ["reopen", "reopen"],
    ["recipes", "recipes"],
    ["budget", "budgets"],
    ["broadcast", "broadcast"],
  ])
    await screen(`dialog-${name}`, { open: async () => palette(id), expect: [".modal", ...(notice ? [notice] : [])], settle: 700 });
  // the dressed agent's conversation recapped and read aloud a line at a time
  await screen("recap-listen", {
    open: async () => {
      await evaluate(FAKE_VOICE);
      await palette("recap");
      if (!(await until(() => present(".modal .listen-start:not(:disabled)"), 6))) return failed("the recap's Listen never comes ready");
      await tap(".modal .listen-start");
      if (!(await until(async () => (await sentence()).length > 3, 4))) return failed("the recap's Listen shows no sentence being said");
      const first = await sentence();
      await tap(".listen-bar .listen-skip");
      if (!(await until(async () => (await sentence()) !== first, 4))) failed("skipping a line of the recap moved nothing");
    },
    expect: [".modal .listen-bar .listen-sentence"],
    close: async () => {
      await evaluate(REAL_VOICE);
      await closeAll();
    },
  });
  await screen("timelapse", {
    open: async () => {
      await palette("timelapse");
      if (!(await until(async () => (await count(".timelapse-screen .screen-line")) >= 6, 6))) failed("the timelapse player draws no frame");
      const clock = () => evaluate(`document.querySelector('.timelapse-clock')?.textContent ?? ""`);
      const before = await clock();
      await sleep(900);
      check((await clock()) !== before, `the timelapse does not play: its clock stays at ${before}`);
      check((await count(".timelapse-pick .sel")) === 1 && (await count(".timelapse-speeds .btn")) === 4, "the player has no recording picker or not its four speeds");
      await tap(".timelapse-bar .icon-btn");
      await tap(".timelapse-speeds .btn:last-child");
      check(await present(".timelapse-speeds .btn.on:last-child"), "By frame is not picked");
    },
    expect: [".modal .timelapse-screen", ".timelapse-scrub", ".timelapse-export"],
    settle: 500,
  });
  // the find over the recorded screens: the text marked where it first stood, then a step to the earlier recording
  await screen("timelapse-find", {
    open: async () => {
      await palette("timelapse");
      if (!(await until(async () => (await count(".timelapse-screen .screen-line")) >= 6, 6))) failed("the timelapse player draws no frame");
      await tap(".timelapse-find input");
      await send("Input.insertText", { text: "screen3" });
      if (!(await until(() => present(".timelapse-screen .screen-line.is-current .m"), 6))) failed("the player does not mark the row the text first stood in");
      const at = await evaluate(`document.querySelector('.timelapse-clock')?.textContent ?? ""`);
      check(at.startsWith("16:00"), `the player is not at the frame the text appeared in: ${at}`);
      await key("Enter", "Enter");
      if (!(await until(async () => (await evaluate(`document.querySelector('.timelapse-find .find-count')?.textContent ?? ""`)).startsWith("2"), 6))) failed("Enter in the find does not step to the next place");
    },
    expect: [".timelapse-find input", ".timelapse-screen .screen-line.is-current"],
    settle: 500,
  });
  // the search panel's Screens part, and a hit of it opening the player at its frame
  await screen("search-screens", {
    open: async () => {
      await palette("search");
      if (!(await until(() => present(".search-head input"), 4))) throw new Error("the search did not open");
      await send("Input.insertText", { text: "screen3" });
    },
    expect: [".search-screens .search-screen-hit .search-screen-row.is-match mark", ".search-screens .search-part-head", ".search-screens .notice"],
    settle: 600,
  });
  await screen("search-screen-open", {
    open: async () => {
      await palette("search");
      if (!(await until(() => present(".search-head input"), 4))) throw new Error("the search did not open");
      await send("Input.insertText", { text: "screen3" });
      if (!(await until(() => present(".search-screen-hit"), 6))) throw new Error("no screen hit to open");
      await tap(".search-screen-hit");
    },
    expect: [".modal .timelapse-screen .screen-line.is-current .m", ".timelapse-find input"],
    settle: 500,
  });
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
      { urlPattern: "*/api/concierge*", requestStage: "Request" },
      { urlPattern: "*/api/scorecard*", requestStage: "Request" },
      { urlPattern: "*/api/scenes/*", requestStage: "Request" },
      { urlPattern: "*/api/scene-runs/*", requestStage: "Request" },
      { urlPattern: "*/api/review/secrets*", requestStage: "Request" },
      { urlPattern: "*/api/review?dir=*", requestStage: "Response" },
      { urlPattern: "*/api/review/conflict*", requestStage: "Request" },
      { urlPattern: "*/api/timelapse/recordings*", requestStage: "Request" },
      { urlPattern: "*/api/standup*", requestStage: "Request" },
      { urlPattern: "*/api/pr/*", requestStage: "Request" },
      { urlPattern: "*/api/recap*", requestStage: "Request" },
      { urlPattern: "*/api/timelapse/search*", requestStage: "Request" },
      { urlPattern: "*/api/fleet-setup*", requestStage: "Request" },
      { urlPattern: "*/api/undo*", requestStage: "Request" },
      { urlPattern: "*/api/limits/offers*", requestStage: "Request" },
      { urlPattern: "*/api/question*", requestStage: "Request" },
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
