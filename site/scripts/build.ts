// Builds the site into dist/: public/ as it is, and each page in src/ rendered to HTML.
// Cloudflare runs it (`bun run build`) for every push, and serves dist/ (see wrangler.jsonc).
import { cp, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { Home } from "../src/home.tsx";
import { NotFound } from "../src/not-found.tsx";

const root = join(import.meta.dir, "..");
const dist = join(root, "dist");
const pages = { "index.html": Home, "404.html": NotFound };

export async function build() {
  await rm(dist, { recursive: true, force: true });
  await cp(join(root, "public"), dist, { recursive: true });
  for (const [path, Page] of Object.entries(pages)) {
    await writeFile(join(dist, path), `<!doctype html>${Page()}`);
  }
}

if (import.meta.main) {
  await build();
  console.log(`Built ${Object.keys(pages).length} pages into dist/`);
}
