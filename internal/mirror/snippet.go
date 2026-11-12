package mirror

// Snippet is what a person adds to herdr's config.toml for the sidebar to show remuda's tokens: herdr draws a $name
// only where a row names it, and a row whose tokens all have no value disappears. The other rows are herdr's defaults,
// state_text beside the agent so the watchdog's note shows. Each row fits herdr's default sidebar of 26 columns. It
// takes the place of the person's own [ui.sidebar.agents] and [ui.sidebar.spaces]: herdr reads a config with a table
// written twice as no config at all. The test runs it through `herdr config check`.
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

// ToastSnippet is herdr's own switch for toasts, off by default: without it herdr answers every toast "disabled". A
// config with a [ui.toast] of its own takes the key there, not a second table.
const ToastSnippet = `[ui.toast]
delivery = "herdr"
`
