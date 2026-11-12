# rembox

- `cli/`: Go CLI; run `make lint test` there.
- `site/public/`: rembox.org, plain HTML (Caddy).
- `install.sh`, `cli/Makefile` and `cli/internal/release` share asset names.
- Store local files under `config.Dir()`, so `implode` removes them.
- Every PR bumps `cli/VERSION` (merging releases it) and updates the site as needed.
