import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { chromium } from "playwright";
const root = "https://zidicommerce-staging-admin-staging.up.railway.app";
const login = readFileSync(process.env.ZIDI_STAGING_LOGIN_FILE, "utf8");
const field = (k) =>
  login.match(new RegExp("^" + k + "\\s*[:=]\\s*(.+)$", "im"))?.[1].trim();
const browser = await chromium.launch({ channel: "chrome" });
const page = await browser.newPage();
page.setDefaultTimeout(12000);
try {
  await page.goto(root + "/login");
  await page.getByLabel("Email", { exact: true }).fill(field("email"));
  await page.getByLabel("Password", { exact: true }).fill(field("password"));
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.waitForURL((u) => u.pathname !== "/login");
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(root);
    if (width > 1024) {
      await page.getByRole("button", { name: "Collapse navigation" }).click();
      await page.getByRole("button", { name: "Expand navigation" }).click();
    } else {
      await page.getByRole("button", { name: "Open navigation" }).click();
      await page.locator("aside.open a").first().waitFor();
      await page.keyboard.press("Escape");
    }
    await page.getByRole("button", { name: "Find a page" }).click();
    await page.getByLabel("Search navigation").pressSequentially("inventory");
    assert.equal(
      await page.getByLabel("Search navigation").inputValue(),
      "inventory",
    );
    await page
      .getByRole("dialog")
      .getByRole("link", { name: /Inventory/ })
      .click();
    await page.waitForURL("**/inventory");
    await page.getByRole("button", { name: "Conversation activity" }).click();
    await page.getByRole("link", { name: "Open conversations" }).click();
    await page.waitForURL("**/conversations");
    for (const path of [
      "/",
      "/stores",
      "/catalogue",
      "/inventory",
      "/customers",
      "/orders",
      "/conversations",
      "/knowledge",
      "/settings/whatsapp",
      "/assistant/ai-chat",
      "/journeys",
      "/team",
      "/advanced/audit-logs",
      "/settings/business",
      "/settings/payments",
    ]) {
      await page.goto(root + path, { waitUntil: "networkidle" });
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
        false,
        path + " overflows at " + width,
      );
      assert.ok(
        await page.locator("main h1, main h2").count(),
        path + " missing heading",
      );
    }
    console.log(
      "Search, activity, navigation and all 15 Admin pages passed at " + width,
    );
  }
  // Simulate a failed GET in this browser only, then exercise merchant recovery.
  let release, attempt=0;
  const delayed = new Promise(resolve => { release=resolve; });
  await page.route("**/v1/journeys", async route => {
    if (attempt++ === 0) {
      await delayed;
      await route.fulfill({status:503,contentType:"application/json",body:JSON.stringify({error:{message:"Unavailable"}})});
    } else {
      await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({data:[]})});
    }
  });
  await page.goto(root+"/journeys");
  await page.getByText("Loading journeys",{exact:true}).waitFor();
  release();
  await page.getByRole("button",{name:"Retry loading journeys"}).click();
  await page.getByText("No journeys yet",{exact:true}).waitFor();
  console.log("Studio loading, request error, retry and empty state passed using isolated browser responses.");
} finally {
  await browser.close();
}
