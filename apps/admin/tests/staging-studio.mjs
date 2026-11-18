// Run with ZIDI_STAGING_LOGIN_FILE pointing at a private email/password file.
// Creates an unbound QA journey and archives it after publication.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { chromium } from "playwright";

const admin = "https://zidicommerce-staging-admin-staging.up.railway.app";
const credentials = readFileSync(process.env.ZIDI_STAGING_LOGIN_FILE, "utf8");
const field = (name) =>
  credentials
    .match(new RegExp("^" + name + "\\s*[:=]\\s*(.+)$", "im"))?.[1]
    .trim();
const browser = await chromium.launch({ channel: "chrome" });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.setDefaultTimeout(15000);
const button = (name) => page.getByRole("button", { name, exact: true });
const tab = (name) => page.getByRole("tab", { name, exact: true });
const name = "Studio QA " + new Date().toISOString();
const created = await (async () => {
  try {
    await page.goto(admin + "/login");
    await page.getByLabel("Email", { exact: true }).fill(field("email"));
    await page.getByLabel("Password", { exact: true }).fill(field("password"));
    await button("Sign in").click();
    await page.waitForURL((url) => url.pathname !== "/login");
    await page.goto(admin + "/journeys");
    await button("New journey").click();
    await page.getByLabel("Journey name").fill(name);
    if (process.env.ZIDI_STUDIO_BLANK_TEST === "1") {
      await page.getByLabel("Search templates").fill("Blank journey");
      await page.getByRole("button",{name:/Blank journey/}).click();
    }
    const modern = await button("Use template").count();
    if (modern && process.env.ZIDI_STUDIO_BLANK_TEST !== "1") {
      await page.getByLabel("Search templates").fill("custom");
      await page.getByRole("button", { name: /Custom journey/ }).click();
      await page.locator(".template-preview li").first().waitFor();
    } else if (!modern) await page.getByRole("radio", { name: /Custom journey/ }).check();
    const [response] = await Promise.all([
      page.waitForResponse(
        (r) =>
          r.request().method() === "POST" &&
          new URL(r.url()).pathname.endsWith("/journeys"),
      ),
      button(modern ? "Use template" : "Create journey").click(),
    ]);
    assert.ok(response.ok(), "Journey creation failed");
    return (await response.json()).data;
  } catch (error) {
    await browser.close();
    throw error;
  }
})();
try {
  if (process.env.ZIDI_STUDIO_BLANK_TEST === "1") {
    await tab("Steps").click();
    await page.getByLabel("Search node library").fill("message");
    await page.locator(".node-library").getByRole("button",{name:"Message",exact:true}).click();
    await page.getByLabel("Customer message",{exact:true}).fill("Welcome to the blank journey test.");
    const [write]=await Promise.all([
      page.waitForResponse(r=>r.request().method()==="PUT" && r.url().includes("/journey-versions/")),
      button("Save draft").click(),
    ]);
    const definition=(await write.json()).data.definition;
    assert.equal(definition.steps[0].key,definition.start);
    assert.equal(definition.steps[0].next,definition.steps[1].key);
    console.log("Blank flow component saved a reachable starting message and end");
  }
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await tab("Canvas").click();
    await button("Zoom in").click();
    await button("Fit workflow to view").click();
    await page.locator(".flow-node").first().focus();
    await page.keyboard.press("Enter");
    await page
      .getByLabel("Customer message", { exact: true })
      .waitFor({ state: "visible" });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
      "Horizontal overflow at " + width,
    );
    console.log(
      "Canvas, focus, node inspector and viewport passed at " + width,
    );
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  if (process.env.ZIDI_STUDIO_BLANK_TEST !== "1" && await button("Select multiple").count()) {
    await tab("Canvas").click();
    await button("Select multiple").click();
    await page.locator(".flow-node").nth(1).click();
    await button("Copy 2").click();
    const before = await page.locator(".flow-node").count();
    await button("Paste").click();
    assert.equal(await page.locator(".flow-node").count(), before + 2);
    await button("Undo").click();
    assert.equal(await page.locator(".flow-node").count(), before);
    await button("Select multiple").click();
    await page.locator(".flow-node").first().click();
    await button("Toggle minimap").click();
    await page.getByRole("img", { name: "Workflow minimap" }).waitFor();
    await button("Connect next").click();
    await page.locator(".flow-node").nth(1).click();
    await button("Undo").click();
    console.log(
      "Multiple selection, paste, undo, connection and minimap passed",
    );
  }
  if (await button("Zidi Architect").count()) {
    await button("Zidi Architect").click();
    await page.getByRole("dialog", {name:"Zidi Architect"}).waitFor();
    await page.keyboard.press("Escape");
    await tab("Steps").click();
    await page.getByLabel("Search node library").fill("message");
    const count = await page.locator(".flow-node").count();
    await page.locator(".node-library").getByRole("button",{name:"Message",exact:true}).click();
    assert.equal(await page.locator(".flow-node").count(),count+1);
    await button("Undo").click();
    await page.locator(".flow-node").first().click();
  }
  const copy = "Hello from staging Studio QA.";
  await page.getByLabel("Customer message", { exact: true }).fill(copy);
  await tab("Canvas").click();
  await button("Auto-layout workflow").click();
  const [save] = await Promise.all([
    page.waitForResponse(
      (r) =>
        r.request().method() === "PUT" &&
        r.url().includes("/journey-versions/"),
    ),
    button("Save draft").click(),
  ]);
  assert.ok(save.ok());
  const saved = (await save.json()).data;
  assert.equal(saved.definition.steps[0].text, copy);
  assert.equal(typeof saved.definition.steps[0].position.x, "number");
  await button("Validate").click();
  await page.getByText("Validation passed.", { exact: true }).waitFor();
  await tab("Preview").click();
  if(await page.getByLabel("Preview device").count()) {
    await page.getByLabel("Preview device").selectOption("desktop");
    assert.equal(await page.locator(".flow-canvas").isVisible(),false);
    await page.getByLabel("Preview device").selectOption("phone");
  }
  await button("Start preview").click();
  await page
    .getByRole("region", { name: "Interactive journey preview" })
    .getByText(copy, { exact: true })
    .waitFor();
  const answer = page.getByLabel("Customer answer");
  if (await answer.isVisible()) {
    await answer.fill("Test request");
    await button("Send preview answer").click();
  }
  await button("Publish").click();
  const [publish] = await Promise.all([
    page.waitForResponse(
      (r) => r.request().method() === "POST" && r.url().endsWith("/publish"),
    ),
    button("Confirm").click(),
  ]);
  assert.ok(publish.ok(), "Publish failed");
  const published = (await publish.json()).data;
  assert.equal(published.binding.version_id, published.version.id);
  assert.equal(published.version.status, "published");
  assert.ok(!published.definition.connection_id);
  console.log(
    "Created, saved, validated, previewed and published unbound staging journey",
    published.journey.id,
    published.version.id,
  );
  await button("New draft").click();
  await button("Publish").click();
  const [republish] = await Promise.all([
    page.waitForResponse(r=>r.request().method()==="POST" && r.url().endsWith("/publish")),
    button("Confirm").click(),
  ]);
  assert.ok(republish.ok());
  const second=(await republish.json()).data;
  assert.ok(second.version.version_number > published.version.version_number);
  await button("Version history").click();
  await button("Roll back").click();
  const [restore] = await Promise.all([
    page.waitForResponse(r=>r.request().method()==="POST" && r.url().endsWith("/activate")),
    button("Confirm").click(),
  ]);
  assert.ok(restore.ok());
  const restored=(await restore.json()).data;
  assert.equal(restored.binding.version_id,published.version.id);
  console.log("Version history restored published version",restored.binding.version_id);
} finally {
  try {
    if (await button("Journey settings").isVisible()) {
      await button("Journey settings").click();
      await button("Archive journey").click();
      const [archive] = await Promise.all([
        page.waitForResponse(
          (r) =>
            r.request().method() === "DELETE" &&
            r.url().endsWith("/journeys/" + created.journey.id),
        ),
        button("Confirm").click(),
      ]);
      assert.ok(archive.ok());
      console.log("Archived QA journey", created.journey.id);
    }
  } finally {
    await browser.close();
  }
}
