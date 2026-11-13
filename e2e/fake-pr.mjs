// The pull request panel's answers, faked in the browser (CDP's Fetch): the display checks draw the panel without gh or
// GitHub — the branch's two pull requests, the repository's page, one pull request in full with a thread, failed and
// running checks, reviewers and files, a job's steps and its log. Nothing reaches the server; a write is answered ok.

const at = (minutes) => new Date(Date.now() - minutes * 60_000).toISOString();
const HEAD = "0123456789abcdef0123456789abcdef01234567";

export const PR_URL = "https://github.com/octo/trips/pull/482";

const card = (number, title, state, minutes, extra = {}) => ({
  id: `PR_${number}`,
  number,
  title,
  url: `https://github.com/octo/trips/pull/${number}`,
  state,
  draft: false,
  updated_at: at(minutes),
  head: "offline-trips",
  base: "main",
  author: "sam",
  checks: "FAILURE",
  repository: "octo/trips",
  ...extra,
});

const branch = () => ({
  checkout: { branch: "offline-trips", remotes: [{ name: "origin", repo: "octo/trips" }], repo: "octo/trips" },
  items: [card(482, "Save trips for offline use", "OPEN", 120), card(470, "Cache the trip list", "CLOSED", 60 * 30, { checks: "SUCCESS" })],
});

const search = () => ({
  items: [
    card(482, "Save trips for offline use", "OPEN", 120),
    card(488, "Show the packing list on the map", "OPEN", 300, { author: "noor", head: "packing-map", checks: "PENDING" }),
    card(479, "Draft: route sharing", "OPEN", 900, { draft: true, author: "lee", head: "share-route", checks: "" }),
    card(466, "Bump the map tiles", "MERGED", 60 * 50, { author: "dependabot", head: "deps/tiles", checks: "SUCCESS" }),
  ],
  cursors: { all: "CURSOR" },
  total: 31,
});

const detail = () => ({
  id: "PR_kwDOAbc123",
  number: 482,
  title: "Save trips for offline use",
  url: PR_URL,
  body: "Trips can now be saved to the device and opened without a connection.\n\n- Adds a **Save offline** button to each trip\n- Stores maps and the packing list locally\n- Syncs changes when the connection is back",
  state: "OPEN",
  draft: false,
  author: "sam",
  head: "offline-trips",
  base: "main",
  head_oid: HEAD,
  changed_files: 3,
  additions: 120,
  deletions: 14,
  review_decision: "CHANGES_REQUESTED",
  mergeable: "MERGEABLE",
  merge_state: "BLOCKED",
  auto_merge: false,
  merge_methods: ["merge", "squash"],
  auto_allowed: true,
  repository: "octo/trips",
  viewer: "sam",
  comments: [{ id: "IC_1", author: "noor", body: "Nice, this has been on the list for a while.", url: `${PR_URL}#issuecomment-1`, at: at(180) }],
  reviews: [{ id: "PRR_1", author: "priya", body: "The sync needs a retry.", url: `${PR_URL}#pullrequestreview-1`, at: at(40), state: "CHANGES_REQUESTED" }],
  threads: [
    {
      id: "PRRT_1",
      path: "src/offline/sync.ts",
      line: 42,
      resolved: false,
      outdated: false,
      can_resolve: true,
      can_reply: true,
      comments: [
        { id: "PRRC_1", author: "priya", body: "This drops the change if the request fails. Can it retry?", url: `${PR_URL}#discussion_r1`, at: at(40), hunk: "@@ -38,4 +38,6 @@\n   const trip = await load(id);\n+  await upload(trip);\n+  markSynced(id);" },
        { id: "PRRC_2", author: "sam", body: "On it.", url: `${PR_URL}#discussion_r2`, at: at(20) },
      ],
    },
    { id: "PRRT_2", path: "README.md", line: 3, resolved: true, outdated: true, can_resolve: true, can_reply: true, comments: [{ id: "PRRC_3", author: "noor", body: "Typo in the heading.", url: `${PR_URL}#discussion_r3`, at: at(600), hunk: "@@ -1,3 +1,3 @@\n-# Trips\n+# Trps" }] },
  ],
  checks: [
    { name: "unit", workflow: "CI", run: 777, job: 1229, status: "COMPLETED", conclusion: "FAILURE", url: "https://github.com/octo/trips/actions/runs/777/job/1229", started_at: at(30), completed_at: at(27) },
    { name: "lint", workflow: "CI", run: 777, job: 1230, status: "COMPLETED", conclusion: "SUCCESS", url: "https://github.com/octo/trips/actions/runs/777/job/1230", started_at: at(30), completed_at: at(29) },
    { name: "e2e", workflow: "E2E", run: 778, job: 1300, status: "IN_PROGRESS", url: "https://github.com/octo/trips/actions/runs/778/job/1300", started_at: at(12) },
    { name: "build", workflow: "CI", run: 777, job: 1231, status: "COMPLETED", conclusion: "SUCCESS", url: "https://github.com/octo/trips/actions/runs/777/job/1231", started_at: at(30), completed_at: at(26) },
    { name: "docs", workflow: "Docs", run: 779, job: 1400, status: "COMPLETED", conclusion: "SUCCESS", url: "https://github.com/octo/trips/actions/runs/779/job/1400", started_at: at(30), completed_at: at(29) },
    { name: "preview", status: "PENDING", url: "https://ci.example/7", started_at: at(30) },
  ],
  reviewers: [
    { login: "priya", state: "CHANGES_REQUESTED" },
    { login: "noor", state: "APPROVED" },
    { login: "lee", state: "REQUESTED" },
  ],
});

const job = () => ({
  id: 1229,
  name: "unit",
  status: "completed",
  conclusion: "failure",
  url: "https://github.com/octo/trips/actions/runs/777/job/1229",
  steps: [
    { number: 1, name: "Set up job", status: "completed", conclusion: "success", started_at: at(30), completed_at: at(30) },
    { number: 2, name: "Install", status: "completed", conclusion: "success", started_at: at(30), completed_at: at(29) },
    { number: 3, name: "Run tests", status: "completed", conclusion: "failure", started_at: at(29), completed_at: at(27) },
  ],
});

const log = () => ({ text: "FAIL src/offline/sync.test.ts\n  ● sync › retries a failed upload\n    expect(received).toBe(expected)\n    Expected: 2\n    Received: 1\nTests: 1 failed, 41 passed, 42 total", cut: false });

const files = () => ({
  files: [
    { path: "src/offline/sync.ts", status: "modified", additions: 40, deletions: 4, patch: "@@ -38,4 +38,6 @@\n   const trip = await load(id);\n+  await upload(trip);\n+  markSynced(id);\n   return trip;" },
    { path: "src/offline/store.ts", status: "added", additions: 78, deletions: 0, patch: "@@ -0,0 +1,3 @@\n+export const store = new Map<string, Trip>();\n+export const save = (t: Trip) => store.set(t.id, t);\n+export const load = (id: string) => store.get(id);" },
    { path: "README.md", status: "modified", additions: 2, deletions: 10, patch: "@@ -1,3 +1,3 @@\n-# Trips\n+# Trips, offline too" },
  ],
});

// answerFor is the faked answer to a /api/pr/… request, null for anything else
export function answerFor(url, method) {
  const path = new URL(url).pathname;
  if (method !== "GET") return { ok: true };
  if (path === "/api/pr/branch") return branch();
  if (path === "/api/pr/search") return search();
  if (path === "/api/pr/detail") return detail();
  if (path === "/api/pr/job") return job();
  if (path === "/api/pr/log") return log();
  if (path === "/api/pr/files") return files();
  return null;
}

// prScreens opens the panel the way a person does — the palette's row for the workspace on screen — and draws its two
// lists, a pull request on each of its tabs, and the merge's confirm card with what stands in the way, cancelled
export async function prScreens({ screen, palette, tap, present, until, evaluate, count, failed, check, onAgents, sleep }) {
  const open = async () => {
    await onAgents();
    await palette("pull-requests");
    if (!(await until(() => present(".pr-panel .pr-views"), 6))) throw new Error("the pull request panel did not open");
  };
  const branchList = async () => {
    await open();
    await tap(".pr-panel .pr-views button");
    if (!(await until(async () => (await count(".pr-panel .pr-row:not(.is-ghost)")) === 2, 6))) failed("the Branch list does not show the branch's two pull requests");
  };
  const detail = async (tab) => {
    await branchList();
    await tap(".pr-panel .pr-row");
    if (!(await until(() => present(".pr-panel .pr-tabs"), 6))) throw new Error("the pull request did not open");
    if (tab > 0) await evaluate(`document.querySelectorAll(".pr-panel .pr-tabs button")[${tab}]?.click()`);
    await sleep(300);
  };
  await screen("pr-branch", { open: branchList, expect: [".pr-panel .pr-row", ".pr-panel .pr-foot"] });
  await screen("pr-repo", {
    open: async () => {
      await open();
      await evaluate(`document.querySelectorAll(".pr-panel .pr-views button")[1]?.click()`);
      if (!(await until(async () => (await count(".pr-panel .pr-row:not(.is-ghost)")) === 4, 6))) failed("the Repo list does not show the repository's page");
      check(await present(".pr-panel .pr-more"), "the Repo list offers no next page");
    },
    expect: [".pr-panel .pr-filters input", ".pr-panel .pr-filter-row .sel"],
  });
  await screen("pr-overview", {
    open: async () => {
      await detail(0);
      check((await count(".pr-panel .pr-stats .pr-stat")) === 3, "the stats strip does not say checks, reviews and unresolved threads");
      check(await present(".pr-panel .pr-stat .is-bad"), "a failed check or a review asking for changes is not in red");
    },
    expect: [".pr-panel .pr-about .pr-title", ".pr-panel .pr-actions", ".pr-panel .pr-group .pr-line"],
  });
  await screen("pr-comments", {
    open: async () => {
      await detail(1);
      check(await present(".pr-panel .pr-entry .fv-diff"), "a thread's code is not drawn as diff rows");
      check(await present(".pr-panel .pr-new"), "a comment this device has not seen is not marked New");
    },
    expect: [".pr-panel .pr-entry", ".pr-panel .pr-new-comment textarea"],
  });
  await screen("pr-checks", {
    open: async () => {
      await detail(2);
      await tap(".pr-panel .pr-check-name");
      if (!(await until(() => present(".pr-panel .pr-step"), 6))) failed("a failed check does not unfold to its job's steps");
      await evaluate(`[...document.querySelectorAll(".pr-panel .pr-check-body .linkish")].at(-1)?.click()`);
      if (!(await until(() => present(".pr-panel .pr-log"), 6))) failed("the failed steps' output does not show");
    },
    expect: [".pr-panel .pr-group-head", ".pr-panel .pr-check-row .btn"],
  });
  await screen("pr-files", {
    open: async () => {
      await detail(3);
      await evaluate(`[...document.querySelectorAll(".pr-panel .pr-file-row")].find((r) => r.querySelector(".pr-add"))?.click()`);
      if (!(await until(() => present(".pr-panel .pr-patch"), 4))) failed("a file's patch does not show");
    },
    expect: [".pr-panel .pr-files-head", ".pr-panel .pr-file-row .pr-add"],
  });
  await screen("pr-merge-confirm", {
    open: async () => {
      await detail(0);
      await tap(".pr-panel .pr-actions");
      if (!(await until(() => present("[role=menu] [role=menuitem]"), 4))) throw new Error("the Actions menu did not open");
      check((await count("[role=menu] [role=menuitem]")) === 4, "Actions does not offer merge when ready, merge, squash and close");
      await evaluate(`document.querySelectorAll("[role=menu] [role=menuitem]")[2]?.click()`);
      if (!(await until(() => present(".pr-panel .pr-confirm .pr-in-the-way"), 4))) failed("the merge's confirm does not say what stands in the way");
      check(await present(".pr-panel .pr-confirm textarea"), "a squash's confirm has no commit message to edit");
    },
    expect: [".pr-panel .pr-confirm .btn.primary"],
    close: async () => {
      await evaluate(`[...document.querySelectorAll(".pr-panel .pr-confirm .btn")].find((b) => !b.classList.contains("primary"))?.click()`);
      if (!(await until(async () => !(await present(".pr-panel .pr-confirm")), 3))) failed("Cancel does not take the confirm card away");
      await evaluate(`document.querySelector(".modal.pr-panel .modal-head > .icon-btn")?.click()`);
      await sleep(200);
    },
  });
}

// fakePullRequests answers one paused request through the DevTools protocol's send
export async function fakePullRequests({ requestId, request }, send) {
  const body = answerFor(request.url, request.method);
  if (body === null) return send("Fetch.continueRequest", { requestId });
  await send("Fetch.fulfillRequest", {
    requestId,
    responseCode: 200,
    responseHeaders: [{ name: "Content-Type", value: "application/json" }],
    body: Buffer.from(JSON.stringify(body)).toString("base64"),
  });
}
