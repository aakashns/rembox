import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

const install = "curl -fsSL https://rembox.org/install.sh | sh";

test("home page, links, and responsive layout", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "Remote dev boxes, set up securely",
  );
  await expect(page.getByRole("link", { name: "rembox home" })).toBeVisible();
  await expect(page.locator("#install-command")).toHaveText(install);
  await expect(page.locator("#install-command")).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  // Each command `rembox help` lists is documented.
  for (const command of ["help", "version", "upgrade"]) {
    await expect(
      page.locator("dl.commands dt", { hasText: `rembox ${command}` }),
    ).toBeVisible();
  }
  // Links that leave the site open in a new tab; links within it don't.
  for (const link of await page.locator("a").all()) {
    const href = await link.getAttribute("href");
    const target =
      href?.startsWith("/") || href?.startsWith("#") ? null : "_blank";
    expect(await link.getAttribute("target"), href!).toBe(target);
  }
  expect(errors).toEqual([]);
});

test("copy succeeds, or gives a selectable fallback when access is denied", async ({
  page,
  context,
  browserName,
}) => {
  test.skip(browserName !== "chromium", "clipboard permissions are Chromium's");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  await page.getByRole("button", { name: "Copy install command" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    install,
  );
  await expect(page.getByRole("status")).toContainText("copied");
  await page.evaluate(() => {
    navigator.clipboard.writeText = async () => {
      throw new Error("Permission denied");
    };
  });
  await page.getByRole("button", { name: "Copy install command" }).click();
  expect(await page.evaluate(() => getSelection()?.toString())).toBe(install);
  await expect(page.getByRole("status")).toContainText("highlighted");
});

test("works without JavaScript", async ({ browser, baseURL }) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  await page.goto(baseURL!);
  await expect(page.locator("#install-command")).toHaveText(install);
  await expect(page.locator("#copy")).toBeHidden();
  await context.close();
});

test("serves the installer as plain text", async ({ request }) => {
  const response = await request.get("/install.sh");
  expect(response.status()).toBe(200);
  expect(response.headers()["content-type"]).toContain("text/plain");
  expect(await response.text()).toBe(
    await readFile(new URL("../public/install.sh", import.meta.url), "utf8"),
  );
});

test("unknown pages get the 404 page", async ({ page }) => {
  const response = await page.goto("/nope");
  expect(response?.status()).toBe(404);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "Page not found",
  );
});
