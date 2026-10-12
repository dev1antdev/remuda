# Security

remuda is a single-user tool: one bearer token is the only credential, and
whoever holds it can do everything the person can — type into their
agents, read their files, run updates on their machines. Keep it on
loopback, a tailnet or an SSH tunnel; never on a public interface.

Found a way around that? Please report it privately through GitHub's
security advisories for this repository ("Report a vulnerability" under
the Security tab) rather than in a public issue. Expect an answer within a
few days. There is no bounty programme.

What the server sends anywhere by itself: version lookups to the npm
registry and GitHub's releases API every six hours (off with
`-no-version-check`), Web Push through your browser vendor's push service
when you enable notifications, and nothing else.

Two things to know about the trust boundaries:

- The machines you add are trusted as much as this one. The remuda you
  open holds every peer's token (read over your SSH, kept in memory) to
  relay their terminals, and the Code mode runs a machine's code-server
  inside this page's origin: whatever runs in that editor — extensions,
  a repository's workspace settings — can reach this remuda's API as you.
  Open the Code mode only on machines you trust with everything here.
- Anything running as your user on this machine (every local agent) can
  read the token file in the state dir. The protections in the browser
  are against remote agents, repositories and pages — a local agent is
  already you.
