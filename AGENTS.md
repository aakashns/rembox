# rembox

- `cli/`: Go CLI; run `make lint test` there.
- `site/public/`: rembox.aakashns.com, plain HTML (Caddy). Keep it short: a line or two per
  command or section, no walls of text; details belong in `rembox help`. Run every change to
  its wording by the user first, and let them edit it.
- To show the site, run it with live reload: `npx live-server site/public --port=8788`.
- `rembox help setup` lists setup's steps in the site's words; a test checks they match.
- `install.sh`, `cli/Makefile` and `cli/internal/release` share asset names.
- Store local files under `config.Dir()`, so `implode` removes them.
- Run external commands with `App.run`, which prints each one before running it, or
  `App.runConfirmed` for sensitive or destructive ones, which also asks first.
- Set tools up their default way, with their own installers and flows, as far as possible; avoid
  custom logic around them.
- `rembox setup` must be idempotent: every step in `cli/internal/remote` checks the server's
  state and changes only what needs changing, so running it again is harmless.
- README.md lists the server scripts in the order they run; keep it current when adding one.
- The FAQ "What does rembox install and set up on the server?" lists what `rembox setup` installs; update it when adding or removing a package.
- Every PR bumps `cli/VERSION` (merging releases it) and updates the site as needed.
