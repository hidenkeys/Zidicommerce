import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { randomBytes } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { runField } from "./local-acceptance-field.mjs";

// Deliberately fixed origins: this suite must never mutate a remote tenant.
const api = "http://127.0.0.1:4325/v1";
const adminURL = "http://127.0.0.1:4326";
const fieldURL = "http://127.0.0.1:4327";
const evidence = ".local-acceptance/evidence";
const stamp = Date.now().toString();
const password = `Acceptance!${randomBytes(24).toString("hex")}`;
const email = `owner-${stamp}@acceptance.invalid`;
const results = [];
let ownerToken = "", orgID, staffToken, store, product;
await mkdir(evidence, { recursive: true, mode: 0o700 });
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE_PATH ?? "playwright");
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
await context.route("**/*", route => {
  const url = new URL(route.request().url());
  return ["127.0.0.1", "localhost"].includes(url.hostname) ? route.continue() : route.abort();
});
const page = await context.newPage();
page.setDefaultTimeout(15000);
const secrets = [password];
function safe(message) {
  let result = String(message);
  for (const secret of secrets) if (secret) result = result.split(secret).join("[redacted]");
  return result.replace(/token=[^\s"&]+/g, "token=[redacted]").replace(/\/invitations\/[^/\s]+/g, "/invitations/[redacted]");
}
async function request(method, route, body, expected = 200, token = ownerToken) {
  const response = await fetch(`${api}${route}`, {
    method, headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }), signal: AbortSignal.timeout(20000), redirect: "error",
  });
  // Never include response bodies: auth responses contain credentials.
  assert.equal(response.status, expected, `${method} ${route.split("?")[0]} returned ${response.status}; expected ${expected}`);
  const text = await response.text();
  return text ? JSON.parse(text).data : undefined;
}
async function poll(work, predicate, description) {
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    const value = await work();
    if (predicate(value)) return value;
    await new Promise(resolve => setTimeout(resolve, 250));
  }
  throw new Error(`Timed out: ${description}`);
}
async function check(label, work) {
  try { await work(); results.push({ check: label, status: "PASS" }); console.log(`PASS ${label}`); }
  catch (error) {
    const reason = safe(error.message).split("\n")[0];
    results.push({ check: label, status: "FAIL", reason }); console.log(`FAIL ${label}: ${reason}`);
    // Credentials are never typed into persisted screenshots.
    await page.locator('input[type="password"], input[type="email"]').evaluateAll(nodes => nodes.forEach(node => { node.value = ""; })).catch(() => {});
    await page.screenshot({ path: `${evidence}/${results.length}-${label.replace(/[^a-z0-9]+/gi, "-")}.png`, fullPage: true }).catch(() => {});
  }
}
async function loginUI(target, base, address, pass) {
  await target.goto(`${base}/login`);
  await target.evaluate(() => localStorage.clear());
  await target.reload();
  await target.getByLabel("Email", { exact: true }).fill(address);
  await target.getByLabel("Password", { exact: true }).fill(pass);
  await target.getByRole("button", { name: "Sign in", exact: true }).click();
  await target.waitForURL(url => url.pathname !== "/login");
  if (base === adminURL) {
    await target.getByRole("complementary", { name: "Primary navigation" }).waitFor();
    const skip = target.getByRole("button", { name: "Skip tour", exact: true });
    await skip.waitFor({ timeout: 5000 }).catch(() => {});
    if (await skip.isVisible()) await skip.click();
  }
}
async function submit(name, method, route, action) {
  const response = page.waitForResponse(response => response.request().method() === method && new URL(response.url()).pathname === `/v1${route}`);
  await action();
  const saved = await response;
  assert.equal(saved.status(), 200, `${name} save returned ${saved.status()}`);
  return (await saved.json()).data;
}
const button = name => page.getByRole("button", { name, exact: true });
try {
  await check("Register fresh owner and organization", async () => {
    const registration = await request("POST", "/auth/register", { email, password, first_name: "Acceptance", last_name: "Owner" }, 200, "");
    ownerToken = registration.access_token; secrets.push(ownerToken);
    const onboard = await request("POST", "/onboarding/organization", { name: `Acceptance Foods ${stamp}`, slug: `acceptance-foods-${stamp}`, country: "NG", currency: "NGN", timezone: "Africa/Lagos", contact_email: email });
    ownerToken = onboard.access_token; secrets.push(ownerToken); orgID = onboard.organization.id;
    assert.equal((await request("GET", "/auth/me")).organization_id, orgID);
    await loginUI(page, adminURL, email, password);
  });
  await check("Knowledge ingest review approve archive", async () => {
    const source = await request("POST", "/document-sources", { title: `Policies ${stamp}`, source_type: "pasted_text", raw_text: "# Delivery\nDelivery takes two days.\n\n# Returns\nReturns are accepted within seven days." });
    assert.equal(source.status, "review_required");
    const chunks = await poll(() => request("GET", `/document-sources/${source.id}/chunks`), rows => rows.length >= 2, "document chunks");
    for (const chunk of chunks) {
      assert.equal(chunk.status, "review_required");
      const reviewed = await request("PATCH", `/document-chunks/${chunk.id}`, { title: chunk.title, content: `${chunk.content}\nReviewed by acceptance.`, status: "review_required" });
      assert.match(reviewed.content, /Reviewed by acceptance/);
      const approved = await request("POST", `/document-chunks/${chunk.id}/approve`, { kind: "delivery", category: "delivery", question: chunk.title, keywords: ["delivery"] });
      assert.equal(approved.status, "approved"); assert.ok(approved.knowledge_entry_id);
      assert.equal((await request("GET", `/knowledge-entries/${approved.knowledge_entry_id}`)).status, "active");
      assert.equal((await request("GET", `/document-sources/${source.id}/chunks`)).find(row => row.id === chunk.id).status, "approved");
      await request("DELETE", `/document-chunks/${chunk.id}`);
    }
    assert.ok((await request("GET", `/document-sources/${source.id}/chunks`)).every(chunk => chunk.status === "archived"));
    await request("DELETE", `/document-sources/${source.id}`);
    assert.equal((await request("GET", `/document-sources/${source.id}`)).status, "archived");
  });
  await check("Catalogue category product variant price forms round-trip", async () => {
    await page.goto(`${adminURL}/catalogue`); await button("Categories").click();
    await page.getByLabel("Category name", { exact: true }).fill(`Meals ${stamp}`);
    const category = await submit("Category", "POST", "/catalogue/categories", () => button("Add category").click());
    assert.ok((await request("GET", "/catalogue/categories")).some(row => row.id === category.id && row.name === `Meals ${stamp}`));
    await page.goto(`${adminURL}/catalogue`); await button("Add product").click();
    await page.getByLabel("Name", { exact: true }).fill(`Rice ${stamp}`);
    await page.getByLabel("Description", { exact: true }).fill("Acceptance meal");
    await page.getByRole("combobox", { name: /^Category\b/ }).selectOption(category.id);
    await page.getByLabel("Price (NGN)", { exact: true }).fill("1500");
    product = await submit("Product", "POST", "/catalogue/products", () => button("Create product").click());
    assert.equal(product.category_id, category.id);
    await page.goto(`${adminURL}/catalogue`); await button(`Edit Rice ${stamp}`).click();
    await page.getByLabel("Description", { exact: true }).fill("Edited acceptance meal");
    await submit("Product edit", "PATCH", `/catalogue/products/${product.id}`, () => button("Save product").click());
    product = await request("GET", `/catalogue/products/${product.id}`); assert.equal(product.description, "Edited acceptance meal");
    await page.goto(`${adminURL}/catalogue`); await button(`Edit Rice ${stamp}`).click();
    await page.getByLabel("Name", { exact: true }).last().fill("Large");
    await page.getByLabel("Price (NGN)", { exact: true }).fill("2000");
    const variant = await submit("Variant", "POST", `/catalogue/products/${product.id}/variants`, () => button("Add option").click());
    await page.goto(`${adminURL}/catalogue`); await button(`Edit Rice ${stamp}`).click();
    await page.getByLabel("Price for Large", { exact: true }).fill("2500");
    const row = page.getByLabel("Price for Large", { exact: true }).locator("..");
    await submit("Price", "PATCH", `/catalogue/variants/${variant.id}`, () => row.getByRole("button", { name: "Save price", exact: true }).click());
    product = await request("GET", `/catalogue/products/${product.id}`);
    assert.equal(product.variants.find(row => row.id === variant.id).price_minor, 250000);
    await page.reload(); await button(`Edit Rice ${stamp}`).click();
    assert.equal(await page.getByLabel("Price for Large", { exact: true }).inputValue(), "2500");
  });
  await check("Store create hours and edit forms round-trip", async () => {
    await page.goto(`${adminURL}/stores`); await button("Add store").first().click();
    await page.getByLabel("Store name", { exact: true }).fill(`Kitchen ${stamp}`);
    await page.getByLabel("Address", { exact: true }).last().fill("1 Acceptance Road");
    await page.getByLabel("City", { exact: true }).last().fill("Lagos");
    await page.getByLabel("Monday opening time", { exact: true }).fill("08:30");
    store = await submit("Store", "POST", "/stores", () => button("Create store").click());
    store = await request("GET", `/stores/${store.id}`);
    assert.ok(store.hours.find(hour => hour.day_of_week === 1).opens_at.startsWith("08:30"));
    await page.reload(); await page.getByRole("button").filter({ hasText: `Kitchen ${stamp}` }).click();
    await page.getByLabel("Name", { exact: true }).fill(`Kitchen edited ${stamp}`);
    await submit("Store edit", "PATCH", `/stores/${store.id}`, () => button("Save store").click());
    store = await request("GET", `/stores/${store.id}`); assert.equal(store.name, `Kitchen edited ${stamp}`);
    assert.ok(store.hours.find(hour => hour.day_of_week === 1).opens_at.startsWith("08:30"));
    await page.reload();
    await page.getByRole("button").filter({ hasText: store.name }).click();
    assert.equal(await page.getByLabel("Name", { exact: true }).inputValue(), store.name);
  });
  await check("Inventory set and adjust forms round-trip", async () => {
    assert.ok(store && product, "Store/catalogue prerequisites failed");
    await page.goto(`${adminURL}/inventory`); await button("Set stock").first().click();
    await page.getByRole("combobox", { name: /^Store\b/ }).selectOption(store.id);
    await page.getByRole("combobox", { name: /^Product and variant\b/ }).selectOption(product.variants[0].id);
    await page.getByLabel("Quantity on hand", { exact: true }).fill("50");
    const stock = await submit("Stock", "POST", "/inventory", () => button("Save stock").click());
    await page.reload(); await button("Adjust").first().click();
    await page.getByLabel("New on-hand quantity", { exact: true }).fill("60");
    await button("Save quantity").click();
    await submit("Adjustment", "PATCH", `/inventory/${stock.id}`, () => page.getByRole("dialog", { name: "Save quantity", exact: true }).getByRole("button", { name: "Save quantity", exact: true }).click());
    const persisted = (await request("GET", "/inventory")).find(row => row.id === stock.id);
    assert.equal(persisted.on_hand, 60); await page.reload(); await page.getByRole("cell", { name: "60", exact: true }).first().waitFor();
  });
  await check("Invite limited staff accept logged link and sign in", async () => {
    const staffEmail = `staff-${stamp}@acceptance.invalid`;
    await request("POST", "/organizations/current/invitations", { email: staffEmail, role: "store_staff", first_name: "Acceptance", last_name: "Staff", store_ids: store ? [store.id] : [] });
    const entry = await poll(async () => (await readFile(".local-acceptance/api.log", "utf8")).split("\n").flatMap(line => { try { return [JSON.parse(line)]; } catch { return []; } }).find(row => row.msg === "local acceptance email" && row.to === staffEmail), Boolean, "local logged invitation");
    const match = entry.text.match(/\/invitations\/accept\?token=([^\s]+)/); assert.ok(match, "No invitation link in local log");
    secrets.push(match[1]);
    const accepted = await request("POST", `/invitations/${match[1]}/accept`, { password, first_name: "Acceptance", last_name: "Staff" }, 200, "");
    staffToken = accepted.access_token; secrets.push(staffToken);
    assert.equal((await request("POST", "/auth/login", { email: staffEmail, password }, 200, "")).user.role, "store_staff");
    await loginUI(page, adminURL, staffEmail, password);
  });
  for (const [label, route, method, endpoint, body] of [
    ["Payments settings", "/settings/payments", "GET", "/payment-configurations"],
    ["Team", "/team", "GET", "/organizations/current/members"],
    ["Channels manage", "/settings/whatsapp", "POST", "/channels", {}],
    ["Flow Studio publish", "/journeys", "POST", "/journey-versions/00000000-0000-0000-0000-000000000001/publish", {}],
  ]) await check(`${label} staff UI denial and API 403`, async () => {
    assert.ok(staffToken, "Staff acceptance prerequisite failed");
    await request(method, endpoint, body, 403, staffToken);
    await page.goto(`${adminURL}${route}`);
    await page.getByText(/access required|permission|not authorized|forbidden|cannot view|not have access/i).first().waitFor();
  });
  await runField({ request, loginUI, page, fieldURL, password, stamp, orgID, check, ownerToken, email });
  await check("Orders simulated payment processing ready completed and cancellation", async () => {
    assert.ok(store && product, "Store/catalogue prerequisites failed");
    const customer = await request("POST", "/customers", { first_name: "Acceptance", last_name: "Customer", email: `customer-${stamp}@acceptance.invalid`, phone: `+23480${stamp.slice(-8)}` });
    const create = key => request("POST", "/orders", { store_id: store.id, customer_id: customer.id, fulfilment_type: "pickup", currency: "NGN", idempotency_key: `${stamp}-${key}`, items: [{ variant_id: product.variants[0].id, quantity: 1 }] });
    const order = await create("complete"); assert.equal(order.status, "awaiting_payment");
    const payment = await request("POST", "/payments/initialize", { order_id: order.id, provider: "test", email: customer.email, idempotency_key: `${order.id}-payment`, source: "local_acceptance" });
    assert.equal(payment.provider, "test");
    await request("POST", "/payments/verify", { reference: payment.reference, source: "local_acceptance" });
    assert.equal((await request("GET", `/orders/${order.id}`)).status, "paid");
    for (const status of ["processing", "ready", "completed"]) {
      await request("POST", `/orders/${order.id}/transition`, { status, idempotency_key: `${order.id}-${status}`, source: "local_acceptance" });
      assert.equal((await request("GET", `/orders/${order.id}`)).status, status);
    }
    const cancelled = await create("cancel");
    await request("POST", `/orders/${cancelled.id}/transition`, { status: "cancelled", reason: "Acceptance cancellation", idempotency_key: `${cancelled.id}-cancel` });
    assert.equal((await request("GET", `/orders/${cancelled.id}`)).status, "cancelled");
  });
  // Keep deletion last: every preceding check exercises this organization.
  await check("Hard-delete acceptance organization after dry run", async () => {
    assert.match(orgID, /^[0-9a-f-]{36}$/i);
    const body = { confirm_slug: `acceptance-foods-${stamp}` };
    assert.equal((await request("GET", `/organizations/${orgID}/hard-delete-capability`)).enabled, true);
    const preview = await request("DELETE", `/organizations/${orgID}?dry_run=true`, body);
    assert.equal(preview.dry_run, true);
    assert.equal(preview.counts.organizations, 1);
    assert.ok(preview.total_rows > 1);
    assert.equal((await request("GET", "/auth/me")).organization_id, orgID);
    const deleted = await request("DELETE", `/organizations/${orgID}`, body);
    assert.equal(deleted.dry_run, false);
    assert.equal(deleted.counts.organizations, 1);
    await request("GET", "/auth/me", undefined, 401, ownerToken);
    await request("POST", "/auth/login", { email, password }, 401, "");
    // Inspect every tenant column directly; a revoked token cannot query counts.
    // Fixed local-acceptance DB only, never DATABASE_URL or a remote database.
    const sql = `DO $check$ DECLARE t record; remaining bigint; BEGIN
      FOR t IN SELECT table_name FROM information_schema.columns
        WHERE table_schema='public' AND column_name='organization_id' LOOP
        EXECUTE format('SELECT count(*) FROM public.%I WHERE organization_id=$1',t.table_name)
          INTO remaining USING '${orgID}'::uuid;
        IF remaining<>0 THEN RAISE EXCEPTION 'tenant rows remain in %',t.table_name; END IF;
      END LOOP;
      IF EXISTS(SELECT 1 FROM organizations WHERE id='${orgID}'::uuid) THEN
        RAISE EXCEPTION 'organization remains'; END IF;
    END $check$;`;
    await promisify(execFile)("/opt/homebrew/bin/psql", ["-X", "-v", "ON_ERROR_STOP=1", "-h", "127.0.0.1", "-p", "55450", "-U", "acceptance", "-d", "commerce_acceptance", "-c", sql], { timeout: 20000 });
  });

} finally {
  await browser.close();
  await writeFile(`${evidence}/results.json`, JSON.stringify(results, null, 2), { mode: 0o600 });
}
console.log(`${results.filter(row => row.status === "PASS").length}/${results.length} checks passed`);
process.exitCode = results.some(row => row.status === "FAIL") ? 1 : 0;
