import assert from "node:assert/strict";
import { readFile, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

const base = process.env.JOURNEY_QA_ADMIN_URL ?? "http://127.0.0.1:3017";
const api = process.env.JOURNEY_QA_API_URL ?? "http://127.0.0.1:8087/v1";
for (const value of [base, api]) assert.ok(["localhost", "127.0.0.1"].includes(new URL(value).hostname), "E2E execution is local-only");
const fixture = JSON.parse(await readFile(process.env.JOURNEY_QA_MANIFEST ?? "/tmp/zidi-r1-qa-manifest.json", "utf8"));
const artifacts = process.env.JOURNEY_QA_ARTIFACT_DIR ?? "/tmp/zidi-r1-browser";
await mkdir(artifacts, { recursive: true });
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE_PATH ?? "playwright");
const browser = await chromium.launch({ headless: true });
const results = [];
let token = "";
async function request(method, route, body, expected = 200) {
  const response = await fetch(`${api}${route}`, { method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) }, ...(body ? { body: JSON.stringify(body) } : {}) });
  const data = await response.json();
  assert.equal(response.status, expected, `${method} ${route}: ${JSON.stringify(data)}`);
  return data.data;
}
token = (await request("POST", "/auth/login", { email: fixture.owner_email, password: fixture.password })).access_token;
const initialOrders = await request("GET", "/orders");
const beforeOrders = initialOrders.length;
const orderIdsBefore = new Set(initialOrders.map(order => order.id));
const name = `Journey browser QA ${Date.now()}`;
async function check(label, work) {
  const start = Date.now();
  try { await work(); results.push({ test: label, status: "passed", milliseconds: Date.now() - start }); console.log(`PASS ${label}`); }
  catch (error) { results.push({ test: label, status: "failed", error: error.message }); throw error; }
}
async function login(page, email) {
  await page.goto(`${base}/login`);
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Password", { exact: true }).fill(fixture.password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.waitForURL(`${base}/`);
  await page.goto(`${base}/journeys`);
}
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage();
const errors = [];
page.on("pageerror", error => errors.push(error.message));
const editor = page.getByRole("region", { name: "Step editor" });
const preview = page.getByRole("region", { name: "Interactive journey preview" });
// Sections may not have an implicit region in every engine; labelled sections
// are selected by stable, owned markup rather than mocked API responses.
const edit = page.locator(".journey-editor");
const demo = page.locator(".journey-preview");
let journey;
try {
  await login(page, fixture.owner_email);
  await check("Create ordering template through actual API", async () => {
    await page.getByRole("button", { name: "New journey", exact: true }).click();
    await page.getByLabel("Journey name", { exact: true }).fill(name);
    await page.getByRole("button", { name: "Create journey", exact: true }).click();
    await page.getByTestId("journey-studio").waitFor();
    journey = (await request("GET", "/journeys")).find(row => row.name === name);
    assert.ok(journey);
  });
  await check("Edit wording, undo/redo, bind test channel and save", async () => {
    await edit.getByLabel("Customer message", { exact: true }).fill("Welcome to our locally tested merchant journey.");
    await page.getByRole("button", { name: "Undo", exact: true }).click();
    assert.match(await edit.getByLabel("Customer message", { exact: true }).inputValue(), /Welcome/);
    await page.getByRole("button", { name: "Redo", exact: true }).click();
    await page.getByRole("button", { name: "Journey settings", exact: true }).click();
    await page.getByLabel("Channel connection").selectOption(fixture.channel_id);
    await page.getByRole("button", { name: "Done", exact: true }).click();
    await page.getByRole("button", { name: "Save draft", exact: true }).click();
    await page.getByText("Draft saved.", { exact: true }).waitFor();
  });
  await check("Reorder safely without changing layout dimensions", async () => {
    const before = await page.getByTestId("journey-studio").boundingBox();
    await page.getByRole("button", { name: "Move Welcome down", exact: true }).click();
    const after = await page.getByTestId("journey-studio").boundingBox();
    assert.equal(before.width, after.width);
    await page.getByRole("button", { name: "Move Welcome up", exact: true }).click();
    // Native drag/drop is an additional path to the same local reorder action.
    await page.locator(".journey-steps li").first().dragTo(page.locator(".journey-steps li").nth(1));
    await page.getByRole("button", { name: "Undo", exact: true }).click();
  });
  await check("Edit options and branch destination", async () => {
    await page.locator(".journey-step-select").filter({ hasText: "Another item" }).click();
    await edit.getByLabel("Label", { exact: true }).nth(1).fill("Continue to checkout");
    await edit.getByLabel("Then go to", { exact: true }).nth(1).selectOption("fulfilment");
    await page.getByRole("button", { name: "Save draft", exact: true }).click();
    await page.getByText("Draft saved.", { exact: true }).waitFor();
  });
  await check("Interactive chat uses long live store/product names", async () => {
    await demo.getByRole("button", { name: "Start preview", exact: true }).click();
    await demo.getByRole("button", { name: fixture.store_name, exact: true }).click();
    await demo.getByRole("button", { name: "Drinks", exact: true }).click();
    await demo.getByRole("button", { name: fixture.product_name, exact: true }).click();
    await demo.getByRole("button", { name: /Regular Jasmine Milk Tea -/ }).click();
    await demo.getByLabel("Customer answer", { exact: true }).waitFor();
  });
  await check("Invalid quantity stays on the same step", async () => {
    await demo.getByLabel("Customer answer", { exact: true }).fill("-1");
    await demo.getByRole("button", { name: "Send preview answer", exact: true }).click();
    await demo.getByText("How many would you like?", { exact: true }).last().waitFor();
    assert.equal(await demo.locator(".journey-current").innerText(), "Quantity");
    await demo.getByLabel("Customer answer", { exact: true }).fill("2");
    await demo.getByRole("button", { name: "Send preview answer", exact: true }).click();
    await demo.getByRole("button", { name: "Continue to checkout", exact: true }).click();
    await demo.getByRole("button", { name: "Pickup", exact: true }).click();
    for (const answer of ["Ada QA", "+2348010000001", "ada@example.test"]) {
      await demo.getByLabel("Customer answer", { exact: true }).fill(answer);
      const response = page.waitForResponse(response => response.url().includes("/journey-previews/") && response.url().endsWith("/answer") && response.request().method() === "POST");
      await demo.getByRole("button", { name: "Send preview answer", exact: true }).click();
      assert.equal((await response).status(), 200);
    }
    await demo.getByRole("button", { name: "Place order", exact: true }).waitFor();
  });
  await check("Flow preview renders compiled screen; preview creates no orders", async () => {
    await demo.getByRole("button", { name: "Flow", exact: true }).click();
    await page.getByTestId("compiled-flow-screen").waitFor();
    assert.match(await page.getByTestId("compiled-flow-screen").getAttribute("data-screen"), /^S_/);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: path.join(artifacts, "desktop-flow.png") });
    assert.equal((await request("GET", "/orders")).length, beforeOrders);
    await demo.getByRole("button", { name: "Chat", exact: true }).click();
    await page.locator(".journey-step-select").filter({ hasText: /^Welcome/ }).click();
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: path.join(artifacts, "desktop-chat.png") });
  });
  await check("Publish immutable version and clone history", async () => {
    await page.getByRole("button", { name: "Validate", exact: true }).click();
    await page.getByText("Validation passed.", { exact: true }).waitFor();
    await page.getByRole("button", { name: "Publish", exact: true }).click();
    await page.getByRole("button", { name: "Confirm", exact: true }).click();
    await page.getByRole("button", { name: "New draft", exact: true }).waitFor();
    await page.getByRole("button", { name: "New draft", exact: true }).click();
    await page.getByRole("button", { name: "Save draft", exact: true }).waitFor();
    await page.getByRole("button", { name: "Version history", exact: true }).click();
    assert.ok((await page.getByRole("dialog").innerText()).includes("Version 2"));
    assert.ok((await page.getByRole("dialog").innerText()).includes("Version 1"));
    await page.getByRole("button", { name: "Close dialog", exact: true }).click();
  });
  await check("Desktop columns do not overlap or overflow", async () => {
    const boxes = await page.locator(".journey-studio > aside, .journey-studio > section").evaluateAll(nodes => nodes.map(node => { const r=node.getBoundingClientRect(); return { left:r.left,right:r.right,width:r.width }; }));
    assert.equal(boxes.length, 3);
    assert.ok(boxes[0].right <= boxes[1].left + 1 && boxes[1].right <= boxes[2].left + 1);
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    assert.deepEqual(errors, []);
  });
  await check("Invalid draft shows actionable validation errors", async () => {
    await edit.getByLabel("Next step", { exact: true }).selectOption("");
    await page.getByRole("button", { name: "Save draft", exact: true }).click();
    await page.getByText("Draft saved.", { exact: true }).waitFor();
    await page.getByRole("button", { name: "Validate", exact: true }).click();
    await page.locator(".journey-issues").waitFor();
    assert.ok((await page.locator(".journey-issues").innerText()).includes("Welcome"));
    await edit.getByLabel("Next step", { exact: true }).selectOption("store");
    await page.getByRole("button", { name: "Save draft", exact: true }).click();
    await page.getByText("Draft saved.", { exact: true }).waitFor();
  });
  await check("Explicit controlled test creates one local test order", async () => {
    await page.getByRole("button", { name: "Version history", exact: true }).click();
    await page.locator(".journey-version").filter({ hasText: "Version 1" }).getByRole("button", { name: "View", exact: true }).click();
    await demo.getByRole("button", { name: "Controlled record test", exact: true }).click();
    await page.getByRole("button", { name: "Confirm", exact: true }).click();
    await demo.getByText("Test records", { exact: true }).waitFor();
    await demo.getByRole("button", { name: fixture.store_name, exact: true }).click();
    await demo.getByRole("button", { name: "Drinks", exact: true }).click();
    await demo.getByRole("button", { name: fixture.product_name, exact: true }).click();
    await demo.getByRole("button", { name: /Regular Jasmine Milk Tea -/ }).click();
    await demo.getByLabel("Customer answer", { exact: true }).fill("1");
    await demo.getByRole("button", { name: "Send preview answer", exact: true }).click();
    await demo.getByRole("button", { name: "Continue to checkout", exact: true }).click();
    await demo.getByRole("button", { name: "Pickup", exact: true }).click();
    for (const answer of ["Test Customer", "+2348010000002", "test@example.test"]) {
      await demo.getByLabel("Customer answer", { exact: true }).fill(answer);
      const response = page.waitForResponse(response => response.url().includes("/journey-tests/") && response.url().endsWith("/answer") && response.request().method() === "POST");
      await demo.getByRole("button", { name: "Send preview answer", exact: true }).click();
      assert.equal((await response).status(), 200);
    }
    await demo.getByRole("button", { name: "Place order", exact: true }).click();
    await demo.getByRole("button", { name: "Check payment", exact: true }).waitFor();
    const orders = await request("GET", "/orders");
    assert.equal(orders.length, beforeOrders + 1);
    const created = orders.find(order => !orderIdsBefore.has(order.id));
    assert.equal(created.total_minor, 450000);
    assert.notEqual(created.status, "paid");
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: path.join(artifacts, "controlled-test.png") });
  });
  const mobile = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const phone = await mobile.newPage();
  await login(phone, fixture.owner_email);
  await phone.getByRole("row").filter({ hasText: name }).getByRole("button", { name: "Open", exact: true }).click();
  await check("Mobile tabs, long labels and Flow remain usable", async () => {
    await phone.getByRole("tab", { name: "Steps", exact: true }).click();
    await phone.locator(".journey-step-select").filter({ hasText: "Another item" }).click();
    assert.ok(await phone.getByLabel("Step name", { exact: true }).isVisible());
    await phone.getByRole("tab", { name: "Preview", exact: true }).click();
    await phone.getByRole("button", { name: "Start preview", exact: true }).click();
    await phone.getByRole("button", { name: fixture.store_name, exact: true }).waitFor();
    assert.ok(await phone.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    await phone.screenshot({ path: path.join(artifacts, "mobile-chat.png"), fullPage: true });
    await phone.getByRole("button", { name: "Flow", exact: true }).click();
    await phone.getByTestId("compiled-flow-screen").waitFor();
    await phone.screenshot({ path: path.join(artifacts, "mobile-flow.png"), fullPage: true });
  });
  const readOnly = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const viewer = await readOnly.newPage();
  await login(viewer, fixture.viewer_email);
  await check("Viewer UI is read-only and API denies edit/publication/test", async () => {
    assert.equal(await viewer.getByRole("button", { name: "New journey", exact: true }).count(), 0);
    await viewer.getByRole("row").filter({ hasText: name }).getByRole("button", { name: "Open", exact: true }).click();
    assert.equal(await viewer.getByRole("button", { name: "Publish", exact: true }).count(), 0);
    assert.equal(await viewer.getByRole("button", { name: "Save draft", exact: true }).count(), 0);
    assert.equal(await viewer.getByLabel("Customer message", { exact: true }).isEnabled(), false);
    const detail = await request("GET", `/journeys/${journey.id}`);
    token = (await request("POST", "/auth/login", { email: fixture.viewer_email, password: fixture.password })).access_token;
    await request("PUT", `/journey-versions/${detail.version.id}`, detail.definition, 403);
    await request("POST", `/journey-versions/${detail.version.id}/publish`, {}, 403);
    await request("POST", `/journey-versions/${detail.version.id}/controlled-test`, { acknowledge_test_records: true }, 403);
    await viewer.screenshot({ path: path.join(artifacts, "viewer-readonly.png"), fullPage: true });
  });
} catch (error) {
  await page.screenshot({ path: path.join(artifacts, "failure.png"), fullPage: true }).catch(() => {});
  console.error(error);
  process.exitCode = 1;
} finally {
  await writeFile(path.join(artifacts, "results.json"), JSON.stringify({ actual_backend: api, mocked_requests: false, results, errors }, null, 2));
  await browser.close();
}
