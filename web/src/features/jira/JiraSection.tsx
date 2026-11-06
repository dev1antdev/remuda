// The Jira section of Settings: the site, the account and the token (kept on the server, never shown back), a Test
// that says who the account is, and Remove. Until a site is set remuda asks Jira nothing.

import { useEffect, useState } from "react";
import { act } from "@/app/act";
import { actions } from "@/app/store";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Notice } from "@/ui";
import { t, tn } from "@/lib/i18n";
import { fetchJira, removeJira, saveJira, testJira, type JiraView } from "./api";

export default function JiraSection() {
  const { data: view, mutate } = useQuery<JiraView>(() => fetchJira(), []);
  const [site, setSite] = useState("");
  const [account, setAccount] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<{ ok: boolean; text: string } | null>(null);
  useEffect(() => {
    if (!view) return;
    setSite(view.site);
    setAccount(view.account);
  }, [view]);

  const run = async (work: () => Promise<string>) => {
    setBusy(true);
    setOutcome(null);
    try {
      setOutcome({ ok: true, text: await work() });
    } catch (e) {
      setOutcome({ ok: false, text: errMsg(e) });
    } finally {
      setBusy(false);
    }
  };
  const save = () =>
    run(async () => {
      const saved = await saveJira(site, account, token);
      mutate(() => saved);
      setToken("");
      await act(Promise.resolve());
      const test = await testJira();
      return t("Signed in as {who}; {projects} visible.", { who: test.who, projects: tn(test.projects, "{n} project", "{n} projects") });
    });
  const test = () =>
    run(async () => {
      const answer = await testJira();
      return t("Signed in as {who}; {projects} visible.", { who: answer.who, projects: tn(answer.projects, "{n} project", "{n} projects") });
    });
  const remove = () =>
    actions.ask(t("Forget the Jira site and its token? The chips go, the tasks made from issues stay."), t("Remove"), () =>
      void run(async () => {
        const cleared = await removeJira();
        mutate(() => cleared);
        setSite("");
        setAccount("");
        setToken("");
        await act(Promise.resolve());
        return t("Jira is off.");
      }),
    );

  const cloud = /\.(atlassian\.net|jira\.com)(\/|$)/i.test(site);
  return (
    <section className="settings-section jira-settings">
      <h3>{t("Jira")}</h3>
      <p className="jira-hint">
        {t("The issue keys your branches and agents name — CRMOPS-6279 — become chips with their status, and a task can come from an issue. remuda asks your site only while a page is open, and nothing before you set it here.")}
      </p>
      <label className="jira-field">
        <span>{t("Site")}</span>
        <input value={site} placeholder="https://yourcompany.atlassian.net" spellCheck={false} inputMode="url" onChange={(e) => setSite(e.target.value)} />
      </label>
      <label className="jira-field">
        <span>{cloud ? t("Account e-mail") : t("Account (empty for a personal access token)")}</span>
        <input value={account} placeholder={cloud ? "you@company.com" : ""} spellCheck={false} autoComplete="off" onChange={(e) => setAccount(e.target.value)} />
      </label>
      <label className="jira-field">
        <span>{cloud ? t("API token") : t("Token")}</span>
        <input
          type="password"
          value={token}
          autoComplete="new-password"
          placeholder={view?.has_token ? t("kept — type a new one to replace it") : cloud ? t("from id.atlassian.com › Security › API tokens") : t("a personal access token")}
          onChange={(e) => setToken(e.target.value)}
        />
      </label>
      <div className="jira-actions">
        <button type="button" className="btn primary sm" disabled={busy || !site.trim() || (!token.trim() && !view?.has_token)} onClick={() => void save()}>
          {busy ? "…" : t("Save")}
        </button>
        <button type="button" className="btn sm" disabled={busy || !view?.has_token} onClick={() => void test()}>
          {t("Test")}
        </button>
        {view?.has_token && (
          <button type="button" className="btn danger sm" disabled={busy} onClick={remove}>
            {t("Remove")}
          </button>
        )}
      </div>
      {outcome && <Notice tone={outcome.ok ? "success" : "error"}>{outcome.text}</Notice>}
    </section>
  );
}
