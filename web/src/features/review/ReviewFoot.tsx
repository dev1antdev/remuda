// The lens's foot: the draft bar with its comments and the two verdicts, and the dialogs it opens — the agent to send the
// comments to, and the verdict with its note and, for an approval, what the agent is asked afterwards.

import { errMsg } from "@/lib/text";
import { t } from "@/lib/i18n";
import SendToAgent from "@/features/agents/SendToAgent";
import type { AgentTarget } from "@/features/agents/targets";
import type { Review } from "./api";
import { DraftBar } from "./DraftBar";
import type { SecretFinding } from "./secrets";
import type { Draft } from "./useDraft";
import type { SecretCheck } from "./useSecretCheck";
import { VerdictDialog } from "./VerdictDialog";

export function ReviewFoot({ draft, review, agent, agents, machineId, paneId, dir, check, listed, onJump, onSent, onError }: {
  draft: Draft;
  review: Review | null;
  agent: string; // the pane's agent, "" for a shell pane
  agents: AgentTarget[];
  machineId: string;
  paneId: string;
  dir: string;
  check: SecretCheck;
  listed: string[];
  onJump: (finding: SecretFinding) => void;
  onSent: () => void; // a verdict went out: the lens looks again
  onError: (text: string) => void;
}) {
  const currentPane = { machineId, paneId };
  const files = review?.files.length ?? 0;
  const commits = review?.commits.length ?? 0;
  return (
    <>
      {review && draft.canComment && (files > 0 || commits > 0 || draft.comments.length > 0) && (
        <DraftBar
          comments={draft.comments.length}
          agent={agent}
          otherAgents={agents.length > 1}
          sending={draft.sending}
          onClear={draft.clear}
          onSend={() => void draft.sendComments({ machineId, paneId, agent }).catch((failure) => onError(errMsg(failure)))}
          onPick={() => draft.setPickingAgent(true)}
          onVerdict={draft.setOpenVerdict}
        />
      )}
      {draft.pickingAgent && (
        <SendToAgent
          title={t("Send the review to")}
          text={draft.message()}
          agents={agents}
          currentPane={currentPane}
          rememberKey={`review:${machineId}|${dir}`}
          onClose={() => draft.setPickingAgent(false)}
          onSend={(target, text) => draft.sendComments(target, text)}
        />
      )}
      {draft.openVerdict && (
        <VerdictDialog
          verdict={draft.openVerdict}
          paneAgent={agent}
          agents={agents}
          currentPane={currentPane}
          comments={draft.comments.length}
          check={check}
          listed={listed}
          onJump={onJump}
          onClose={draft.closeVerdict}
          onSend={(target, note, ask) => draft.sendVerdict(draft.openVerdict!, target, note, ask, onSent)}
        />
      )}
    </>
  );
}
