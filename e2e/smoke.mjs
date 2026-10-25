// The end-to-end smoke scripts/e2e.sh runs: headless Chrome over the DevTools protocol against a remuda of
// its own on a throwaway herdr. Sign in, a scratch workspace through the API, the terminal typed into and
// read back, the chat, the Wall, Review, Activity and the palette, then the workspace closed. One line per
// step; the first failure ends the run with a non-zero exit. Node's own fetch and WebSocket, no dependency.
import { writeFileSync } from "node:fs";

const base = process.env.E2E_URL;
const cdp = process.env.E2E_CDP;
const token = process.env.E2E_TOKEN;
const out = process.env.E2E_OUT;
const wsDir = process.env.E2E_WS_DIR;
for (const [name, value] of Object.entries({ E2E_URL: base, E2E_CDP: cdp, E2E_TOKEN: token, E2E_OUT: out, E2E_WS_DIR: wsDir })) {
  if (!value) throw new Error(`${name} is not set`);
}

const step = (name, detail = "") => console.log(`e2e: ${name}${detail ? ` — ${detail}` : ""}`);
const fail = (what) => {
  throw new Error(what);
};
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// until polls a check every 500 ms and gives up after `seconds`.
async function until(what, check, seconds = 20) {
  for (let i = 0; i < seconds * 2; i++) {
    const value = await check();
    if (value) return value;
    await sleep(500);
  }
  fail(`${what}: not within ${seconds} s`);
}

async function api(method, path, body) {
  const response = await fetch(base + path, {
    method,
    headers: { Authorization: `Bearer ${token}`, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) fail(`${method} ${path}: ${response.status} ${(await response.text()).trim()}`);
  return response.headers.get("content-type")?.includes("json") ? response.json() : response.text();
}

// The browser: one page over the DevTools protocol.
const target = await (await fetch(`${cdp}/json/new?about:blank`, { method: "PUT" })).json();
const socket = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  socket.onopen = resolve;
  socket.onerror = reject;
});
let nextId = 0;
const waiting = new Map();
const consoleErrors = [];
socket.onmessage = (message) => {
  const msg = JSON.parse(message.data);
  if (msg.id && waiting.has(msg.id)) {
    waiting.get(msg.id)(msg);
    waiting.delete(msg.id);
  }
  if (msg.method === "Runtime.exceptionThrown") consoleErrors.push(msg.params.exceptionDetails?.exception?.description ?? "exception");
  if (msg.method === "Runtime.consoleAPICalled" && msg.params.type === "error") consoleErrors.push(msg.params.args.map((arg) => arg.value ?? arg.description).join(" "));
};
const send = (method, params = {}) =>
  new Promise((resolve) => {
    const id = ++nextId;
    waiting.set(id, resolve);
    socket.send(JSON.stringify({ id, method, params }));
  });
const evaluate = async (expression) => {
  const reply = await send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
  if (reply.result?.exceptionDetails) fail(`in the page: ${reply.result.exceptionDetails.text} ${reply.result.exceptionDetails.exception?.description ?? ""}`);
  return reply.result?.result?.value;
};
const shot = async (name) => {
  const reply = await send("Page.captureScreenshot", { format: "png" });
  writeFileSync(`${out}/${name}.png`, Buffer.from(reply.result.data, "base64"));
};
const key = async (keyName, code, modifiers = 0, text) => {
  const virtualKey = keyName === "Enter" ? 13 : keyName === "Escape" ? 27 : keyName.length === 1 ? keyName.toUpperCase().charCodeAt(0) : undefined;
  await send("Input.dispatchKeyEvent", { type: "rawKeyDown", key: keyName, code, modifiers, windowsVirtualKeyCode: virtualKey, text });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: keyName, code, modifiers });
};
const click = (selector) => evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return false; el.click(); return true; })()`);
const clickText = (selector, text) => evaluate(`(() => { const el = [...document.querySelectorAll(${JSON.stringify(selector)})].find((b) => b.innerText.trim() === ${JSON.stringify(text)}); if (!el) return false; el.click(); return true; })()`);
const present = (selector) => evaluate(`!!document.querySelector(${JSON.stringify(selector)})`);
const localSessions = async () => (await api("GET", "/api/roster")).machines.find((machine) => machine.id === "local")?.sessions ?? [];

let workspace = null;
try {
  await send("Page.enable");
  await send("Runtime.enable");
  await send("Emulation.setDeviceMetricsOverride", { width: 1400, height: 900, deviceScaleFactor: 1, mobile: false });

  const health = await (await fetch(`${base}/api/health`)).json();
  step("health answers", `version ${health.version ?? "?"}`);

  await until("the local machine connected", async () => (await api("GET", "/api/roster")).machines?.some((machine) => machine.id === "local" && machine.state === "connected"));
  step("roster has the local machine, connected");

  workspace = await api("POST", "/api/sessions", { machine: "local", dir: wsDir, agent: "shell" });
  if (typeof workspace?.id !== "string" || typeof workspace?.pane !== "string") fail(`create answered ${JSON.stringify(workspace)}`);
  step("scratch workspace created", `${workspace.id}, pane ${workspace.pane}`);
  await until("the pane in the roster", async () => (await localSessions()).some((session) => session.id === workspace.id));

  await send("Page.navigate", { url: `${base}/?token=${encodeURIComponent(token)}&machine=local&pane=${encodeURIComponent(workspace.pane)}&lens=terminal` });
  await until("the page signed in and on the scratch pane", async () => {
    const search = await evaluate("location.search");
    return search && !search.includes("token=") && search.includes(encodeURIComponent(workspace.pane)) && (await present(".topbar"));
  });
  if (!(await evaluate("location.search")).includes(encodeURIComponent(workspace.pane))) fail("the page is not on the scratch pane: nothing is typed");
  step("signed in, the scratch pane selected", await evaluate("location.search"));

  await until("the terminal mounted", () => present(".lens-pane:not([hidden]) .xterm .xterm-screen"));
  await sleep(1500);
  await click(".lens-pane:not([hidden]) .xterm .xterm-screen");
  await evaluate(`document.querySelector('.lens-pane:not([hidden]) .xterm-helper-textarea')?.focus()`);
  if ((await evaluate("document.activeElement?.className ?? ''")) !== "xterm-helper-textarea") fail("the terminal did not take the focus");
  await sleep(500);
  await key("u", "KeyU", 2); // Ctrl-U: the line cleared of what the terminal answered to the app's queries while attaching
  await send("Input.insertText", { text: "echo e2e-ok" });
  await key("Enter", "Enter", 0, "\r");
  const screen = await until("e2e-ok on the pane's screen", async () => {
    const read = await api("GET", `/api/screen?machine=local&pane=${encodeURIComponent(workspace.pane)}`);
    const lines = (read.text ?? "")
      .replace(/\x1b\[[0-9;?]*[A-Za-z]/g, "")
      .split(/\r?\n/)
      .map((line) => line.trim());
    return lines.includes("e2e-ok") ? lines : null;
  });
  step("the terminal echoed back", `${screen.filter((line) => line.includes("e2e-ok")).length} lines carry e2e-ok`);
  await shot("terminal");

  if (!(await clickText(".lens-switch button", "Chat"))) fail("no Chat button in the lens switch");
  await until("the chat lens", () => present(".lens-pane:not([hidden]) .composer textarea"));
  step("the chat lens shows its composer");

  if (!(await clickText(".rail-btn", "Wall"))) fail("no Wall button on the rail");
  await until("the Wall's bar", () => present(".wall-bar"));
  step("the Wall", await evaluate(`[...document.querySelectorAll('.wall-bar button')].map((b) => b.innerText.trim()).filter(Boolean).join(' | ')`));
  await shot("wall");

  if (!(await clickText(".rail-btn", "Review"))) fail("no Review button on the rail");
  await until("the Review queue", () => present(".queue-head"));
  step("the Review mode", await evaluate(`document.querySelector('.queue-head')?.innerText.replace(/\\s+/g, ' ').trim()`));

  if (!(await clickText(".rail-btn", "Activity"))) fail("no Activity button on the rail");
  await until("the Activity panel", () => present(".activity-dialog"));
  step("the Activity panel opens");
  await shot("activity");
  await key("Escape", "Escape");
  await until("the Activity panel closed", async () => !(await present(".activity-dialog")));

  const mac = await evaluate(`/mac/i.test(navigator.platform)`);
  await key("k", "KeyK", mac ? 4 : 2);
  await until("the palette", () => present(".palette-row"));
  await send("Input.insertText", { text: "go to the wall" });
  const first = await until(
    "the palette's answer",
    async () => {
      const title = await evaluate(`document.querySelector('.palette-row .palette-title')?.innerText ?? ''`);
      return /wall/i.test(title) ? title : null;
    },
    5,
  );
  step("the palette answers", first);
  await key("Escape", "Escape");

  const closed = workspace.id;
  await api("DELETE", `/api/sessions/${encodeURIComponent(closed)}?machine=local`);
  workspace = null;
  await until("the workspace gone from the roster", async () => !(await localSessions()).some((session) => session.id === closed));
  step("the scratch workspace closed");

  if (consoleErrors.length) fail(`${consoleErrors.length} console error(s): ${consoleErrors.slice(0, 3).join(" | ")}`);
  step("no console errors");
} catch (error) {
  console.error(`e2e: FAIL: ${error.message}`);
  try {
    await shot("failure");
    console.error(`e2e: screenshot ${out}/failure.png`);
  } catch {}
  process.exitCode = 1;
} finally {
  if (workspace) await api("DELETE", `/api/sessions/${encodeURIComponent(workspace.id)}?machine=local`).catch(() => {});
  socket.close();
}
