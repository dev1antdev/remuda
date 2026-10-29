# remuda for the Mac

A thin native shell around remuda's page: one window with its own Dock
icon, a menu bar item that says who needs you, native notifications with
the same buttons the phone gets, a global hotkey. The page inside is the
same one the browser shows — this app adds the parts a browser tab cannot
be: a place in ⌘-Tab, the menu bar, the Dock badge, notifications that
come from macOS rather than from a browser vendor's push service.

## Build

Swift 6 from the Command Line Tools is enough; no Xcode, no dependency.

```sh
make mac            # → bin/remuda.app, ad-hoc signed
open bin/remuda.app
```

`macos/build.sh` runs `swift build -c release`, lays the bundle out, makes
the icon from `web/public/icons/icon-512.png` and signs the app with the
ad-hoc identity, so Gatekeeper runs it from here. An app downloaded from
somewhere else is unsigned in Gatekeeper's eyes: right-click → Open the
first time, or build it yourself.

## First launch

The app signs in by itself on the Mac that runs remuda: it reads the
token remuda keeps in `~/Library/Application Support/remuda/token` and
opens the page with it once; the server turns that into a device cookie
(Settings › Devices lists it as "remuda on macOS") and the web view keeps
the cookie between launches. A remuda elsewhere — a tailnet address, say
— is paired in Settings… by pasting its pairing link
(`https://host/?token=…`); the address goes to the app's defaults, the
token to the login keychain under "remuda".

macOS asks once whether the app may notify you; Settings… shows the
answer and asks again when it was never answered.

## What it does

- **The menu bar item**: the mark with a dot in the colour of the most
  pressing agent — red for a question, amber for a watchdog note, green
  for a finish nobody looked at — and the count beside it. The menu lists
  those agents with the first line of what they said; a click opens the
  pane's chat in the window. Then "N working · M idle", Open remuda
  (⌥⌘R), Wall, Tasks, Standup, Settings…, Quit.
- **The Dock badge** carries the same count.
- **Notifications** are the alerts the phone would get — a question after
  10 s, a finish after a minute, a watchdog note, a digest, a budget — with
  their buttons: *Open*, *Send Esc*, the question's own two choices. The
  app is one more device of remuda's push service (`POST /api/push/socket`,
  then `/ws/alerts`), so the alert settings and quiet hours in the page's
  alerts panel apply to it like to a phone.
- **The hotkey** ⌥⌘R shows and hides the window from any app; another
  combination goes into Settings… as `ctrl+opt+r` or `⌃⌥R`.
- **Closing the window** hides it; the app stays in the menu bar. Quit is
  in the menu.
- A file the page saves (Export as Markdown) lands in `~/Downloads`.
  Links to other sites open in your browser.

## Not here

- The pop-out window (Chrome's Document Picture-in-Picture API): its
  buttons do not appear in the app.
- Dictation in the composer: WebKit outside Safari has no
  `SpeechRecognition`.
- Web Push: replaced by the native notifications above.

## The probe

```sh
bin/remuda.app/Contents/MacOS/remuda --probe [--server http://127.0.0.1:7526] [--token-file PATH] [--seconds 15]
```

No window, no menu bar item, no notification: the page is loaded off
screen and every message it posts over the bridge, every alert frame and
every change of the socket's state is printed as one JSON line. Exit 0
when the page said `ready` within the time, 1 otherwise. The check behind
`make mac` in a pull request.

## How the page and the app talk

The app injects `window.remudaShell = {native: true, version}` before the
page runs and listens on the script message handler `remuda`. The page
posts `{type: "ready"}` once, `{type: "badge", count, tone}` and
`{type: "menu", rows, working, idle}` whenever they change; the app calls
`remudaShell.select(machine, pane)`, `remudaShell.openPanel(name)` and
`remudaShell.openMode(mode)`, which the page defines. The sources are one
file per piece under `Sources/Remuda/`: `WebWindow` (the window and the
web view), `StatusItem`, `Badge`, `AlertSocket` (the device and its
socket), `Notifier` (the notifications and their buttons), `Hotkey`,
`Settings`, `Probe`, `Server` (the address, the token, the keychain),
`Bridge` (the message types), `MainMenu`, `AppDelegate`.
