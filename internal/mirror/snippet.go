package mirror

// Snippet is what a person adds to herdr's config.toml for the sidebar to show remuda's tokens: herdr draws a $name
// only where a row names it, a row whose tokens all have no value disappears, and the rest of the rows are herdr's
// defaults. Each row fits herdr's default sidebar of 26 columns. The test runs it through `herdr config check`.
const Snippet = `[ui.sidebar.agents]
rows = [
  ["state_icon", "machine", "workspace", "tab"],
  ["agent", "state_text"],
  ["$remuda_model", "$remuda_cost", "$remuda_context"],
  ["$remuda_pr", "$remuda_task"],
  [{ token = "$remuda_alert", fg = "#e78284", bold = true }],
  [{ token = "$remuda_said", dim = true }],
]

[ui.sidebar.spaces]
rows = [
  ["state_icon", "workspace", "$remuda_cost"],
  ["branch", "git_status", "$remuda_pr"],
  ["$remuda_review"],
]
`

// ToastSnippet is herdr's own switch for toasts, off by default: without it herdr answers every toast "disabled".
const ToastSnippet = `[ui.toast]
delivery = "herdr"
`
