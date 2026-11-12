# rembox

- `cli/`: Go CLI. Run `make lint test` there.
- `site/public/`: rembox.org, plain HTML served by Caddy.
- Asset names (`rembox-<os>-<arch>`, `SHA256SUMS`) are shared by `install.sh`, `cli/Makefile` and `cli/internal/release`.
- Every PR bumps `cli/VERSION` (merging releases it) and updates the site's docs as needed.
