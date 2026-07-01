#!/usr/bin/env node
/**
 * Capture README screenshots from a running local instance.
 *
 * Usage:
 *   APP_URL=http://localhost:8080 \
 *   SCREENSHOT_USER=youruser SCREENSHOT_PASSWORD=yourpass \
 *   SCREENSHOT_GAME_ID=820 \
 *   node scripts/capture-screenshots.mjs
 */

import { chromium } from "playwright";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const baseURL = (process.env.APP_URL || "http://localhost:8080").replace(/\/$/, "");
const username = process.env.SCREENSHOT_USER;
const password = process.env.SCREENSHOT_PASSWORD;
const gameID = process.env.SCREENSHOT_GAME_ID;

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.join(__dirname, "..", "docs", "screenshots");

const view = { viewport: { width: 1280, height: 800 }, colorScheme: "dark" };

async function capture(page, name) {
  const file = path.join(outDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: false });
  console.log(`wrote ${file}`);
}

async function waitForApp(page) {
  await page.waitForLoadState("domcontentloaded");
  await page.waitForTimeout(500);
}

async function main() {
  await mkdir(outDir, { recursive: true });

  const browser = await chromium.launch();

  const loginContext = await browser.newContext(view);
  const loginPage = await loginContext.newPage();
  await loginPage.goto(`${baseURL}/`);
  await waitForApp(loginPage);
  await capture(loginPage, "login");
  await loginContext.close();

  if (!username || !password) {
    console.log("Set SCREENSHOT_USER and SCREENSHOT_PASSWORD to capture authenticated pages.");
    await browser.close();
    return;
  }

  const context = await browser.newContext(view);
  const loginResp = await context.request.post(`${baseURL}/login`, {
    form: { username, password },
    maxRedirects: 0,
  });
  if (loginResp.status() !== 302) {
    throw new Error(`login failed: HTTP ${loginResp.status()}`);
  }

  const cookies = await context.cookies();
  await context.clearCookies();
  await context.addCookies(
    cookies.map((c) => ({
      name: c.name,
      value: c.value,
      domain: c.domain,
      path: c.path,
      expires: c.expires,
      httpOnly: c.httpOnly,
      secure: false,
      sameSite: "Lax",
    })),
  );

  const page = await context.newPage();

  await page.goto(`${baseURL}/dashboard`);
  await waitForApp(page);
  await capture(page, "dashboard");

  await page.goto(`${baseURL}/library`);
  await waitForApp(page);
  await capture(page, "library");

  if (gameID) {
    await page.goto(`${baseURL}/library/games/${gameID}`);
    await waitForApp(page);
    await capture(page, "game-detail");
  }

  await browser.close();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
