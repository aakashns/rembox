# rembox

- `cli/`: Go CLI. Run `make lint test` there.
- `site/`: rembox.org (Bun). Run `bun run check && bun run test` there.
- Asset names (`rembox-<os>-<arch>`, `SHA256SUMS`) are shared by `site/public/install.sh`, `cli/Makefile` and `cli/internal/release`; change them together.
- Every PR bumps `cli/VERSION`; merging to main releases it.
