// Builds the site and serves dist/ locally the way Cloudflare does in production: / is
// index.html, unknown paths get 404.html with a 404 status, and public/_headers sets the
// installer's content type. `bun --watch` restarts it when src/ changes, and it rebuilds itself
// when public/ does.
import { watch } from "node:fs";
import { join, normalize } from "node:path";

import { build } from "./scripts/build.ts";

const root = join(import.meta.dir, "dist");
const notFound = Bun.file(join(root, "404.html"));

// One build at a time, so a burst of changes doesn't copy over one being cleared.
let building = build();
await building;
watch(join(import.meta.dir, "public"), { recursive: true }, () => {
  building = building.then(build).catch(console.error);
});

const server = Bun.serve({
  hostname: process.env.HOST ?? "127.0.0.1",
  port: Number(process.env.PORT ?? 8788),
  async fetch(request) {
    await building;
    let path = normalize(decodeURIComponent(new URL(request.url).pathname));
    if (path.endsWith("/")) path += "index.html";
    const file = Bun.file(join(root, path));
    if (path.startsWith("/_") || !(await file.exists())) {
      return new Response(notFound, { status: 404 });
    }
    const headers = path.endsWith(".sh")
      ? { "Content-Type": "text/plain; charset=utf-8" }
      : undefined;
    return new Response(file, { headers });
  },
});

console.log(`Serving the site at ${server.url}`);
