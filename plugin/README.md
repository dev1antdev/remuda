# herdr-ui for JetBrains IDEs

A tool window with herdr-ui inside PhpStorm / GoLand / IDEA — your agents'
chat, terminals, approvals and worktrees next to the code, with the file
paths an agent mentions opening in the editor. Built for **Remote
Development**: the plugin runs on the backend, on the same host as herdr.

## What it does

- **herdr** tool window (right side): the embedded herdr-ui, signed in
  automatically, opened on the pane whose folder is the project.
- A path the agent prints (terminal or chat) → opens in the IDE editor at
  that line. Files on other machines still open in herdr-ui's viewer.
- **Ask Agent About This File** (editor / project view context menu,
  `Ctrl+Shift+H`): puts `@path` into the chat composer — with the selected
  lines quoted when there is a selection.
- **Tools → Open herdr-ui in Browser**: the same page in a real browser;
  in Remote Development, Gateway forwards the port.
- Starts the `herdr-ui` binary itself if nothing answers on `127.0.0.1:7317`.

## Install (Remote Development)

On the **remote host** (where herdr and the IDE backend run):

1. Put `herdr-ui` next to herdr — from this repo, `make deploy HOST=<ssh host>`
   installs the right build into `~/.local/bin/herdr-ui`.
2. Enable the backend's embedded browser, which is off by default in
   Remote Development: add to the login-shell environment, e.g. `~/.zshenv`
   on macOS or `~/.profile` on Linux:
   ```sh
   export REMOTE_DEV_SERVER_JCEF_ENABLED=true
   ```
   and restart the backend (close the project in Gateway, reconnect).
3. Build the plugin (`make plugin` here → `plugin/build/distributions/*.zip`)
   and copy the zip to the remote host.

In the IDE (the Gateway client): **Settings → Plugins → ⚙ → Install Plugin
from Disk…** — the file chooser browses the backend's filesystem; pick the
zip, restart. The **herdr** tool window appears on the right.

Without step 2 the tool window explains the situation and offers **Open in
browser**, which still works through port forwarding; everything else
(starting herdr-ui, the browser link) is the same.

## Environment

| variable | meaning |
| --- | --- |
| `HERDR_UI_BIN` | path to the binary when it isn't on PATH / in `~/.local/bin` |
| `HERDR_UI_PORT` | port herdr-ui listens on (default 7317) |

The plugin's herdr-ui log is `$TMPDIR/herdr-ui-plugin.log` on the host.

## Development

```sh
cd plugin
./gradlew buildPlugin     # → build/distributions/herdr-ui-jetbrains-<v>.zip
./gradlew runIde          # a sandbox IntelliJ Community with the plugin
```

Platform APIs only (`com.intellij.modules.platform`), compiled against
IntelliJ 2025.1, no upper build bound.
