# rembox site

[rembox.org](https://rembox.org): one page of docs, and `public/install.sh`, the installer.

- `bun install`, then `bun run dev` serves it at http://127.0.0.1:8788.
- `bun run check` and `bun run test` (Playwright) check it.
- Cloudflare Workers Builds deploys it on every push to main (see `wrangler.jsonc`), with `site/` as the root directory.
