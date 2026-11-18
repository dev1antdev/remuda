# remuda's design book

How the page looks and speaks, as rules for whoever changes it — an agent or a person. The values live in
`web/src/styles.css` and the shared pieces in `web/src/ui/`; this file names them and says how to use them. Read it
before changing anything a person sees. `web/src/lib/designDoc.test.ts` checks that every custom property and class
named here exists in the stylesheet, so a rename that forgets this file fails the tests.

remuda is a working tool open all day on a desk and a phone: quiet, dense, driven from the keyboard. Colour comes from
the theme and one accent; what needs the person — an agent asking, something blocked, work done — is the only thing
that stands out.

## Words

- Say what changes for the person, not how it works: "Alerts", not "webhook config"; "Restart Claude Code on its
  session…", not "restart the process".
- A button says exactly what it does, with a verb: "Commit staged", "Push without hooks", "Resume all", "Wake up". One
  that opens a dialog ends in "…": "Scan a folder…", "Commit anyway…".
- A toast says what came of it: "Committed 4728a00c: …", "Link copied". No "successfully", no "please".
- An error says what went wrong and what to do, in git's or herdr's own words when there are some. No apologies, no
  "something went wrong".
- A confirm names what is lost or rewritten, and its button repeats the act ("Discard", "Commit anyway"), never "OK".
- Sentence case everywhere. No exclamation marks, no emoji.
- Every time through `lib/time.ts`, every count through `plural` / `tn`. Strings are English in `t()` / `tn()`; the
  Russian lives in `lib/i18n/ru*.ts`, a counted word with its three forms. A rail label too long in a language gets a
  short one through `tc("rail", …)`.
- A name of a machine, folder or path goes through the screen-sharing mode, which can swap it for a neutral one.

## Colour

- Twenty themes, ten dark and ten light, each in Catppuccin's 26 named slots (`--ctp-base`, `--ctp-text`, …); Latte
  by day and Frappé by night unless the person picks others. One of fourteen named colours is the accent (peach
  unless picked).
- **Write with the semantic tokens, never a slot or a literal colour:**
  - surfaces: `--bg` (the page), `--bg-raised` (sidebar, heads, cards, dialogs), `--bg-field` (a text field's well),
    `--bg-active` (selected), `--bg-hover` (under the pointer), `--bg-quiet` / `--bg-quiet-hover` (a quiet button),
    `--bg-glass` over `--glass` for anything floating (menu, popover, tip, toast, a Select's list);
  - ink: `--fg` (body text), `--fg-dim` (secondary, metadata), `--fg-faint` (hints and placeholders only — below
    4.5:1 in most themes, so never carry meaning on it);
  - hairlines: `--line`, `--line-strong`, `--line-hover` — the ink at 9, 15 and 26 %, as quiet on a raised surface as on
    the base; never a solid grey border.
- **The accent** `--accent` marks the primary button, focus and the selected. Text on it is `--on-accent`, the ink
  that holds 4.5:1 for that colour in that theme; never a literal white.
- **Status colours keep one meaning each:** `--red` blocked, failed, dangerous, removed lines; `--green` done, added
  lines; `--amber` a note, an agent waiting on the person (`--on-amber` on it); `--blue` information, links,
  thinking; `--violet` another agent's words. Each has a soft fill (`--accent-soft`, `--green-soft`, `--amber-soft`,
  `--blue-soft`, `--red-soft`) for a badge or a tone, and some a ring (`--accent-ring`, `--green-ring`, `--amber-ring`,
  `--red-ring`). A status always carries a word or an icon too, never colour alone.
- A slot is used directly only where the colour is the meaning: code highlighting, the activity ribbon's kinds, the
  accent picker. A vendor's hue (`ui/brand.ts`) is never green or red — those mean done and blocked.
- A rule a light theme changes keys on `data-theme="light"`, never on one theme.

## Type

- The system UI font, no web fonts; numbers are tabular everywhere, so counters and columns never jump.
- Sizes only from the scale, never a literal px: `--fs-10`, `--fs-11` (badges, rail labels), `--fs-12` (rows,
  metadata — the most used), `--fs-13` (controls, fields, menu items), `--fs-13-5` (body), `--fs-15` (a dialog's or a
  panel's title), `--fs-18` (a place's title), `--fs-20` (rare big numbers). Each is multiplied by the person's text
  size (`--text-scale`), so a literal size would not follow it.
- Headings weigh 600 with `letter-spacing: -0.01em`; body is 1.45, an agent's Markdown (`.md`) 1.55.
- Code, commands, paths and hashes wear `--mono`: the system's monospace or the one the person picked, the same stack
  the terminal gets.

## Size, shape, depth, motion

- Controls: `--ctl-lg` 36, `--ctl` 32 (a button, a field, a Select), `--ctl-sm` 28, `--ctl-xs` 24. They grow with the
  text size and an eighth under a finger (`--ctl-touch`); never set a control's height in px.
- Density: `--row-h`, `--row-pad-y`, `--tab-h`, `--tile-pad-y`, `--task-row-h`, `--gap-row`. Compact tightens them, with
  a mouse on a screen wider than 920 px only. There is no spacing scale: take a neighbour's padding rather than invent
  one.
- Radii only from the scale: `--r-xs` 5 (badges, keys), `--r-sm` 7 (menu items, rows), `--r-md` 10 (buttons, fields —
  the default), `--r-lg` 13 (cards, menus, popovers, the composer), `--r-xl` 18 (dialogs).
- Depth: `--shadow-sm` (a filled button), `--shadow-md` (anything floating), `--shadow-lg` (dialogs, drawers);
  `--inset-top` is the sheen on a filled button, `--inset-well` a recessed well. Layers climb `--z-drawer` <
  `--z-dialog` < `--z-menu` < `--z-select`.
- Motion: `--ease` with `--fast` (120 ms, hover and press) or `--base` (180 ms, appearing). A floating surface grows
  out of the button it hangs from; a toast slides in from its corner. `prefers-reduced-motion` stills it all.

## Layout

- **The rail** down the left: the five modes, the places, then the tools and Settings under a hairline; each an icon
  on a pill with its name under it, whole. At 920 px and below it lies along the bottom.
- **A pane's header** is `[burger][title — takes the slack][usage pill][lens switch][pop-out][search]`. The lens
  buttons keep equal room; a lens that cannot open is disabled, not removed; a shell keeps the switch's room unseen
  (`.lens-switch.is-vacant`).
- **Nothing in a bar moves when state changes.** Reserve the room a count or a second button will need (the Changes
  count keeps it while empty, `.lens-badge.empty`). Transient status — offline, a new version, "Copied", an error — is
  a toast in the bottom corner, never a pill in a bar.
- The tab strip has one tools slot: the lens's own control and the **View** menu. Never a row of icons in the header.
- Lenses stay mounted once shown; only the one on screen polls. The chat is one centred column of at most 880 px.

## Components

- **Every `<button>` carries exactly one family class:** `.btn` (quiet), `.btn.primary` (the one filled button of a
  surface), `.btn.danger`, `.btn.ghost`, `.icon-btn` (with `.sm` / `.lg`), or `.linkish`. A toggled one wears `.on`;
  so do a selected sidebar row, tab and split pane — never `.sel`, which is the Select's own class.
- A button is styled by its family, not by where it sits. The only context rules are components whose children are
  the component: the segmented controls (`.lens-switch`, `.fv-tabs`), a row's menu (`.row-menu`), the key bar
  (`.keybar .key`).
- **An icon-only button says what it does through `Tip` and an `aria-label`**, never a `title` alone.
- **A set of actions is a `Menu` with words** (`.menu`, `.menu-item`: an icon, the words, a hint, a shortcut), not a
  row of icons. A panel behind a button is a `Popover` (`.popover`), a tooltip a `Tip` (`.tip`).
- **Fields** share one rule — every input, textarea and `.sel`, with `.sm` and `.bare`: the `--bg-field` well, a
  `--line-strong` border, `--r-md`, `--ctl` tall, `--fs-13`; focus is an `--accent` border and a 3 px `--accent-soft`
  ring; placeholders are `--fg-faint`. Every native `<select>` is the `Select` from `ui/Select.tsx`.
- Rows share a radius, a hover and a focus ring. Cards are `.card`, counts `.badge` (`.quiet`), state dots
  `.dot.dot-<status>` and `.dot.machine-state.is-<state>`.
- A place with nothing to show says so through `Empty` (`.empty-state`: an icon on a soft tile, one line, a hint and
  a button where they help). A loading shape wears the one shimmer of `.bone`.
- `Dialog` is the only modal: one scrim, a head with icon, title, controls and ×, Escape, focus in and back out,
  `#root` inert while open. `FormDialog` and `ConfirmDialog` build on it. A note in place is a `Notice` (tones error,
  success, warn, info, hint); a passing word is a toast.
- Reuse a rule before writing one.

## Icons and marks

- UI icons are `ui/icons.tsx`: 24×24 stroke paths at 1.75, round caps and joins, `currentColor`. Size 12 in badges and
  small buttons, 14 beside text, 16 in headers and large buttons. Never an emoji or a font glyph as an icon.
- File and folder icons are Material Icon Theme's SVGs through `FileIcon` / `FolderIcon`, picked as VS Code picks
  them, a light variant in a light theme.
- An agent's mark is `Logo` in its vendor's hue (`ui/marks.ts`, `ui/brand.ts`); the marks are third parties'
  (`web/marks/NOTICE.md`) and imply no affiliation.
- remuda's own mark is the app icon: an orange square with cut corners on near black (`web/public/icons/`).

## Access

- Text is `--fg` or `--fg-dim` on its surface (4.5:1 in every theme); on a coloured fill, `--on-accent` or `--on-amber`.
- Focus is always visible. Menus walk with the arrows, Escape closes, a dialog takes the focus and gives it back.
- Under a finger every control grows and what a mouse reveals on hover stays visible.
- `prefers-reduced-motion` is honoured; a status never rests on colour alone.
