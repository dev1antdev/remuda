import { describe, expect, it } from "vitest";
import type { DiffRow } from "./hunks";
import {
  checkState,
  foundCount,
  heldBack,
  jumpable,
  onSecretRow,
  ruleWords,
  secretsByFile,
  sendAnywayAsk,
  settingsNotes,
  skippedWords,
  stateWords,
  whereWords,
  type SecretFinding,
  type SecretScan,
} from "./secrets";

const finding = (over: Partial<SecretFinding> = {}): SecretFinding => ({ file: "config/app.yml", line: 12, rule: "github-token", excerpt: "token: ghp_••••••••", ...over });
const scan = (over: Partial<SecretScan> = {}): SecretScan => ({ findings: [], files: 3, at: "2026-11-12T10:00:00Z", ...over });
const row = (over: Partial<DiffRow>): DiffRow => ({ kind: "add", text: "+x", newLine: 12, oldLine: null, ...over });

describe("the check's state", () => {
  it("is off without a commit or a pull request asked, checking until an answer, then clean or found", () => {
    expect(checkState(false, scan(), "", false)).toBe("off");
    expect(checkState(true, null, "", true)).toBe("checking");
    expect(checkState(true, scan(), "", false)).toBe("clean");
    expect(checkState(true, scan({ findings: [finding()] }), "", false)).toBe("found");
    expect(checkState(true, scan({ more: 2 }), "", false)).toBe("found");
  });
  it("fails on an error, and checks again while a retry runs", () => {
    expect(checkState(true, null, "the secret check could not run: timed out", false)).toBe("failed");
    expect(checkState(true, null, "the secret check could not run: timed out", true)).toBe("checking");
  });
  it("holds the approval back after findings or a failure, never while clean or off", () => {
    expect(heldBack("found")).toBe(true);
    expect(heldBack("failed")).toBe(true);
    expect(heldBack("clean")).toBe(false);
    expect(heldBack("checking")).toBe(false);
    expect(heldBack("off")).toBe(false);
  });
  it("counts the findings past the cap", () => {
    expect(foundCount(scan({ findings: [finding(), finding({ line: 13 })], more: 5 }))).toBe(7);
    expect(foundCount(null)).toBe(0);
  });
});

describe("the check's words", () => {
  it("say what was read, and the commits a pull request's check read too", () => {
    expect(stateWords("clean", scan({ files: 1 }), "commit")).toBe("No secrets in the 1 file the commit would carry.");
    expect(stateWords("clean", scan({ files: 4, push: true, commits: 2 }), "pr")).toBe("No secrets in the 4 files the commit would carry. The 2 commits not pushed yet hold none either.");
    expect(stateWords("clean", scan({ files: 0 }), "commit")).toBe("Nothing for the commit to carry.");
    expect(stateWords("found", scan({ findings: [finding()] }), "commit")).toMatch(/^1 thing here looks like a secret/);
    expect(stateWords("checking", null, "pr")).toMatch(/commit and the push/);
  });
  it("name how many findings in the confirm, or that the check failed", () => {
    expect(sendAnywayAsk("found", scan({ findings: [finding(), finding({ line: 3 })] }), "commit")).toBe("2 possible secrets are in what the agent would commit. Approve and ask for the commit anyway?");
    expect(sendAnywayAsk("found", scan({ findings: [finding()] }), "pr")).toBe("1 possible secret is in what the agent would commit and push. Approve and ask for the pull request anyway?");
    expect(sendAnywayAsk("failed", null, "commit")).toMatch(/^The secret check did not finish/);
  });
  it("name every rule, and a rule the page does not know as a secret value", () => {
    const rules = ["private-key", "aws-access-key", "github-token", "github-fine-grained-token", "gitlab-token", "slack-token", "slack-webhook", "stripe-live-key", "anthropic-key", "openai-key",
      "google-api-key", "telegram-bot-token", "npm-token", "huggingface-token", "groq-key", "xai-key", "openrouter-key", "sendgrid-key", "digitalocean-token", "docker-token", "pypi-token",
      "google-oauth-secret", "shopify-token", "linear-key", "atlassian-token", "age-key", "vault-token", "jwt", "auth-header", "url-password", "key-file"];
    for (const rule of rules) expect(ruleWords(rule), rule).not.toBe(ruleWords("something-new"));
    expect(new Set(rules.map(ruleWords)).size).toBe(rules.length);
    expect(ruleWords("github-token")).toBe("GitHub token");
    expect(ruleWords("url-password")).toBe("Password in an address");
    expect(ruleWords("something-new")).toBe("Secret value");
  });
  it("place a finding by its commit, its file and its line", () => {
    expect(whereWords(finding())).toBe("config/app.yml:12");
    expect(whereWords(finding({ commit: "abc1234" }))).toBe("abc1234 · config/app.yml:12");
    expect(whereWords(finding({ line: 0, file: "certs/store.p12" }))).toBe("certs/store.p12");
    expect(whereWords(finding({ commit: "staged", file: ".env", line: 2 }))).toBe("staged · .env:2");
  });
  it("say what was not read, by reason", () => {
    expect(skippedWords(undefined)).toBe("");
    expect(skippedWords([{ path: "a.png", why: "binary" }, { path: "b.png", why: "binary" }, { path: "dump.sql", why: "large" }, { why: "many", count: 12 }])).toBe(
      "Not read: 2 binary files, 1 file over 1 MB, 12 more untracked files.",
    );
    expect(skippedWords([{ why: "older", count: 340 }])).toBe("Not read: 340 older commits not pushed yet.");
  });
});

describe("the rows a finding marks", () => {
  it("is the new file's line of the work's finding, never a removed line or a commit's", () => {
    expect(onSecretRow(finding(), row({}))).toBe(true);
    expect(onSecretRow(finding(), row({ kind: "", oldLine: 10 }))).toBe(true);
    expect(onSecretRow(finding(), row({ kind: "del", newLine: null, oldLine: 12 }))).toBe(false);
    expect(onSecretRow(finding(), row({ newLine: 11 }))).toBe(false);
    expect(onSecretRow(finding({ commit: "abc1234" }), row({}))).toBe(false);
    expect(onSecretRow(finding({ commit: "staged" }), row({}))).toBe(false); // the index's line, which the file may no longer hold
    expect(onSecretRow(finding({ line: 0 }), row({ newLine: 0 }))).toBe(false);
  });
  it("groups the work's findings by file, and jumps only to a listed file's line", () => {
    const byFile = secretsByFile(scan({ findings: [finding(), finding({ line: 20 }), finding({ file: "b.ts" }), finding({ commit: "abc1234" }), finding({ line: 0, file: "k.p12" })] }));
    expect(byFile.get("config/app.yml")?.map((f) => f.line)).toEqual([12, 20]);
    expect([...byFile.keys()]).toEqual(["config/app.yml", "b.ts"]);
    expect(jumpable(finding(), ["config/app.yml"])).toBe(true);
    expect(jumpable(finding(), ["other.ts"])).toBe(false);
    expect(jumpable(finding({ commit: "abc1234" }), ["config/app.yml"])).toBe(false);
  });
});

describe("the settings files", () => {
  it("say nothing of an ignored local file without a secret, nor of the shared one", () => {
    expect(settingsNotes([{ path: ".claude/settings.local.json", local: true, ignored: true }, { path: ".claude/settings.json", tracked: true }])).toEqual([]);
  });
  it("tell of a tracked local file and of one git would carry", () => {
    const [tracked] = settingsNotes([{ path: ".claude/settings.local.json", local: true, tracked: true }]);
    expect(tracked).toMatchObject({ tone: "hint", fix: "Untrack it with git rm --cached .claude/settings.local.json and add it to .gitignore." });
    const [loose] = settingsNotes([{ path: ".claude/settings.local.json", local: true }]);
    expect(loose).toMatchObject({ tone: "hint", fix: "Add it to .gitignore." });
  });
  it("warn of a secret in either file, with where it is and what to do", () => {
    const note = settingsNotes([{ path: ".claude/settings.local.json", local: true, tracked: true, findings: [finding({ file: ".claude/settings.local.json", line: 4, rule: "auth-header" })] }])[0]!;
    expect(note.tone).toBe("warn");
    expect(note.text).toBe(".claude/settings.local.json holds 1 thing that looks like a secret (Authorization header at line 4): an allowed command can carry a token.");
    expect(note.fix).toMatch(/revoke it.*git rm --cached/);
    const shared = settingsNotes([{ path: ".claude/settings.json", tracked: true, findings: [finding({ line: 2 }), finding({ line: 9, rule: "npm-token" })] }])[0]!;
    expect(shared.text).toMatch(/holds 2 things that look like secrets \(GitHub token at line 2, npm token at line 9\)/);
    expect(shared.fix).not.toMatch(/git rm/);
  });
});
