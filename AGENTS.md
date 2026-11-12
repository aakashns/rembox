# rembox

- `cli/`: Go CLI; run `make lint test` there.
- `site/public/`: rembox.aakashns.com, plain HTML (Caddy). Keep it short: a line or two per
  command or section, no walls of text; details belong in `rembox help`.
- `install.sh`, `cli/Makefile` and `cli/internal/release` share asset names.
- Store local files under `config.Dir()`, so `implode` removes them.
- Run external commands with `App.run`, which prints each one before running it, or
  `App.runConfirmed` for sensitive or destructive ones, which also asks first.
- Every PR bumps `cli/VERSION` (merging releases it) and updates the site as needed.
