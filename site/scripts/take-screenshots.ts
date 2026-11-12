// Saves screenshots of the built site to screenshots/ (ignored by git), for pull requests:
// the whole home page on desktop and on a phone. Run `bun run build` first.
import { chromium, devices } from "@playwright/test";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";

const root = join(import.meta.dir, "..");
const output = join(root, "screenshots");
await mkdir(output, { recursive: true });

// Serves dist/ on a free port, as server.ts does, without rebuilding or watching.
const server = Bun.serve({
  port: 0,
  async fetch(request) {
    let path = new URL(request.url).pathname;
    if (path.endsWith("/")) path += "index.html";
    const file = Bun.file(join(root, "dist", path));
    return (await file.exists())
      ? new Response(file)
      : new Response(Bun.file(join(root, "dist/404.html")), { status: 404 });
  },
});

const browser = await chromium.launch();
try {
  const sizes = {
    desktop: { viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 },
    mobile: devices["iPhone 13"],
  };
  for (const [name, options] of Object.entries(sizes)) {
    const context = await browser.newContext(options);
    const page = await context.newPage();
    await page.goto(server.url.href);
    await page.screenshot({
      path: join(output, `home-${name}.png`),
      fullPage: true,
    });
    await context.close();
  }
  console.log(`Saved screenshots to ${output}`);
} finally {
  await browser.close();
  server.stop();
}
