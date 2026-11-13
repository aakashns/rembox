# rembox

- `cli/`: Go CLI; run `make lint test` there.
- `site/public/`: rembox.aakashns.com, plain HTML (Caddy). Keep it short: a line or two per
  command or section, no walls of text; details belong in `rembox help`.
- `install.sh`, `cli/Makefile` and `cli/internal/release` share asset names.
- Store local files under `config.Dir()`, so `implode` removes them.
- Run external commands with `App.run`, which prints each one before running it, or
  `App.runConfirmed` for sensitive or destructive ones, which also asks first.
- `rembox setup` must be idempotent: every step in `cli/internal/remote` checks the server's
  state and changes only what needs changing, so running it again is harmless.
- `plan.md` is the roadmap: what's done, what's next. Keep it current in every PR.
- Every PR bumps `cli/VERSION` (merging releases it) and updates the site as needed.
