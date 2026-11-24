import assert from "node:assert/strict";

// Uses the published service-booking bot and runtime simulator: no database
// fixtures, inbound WhatsApp bypass, outgoing messages, or payment provider.
export async function runField({ request, page, fieldURL, password, stamp, email, ownerToken, check }) {
  const owner = (method, route, body) => request(method, route, body, 200, ownerToken);
  let pool, bot, channel, job, providers = [], selected;
  async function signIn(address, destination) {
    await page.goto(`${fieldURL}/login`);
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.getByLabel("Email address", { exact: true }).fill(address);
    await page.getByLabel("Password", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page.waitForURL(`${fieldURL}/${destination}`);
  }
  async function poll(fn, message) {
    for (let attempt = 0; attempt < 40; attempt++) {
      const value = await fn();
      if (value) return value;
      await new Promise(resolve => setTimeout(resolve, 250));
    }
    assert.fail(message);
  }
  await check("Field owner sign-in, provider creation and booking mutation", async () => {
    await signIn(email, "owner");
    await page.getByRole("heading", { name: "Overview", exact: true }).waitFor();
    pool = await owner("POST", "/field/pools", { name: `Acceptance repairs ${stamp}`, slug: `acceptance-repairs-${stamp}`, status: "active" });
    await owner("PUT", "/field/settings", { require_booking_fee: false, acceptance_window_seconds: 900, dispatch_strategy: "sequential", provider_portal_base_url: fieldURL });
    for (let index = 0; index < 2; index++) {
      const address = `acceptance-provider-${stamp}-${index}@example.test`;
      const provider = await owner("POST", "/field/providers", {
        name: `Acceptance provider ${stamp} ${index}`, email: address, password,
        phone: `+23481000000${index}1`, whatsapp_number: "", area: "Lekki",
        address: "Acceptance test address, Lekki", availability: "available", status: "active", pool_ids: [pool.id], rating_average: 5 - index,
      });
      const session = await request("POST", "/auth/login", { email: address, password }, 200, "");
      assert.ok(session.access_token, "Provider login must issue a token");
      providers.push({ ...provider, email: address, token: session.access_token });
    }
    bot = await owner("POST", "/bots/service-booking", { name: `Acceptance service ${stamp}` });
    const versions = await owner("GET", `/bots/${bot.id}/versions`);
    assert.ok(versions.length, "Service template must create a version");
    await owner("POST", `/bot-versions/${versions[0].id}/publish`, {});
    channel = await owner("POST", "/channels", { provider: "web", display_name: `Local field simulator ${stamp}`, status: "active", config: JSON.stringify({ bot_id: bot.id }) });
    const session = await owner("POST", "/runtime/test/start", { bot_id: bot.id, channel_id: channel.id, sender: `acceptance-field-${stamp}`, external_conversation_id: `acceptance-field-${stamp}` });
    for (const [index, text] of ["hi", pool.name, "Acceptance Customer", "Lekki", "12 Acceptance Street", `Acceptance repair ${stamp}`, "tomorrow morning"].entries()) {
      const result = await owner("POST", "/runtime/test/message", { session_id: session.id, external_message_id: `field-${stamp}-${index}`, text });
      assert.ok(!result.error, `Booking conversation step ${index} must succeed`);
    }
    job = (await owner("GET", "/field/requests")).find(row => row.session_id === session.id);
    assert.ok(job, "Simulator must persist a real field request");
    assert.equal(job.status, "dispatching");
    const matches = await owner("GET", `/field/requests/${job.id}/matches`);
    assert.equal(matches.length, 2, "Both fresh providers must match");
  });
  await check("Field provider declines dispatch and next provider accepts", async () => {
    assert.ok(job, "Field booking setup failed");
    for (const provider of providers) {
      const inbox = await request("GET", "/field/inbox", undefined, 200, provider.token);
      if (inbox.some(row => row.request_id === job.id)) selected = provider;
    }
    assert.ok(selected, "A matching provider must receive a dispatch");
    const first = selected;
    const attempt = (await request("GET", "/field/inbox", undefined, 200, first.token)).find(row => row.request_id === job.id);
    await signIn(first.email, "provider");
    await page.goto(`${fieldURL}/provider/inbox`);
    const declined = page.waitForResponse(response => response.url().endsWith(`/field/dispatch/${attempt.id}/decline`) && response.request().method() === "POST");
    await page.getByRole("button", { name: "Decline", exact: true }).click();
    assert.equal((await declined).status(), 200);
    assert.ok(!(await request("GET", "/field/inbox", undefined, 200, first.token)).some(row => row.id === attempt.id), "Declined dispatch must leave the inbox");
    assert.equal((await owner("GET", `/field/requests/${job.id}`)).status, "dispatching");
    selected = providers.find(row => row.id !== first.id);
    const next = await poll(async () => (await request("GET", "/field/inbox", undefined, 200, selected.token)).find(row => row.request_id === job.id), "Next provider must receive dispatch after decline");
    assert.equal(next.status, "notified");
    await signIn(selected.email, "provider");
    await page.goto(`${fieldURL}/provider/inbox`);
    const accepted = page.waitForResponse(response => response.url().endsWith(`/field/dispatch/${next.id}/accept`) && response.request().method() === "POST");
    await page.getByRole("button", { name: "Accept", exact: true }).click();
    assert.equal((await accepted).status(), 200);
    const saved = await owner("GET", `/field/requests/${job.id}`);
    assert.equal(saved.status, "assigned");
    assert.equal(saved.assigned_provider_id, selected.id);
    assert.equal((await request("GET", "/field/home", undefined, 200, selected.token)).current_job.status, "assigned");
  });
  await check("Field assigned job completes every work status through UI", async () => {
    assert.ok(job && selected, "Dispatch setup failed");
    await page.goto(`${fieldURL}/provider/jobs/${job.id}`);
    for (const status of ["on_the_way", "arrived", "in_progress", "completed"]) {
      const changed = page.waitForResponse(response => response.url().endsWith(`/field/requests/${job.id}/status`) && response.request().method() === "POST");
      await page.getByRole("button", { name: status.replaceAll("_", " "), exact: true }).click();
      assert.equal((await changed).status(), 200);
      await poll(async () => (await owner("GET", `/field/requests/${job.id}`)).status === status, `Field request must persist ${status}`);
      await page.locator(".pill").filter({ hasText: status.replaceAll("_", " ") }).waitFor();
    }
    const home = await request("GET", "/field/home", undefined, 200, selected.token);
    assert.equal(home.availability, "available");
    assert.equal(home.jobs_completed, 1);
  });
}
