// The middle of the status bar: where the pane on screen works — its workspace's branch, its pull request's icon in
// the colour of its state, the issue its work names — in quiet text; the request and the issue open on a click.

import type { Located } from "@/app/current";
import { useSharing } from "@/features/roster/useSharing";
import { issueTitle } from "@/features/jira/jira";
import { openUrl } from "@/lib/host";
import { t } from "@/lib/i18n";
import { Icon, Tip } from "@/ui";
import { placeOf, prTip, prTone } from "./place";

export default function Place({ current }: { current: Located | null }) {
  const { sessionHidden } = useSharing();
  if (!current || sessionHidden(current.machine.id, current.session)) return <div className="status-place" />;
  const { branch, pr, issue } = placeOf(current.session, current.pane);
  return (
    <div className="status-place">
      {branch && (
        <Tip text={t("The branch {branch} is checked out in this workspace", { branch })} side="top">
          <span className="status-branch" tabIndex={0}>
            <Icon name="git-branch" size={12} />
            <span className="status-branch-name">{branch}</span>
          </span>
        </Tip>
      )}
      {pr && (
        <Tip text={prTip(pr)} side="top">
          <button type="button" className={`btn ghost sm status-item status-pr is-${prTone(pr)}`} onClick={() => openUrl(pr.url)} aria-label={t("Open {label}", { label: pr.label })}>
            <Icon name="git-pull-request" size={12} />
            <span>{pr.label}</span>
          </button>
        </Tip>
      )}
      {issue && (
        <Tip text={issueTitle(issue)} side="top">
          <button type="button" className="btn ghost sm status-item status-issue" onClick={() => openUrl(issue.url)} aria-label={t("Open {key}", { key: issue.key })}>
            <Icon name="circle-dot" size={12} />
            <span>{issue.key}</span>
          </button>
        </Tip>
      )}
    </div>
  );
}
