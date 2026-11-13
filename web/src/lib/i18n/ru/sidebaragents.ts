// The Russian words of the sidebar's agents: "Needs you" and the pinned group over the machines, a line per agent with
// its subagents, the folded heads, the foot, the orders by priority and by ticket prefix, and the walk keys.

import type { Dictionary } from "../../i18n";

export const sidebaragents: Dictionary = {
  "Needs you": "Нужны вы",
  "sidebar|Pinned": "Закреплённые",
  "{n} need you": ["{n} ждёт вас", "{n} ждут вас", "{n} ждут вас"],
  "{n} working": ["{n} работает", "{n} работают", "{n} работают"],
  Idle: "Простаивают",
  "No prefix": "Без префикса",
  "By priority": "По приоритету",
  "By ticket prefix": "По префиксу задачи",
  "by priority: needs you, done, working, idle": "по приоритету: нужны вы, готово, работают, простаивают",
  "by ticket prefix: PR-12, INS-4…": "по префиксу задачи: PR-12, INS-4…",
  "<1m": "<1 мин",
  "finished, not looked at yet": "закончили, вы ещё не смотрели",
  "Show its {n} subagents": ["Показать его {n} субагента", "Показать его {n} субагента", "Показать его {n} субагентов"],
  "Fold its subagents": "Свернуть его субагентов",
  "Pin this agent to the top": "Закрепить этого агента наверху",
  "a subagent": "субагент",
  "Open its conversation": "Открыть его разговор",
  "{n} more at work": ["ещё {n} работает", "ещё {n} работают", "ещё {n} работают"],
  "Go to the one that has waited longest": "Перейти к тому, кто ждёт дольше всех",
  primary: "основной",
  "The repository's main checkout; its worktrees are listed beside it": "Основная копия репозитория; его worktree перечислены рядом",
  "{n} commits not pushed": ["{n} коммит не отправлен", "{n} коммита не отправлены", "{n} коммитов не отправлены"],
  "{n} commits to pull": ["отстаёт на {n} коммит", "отстаёт на {n} коммита", "отстаёт на {n} коммитов"],
  "as of the last fetch": "по последнему fetch",
  "the next or previous agent in the sidebar's order, the pinned ones first": "следующий или предыдущий агент в порядке боковой панели, закреплённые первыми",
};
