// Delays a read-only response in this browser to reproduce rapid order switching.
import { chromium } from "playwright";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const admin = "https://zidicommerce-staging-admin-staging.up.railway.app";
const login = readFileSync(process.env.ZIDI_STAGING_LOGIN_FILE,"utf8");
const field = key => login.match(new RegExp("^"+key+"\\s*[:=]\\s*(.+)$","im"))?.[1].trim();
const browser=await chromium.launch({channel:"chrome"});
const page=await browser.newPage();
page.setDefaultTimeout(15000);
try {
  await page.goto(admin+"/login");
  await page.getByLabel("Email",{exact:true}).fill(field("email"));
  await page.getByLabel("Password",{exact:true}).fill(field("password"));
  await page.getByRole("button",{name:"Sign in",exact:true}).click();
  await page.waitForURL(url=>url.pathname!=="/login");
  await page.goto(admin+"/orders");
  await page.locator(".order-detail-head").waitFor();
  const buttons=page.getByRole("button",{name:/^Open order ZC-/});
  assert.ok(await buttons.count()>=3);
  const finalOrder=await buttons.nth(2).innerText();
  let release, signal;
  const gate=new Promise(resolve=>{release=resolve;});
  const issued=new Promise(resolve=>{signal=resolve;});
  let first=true;
  await page.route("**/orders/*/operations",async route=>{
    if(!first){await route.continue();return;}
    first=false;signal();
    const response=await route.fetch();
    await gate;
    await route.fulfill({response});
  });
  await buttons.nth(1).focus();await page.keyboard.press("Enter");await issued;
  await buttons.nth(2).focus();await page.keyboard.press("Enter");
  await page.getByRole("heading",{name:finalOrder,exact:true}).waitFor();
  const finished=page.waitForResponse(r=>r.url().endsWith("/operations"));
  release();await finished;
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  assert.equal(await page.locator(".order-detail-head h2").innerText(),finalOrder);
  console.log("Rapid keyboard order selection retained the correct detail after a delayed response.");
} finally {await browser.close();}
