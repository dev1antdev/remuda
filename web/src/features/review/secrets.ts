// The secret check before an approval that asks for a commit or a pull request: what the server found in the work, in
// words; whether the dialog's button waits for a confirm; which diff rows a finding sits on; and what the lens head
// says of Claude Code's settings files.

import { t, tn } from "@/lib/i18n";
import type { DiffRow } from "./hunks";
import type { VerdictAsk } from "./draft";

// SecretFinding is one thing that looks like a secret: line 0 is the file as a whole (a keystore); the excerpt is masked;
// commit is the commit that adds it, or STAGED when only the index holds it (what git commit carries all the same)
export type SecretFinding = { file: string; line: number; rule: string; excerpt: string; commit?: string };
export const STAGED = "staged";
export type SkipReason = "binary" | "large" | "partly" | "budget" | "many" | "older";
export type SkippedFile = { path?: string; why: SkipReason; count?: number }; // count: the untracked files past the cap, or the commits
// SecretScan is the server's answer: the findings, what it did not read, how many files and commits it read
export type SecretScan = { findings: SecretFinding[]; more?: number; skipped?: SkippedFile[]; files: number; commits?: number; push?: boolean; at: string };
// SettingsAudit is one of Claude Code's settings files in the folder or its repository's root
export type SettingsAudit = { path: string; local?: boolean; tracked?: boolean; ignored?: boolean; findings?: SecretFinding[] };

const RULES: Record<string, string> = {
  "private-key": t("Private key"),
  "aws-access-key": t("AWS access key"),
  "github-token": t("GitHub token"),
  "github-fine-grained-token": t("GitHub fine-grained token"),
  "gitlab-token": t("GitLab token"),
  "slack-token": t("Slack token"),
  "slack-webhook": t("Slack webhook"),
  "stripe-live-key": t("Stripe live key"),
  "anthropic-key": t("Anthropic API key"),
  "openai-key": t("OpenAI API key"),
  "google-api-key": t("Google API key"),
  "telegram-bot-token": t("Telegram bot token"),
  "npm-token": t("npm token"),
  "huggingface-token": t("Hugging Face token"),
  "groq-key": t("Groq API key"),
  "xai-key": t("xAI API key"),
  "openrouter-key": t("OpenRouter API key"),
  "sendgrid-key": t("SendGrid API key"),
  "digitalocean-token": t("DigitalOcean token"),
  "docker-token": t("Docker Hub token"),
  "pypi-token": t("PyPI token"),
  "google-oauth-secret": t("Google OAuth client secret"),
  "shopify-token": t("Shopify token"),
  "linear-key": t("Linear API key"),
  "atlassian-token": t("Atlassian API token"),
  "age-key": t("age secret key"),
  "vault-token": t("Vault token"),
  jwt: t("JSON Web Token"),
  "auth-header": t("Authorization header"),
  "url-password": t("Password in an address"),
  "assigned-secret": t("Secret value"),
  "key-file": t("Keystore file"),
};

export const ruleWords = (rule: string): string => RULES[rule] ?? t("Secret value");

// CheckState is where the check stands for the dialog: not asked (no commit or pull request), running, its answer, or failed
export type CheckState = "off" | "checking" | "clean" | "found" | "failed";

export function checkState(asked: boolean, scan: SecretScan | null, error: string, fetching: boolean): CheckState {
  if (!asked) return "off";
  if (error && !fetching) return "failed";
  if (!scan) return "checking";
  return foundCount(scan) > 0 ? "found" : "clean";
}

export const foundCount = (scan: SecretScan | null): number => (scan ? scan.findings.length + (scan.more ?? 0) : 0);

// heldBack: the approval that asks for a commit or a pull request waits for a confirm, after findings or a failed check
export const heldBack = (state: CheckState): boolean => state === "found" || state === "failed";

// asksPush: the pull request's check reads the commits a push would send, the commit's only the work
export const asksPush = (ask: VerdictAsk): boolean => ask === "pr";

export function stateWords(state: CheckState, scan: SecretScan | null, ask: VerdictAsk): string {
  if (state === "checking") return asksPush(ask) ? t("Looking for secrets in what the commit and the push would carry…") : t("Looking for secrets in what the commit would carry…");
  if (state === "found") return tn(foundCount(scan), "{n} thing here looks like a secret. Take it out, or send anyway once you have looked.", "{n} things here look like secrets. Take them out, or send anyway once you have looked.");
  if (state !== "clean" || !scan) return "";
  const files = scan.files > 0 ? tn(scan.files, "No secrets in the {n} file the commit would carry.", "No secrets in the {n} files the commit would carry.") : t("Nothing for the commit to carry.");
  return scan.push && scan.commits ? `${files} ${tn(scan.commits, "The {n} commit not pushed yet holds none either.", "The {n} commits not pushed yet hold none either.")}` : files;
}

// sendAnywayAsk is the confirm before an approval goes out past the check: it names how many findings, or that the check failed
export function sendAnywayAsk(state: CheckState, scan: SecretScan | null, ask: VerdictAsk): string {
  const push = asksPush(ask);
  if (state === "failed") {
    return push
      ? t("The secret check did not finish, so nothing is known about secrets in what the agent would commit and push. Approve and ask for the pull request anyway?")
      : t("The secret check did not finish, so nothing is known about secrets in what the agent would commit. Approve and ask for the commit anyway?");
  }
  const n = foundCount(scan);
  return push
    ? tn(n, "{n} possible secret is in what the agent would commit and push. Approve and ask for the pull request anyway?", "{n} possible secrets are in what the agent would commit and push. Approve and ask for the pull request anyway?")
    : tn(n, "{n} possible secret is in what the agent would commit. Approve and ask for the commit anyway?", "{n} possible secrets are in what the agent would commit. Approve and ask for the commit anyway?");
}

// whereWords is a finding's place: the commit that adds it (or "staged"), the file and its line, or the file alone
export function whereWords(finding: SecretFinding): string {
  const commit = finding.commit === STAGED ? t("staged") : finding.commit;
  return `${commit ? `${commit} · ` : ""}${finding.line > 0 ? `${finding.file}:${finding.line}` : finding.file}`;
}

// jumpable: a finding the lens can open at its row, one of the work's on a line of a file it lists
export const jumpable = (finding: SecretFinding, listed: string[]): boolean => !finding.commit && finding.line > 0 && listed.includes(finding.file);

const SKIPS: { why: SkipReason; words: (n: number) => string }[] = [
  { why: "binary", words: (n) => tn(n, "{n} binary file", "{n} binary files") },
  { why: "large", words: (n) => tn(n, "{n} file over 1 MB", "{n} files over 1 MB") },
  { why: "partly", words: (n) => tn(n, "the end of {n} change over 1 MB", "the end of {n} changes over 1 MB") },
  { why: "budget", words: (n) => tn(n, "{n} file past the 8 MB read in all", "{n} files past the 8 MB read in all") },
  { why: "many", words: (n) => tn(n, "{n} more untracked file", "{n} more untracked files") },
  { why: "older", words: (n) => tn(n, "{n} older commit not pushed yet", "{n} older commits not pushed yet") },
];

const COUNTED: SkipReason[] = ["many", "older"]; // skips that count many at once

// skippedWords says what the check did not read, "" when it read everything
export function skippedWords(skipped: SkippedFile[] | undefined): string {
  if (!skipped?.length) return "";
  const parts = SKIPS.map(({ why, words }) => {
    const n = skipped.filter((skip) => skip.why === why).reduce((sum, skip) => sum + (COUNTED.includes(why) ? (skip.count ?? 0) : 1), 0);
    return n > 0 ? words(n) : "";
  }).filter(Boolean);
  return parts.length ? t("Not read: {what}.", { what: parts.join(", ") }) : "";
}

// onSecretRow says whether a finding sits on a diff row: the work's, on the new file's line, never a removed one
export const onSecretRow = (finding: SecretFinding, row: DiffRow): boolean => !finding.commit && finding.line > 0 && row.kind !== "meta" && row.kind !== "del" && row.newLine === finding.line;

// secretsByFile is the work's findings by their file, for the rows of each diff
export function secretsByFile(scan: SecretScan | null): Map<string, SecretFinding[]> {
  const byFile = new Map<string, SecretFinding[]>();
  for (const finding of scan?.findings ?? []) if (!finding.commit && finding.line > 0) byFile.set(finding.file, [...(byFile.get(finding.file) ?? []), finding]);
  return byFile;
}

// SettingsNote is what the lens head says of one settings file: what is wrong, then what to do
export type SettingsNote = { path: string; tone: "warn" | "hint"; text: string; fix: string };

export function settingsNotes(audits: SettingsAudit[] | undefined): SettingsNote[] {
  const notes: SettingsNote[] = [];
  for (const audit of audits ?? []) {
    const findings = audit.findings ?? [];
    const untrack = t("Untrack it with git rm --cached {path} and add it to .gitignore.", { path: audit.path });
    if (findings.length > 0) {
      const where = findings
        .slice(0, 3)
        .map((finding) => t("{what} at line {line}", { what: ruleWords(finding.rule), line: finding.line }))
        .join(", ");
      const text = tn(findings.length, "{path} holds {n} thing that looks like a secret ({where}): an allowed command can carry a token.", "{path} holds {n} things that look like secrets ({where}): an allowed command can carry a token.", { path: audit.path, where });
      const revoke = t("Take it out of the file and revoke it where it was issued.");
      notes.push({ path: audit.path, tone: "warn", text, fix: audit.local && audit.tracked ? `${revoke} ${untrack}` : revoke });
    } else if (audit.local && audit.tracked) {
      notes.push({ path: audit.path, tone: "hint", text: t("{path} is tracked by git: Claude Code keeps your own approvals there, and an allowed command can carry a token.", { path: audit.path }), fix: untrack });
    } else if (audit.local && !audit.ignored) {
      notes.push({ path: audit.path, tone: "hint", text: t("{path} is not ignored by git: a commit of everything would carry your own approvals.", { path: audit.path }), fix: t("Add it to .gitignore.") });
    }
  }
  return notes;
}
