import assert from 'node:assert/strict';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const admin='https://zidicommerce-staging-admin-staging.up.railway.app';
const api='https://zidicommerce-staging-api-staging.up.railway.app/v1';
const out=process.env.OUT_DIR || '.staging-evidence/2026-11-24-crud';await mkdir(out,{recursive:true,mode:0o700});
const login=await readFile(process.env.LOGIN_FILE || '/Users/oluwateniolasobande/.zidi-staging-login.YZZyMV/login.txt','utf8');
const email=login.match(/^Email:\s*(.+)$/mi)[1].trim(),password=login.match(/^Password:[ \t]*(.+)$/mi)[1].trim();
const browser=await chromium.launch({headless:true});const context=await browser.newContext({viewport:{width:1440,height:1000}});const page=await context.newPage();page.setDefaultTimeout(20000);
let token='';const results=[];const stamp=Date.now();
const button=name=>page.getByRole('button',{name,exact:true});
function safe(v){return String(v).split(password).join('[redacted]').replace(/eyJ[A-Za-z0-9_.-]+/g,'[redacted]').replace(/token=[^\s&]+/g,'token=[redacted]');}
async function check(name,fn){try{const detail=await fn();results.push({name,status:'PASS',detail});console.log('PASS '+name);}catch(e){results.push({name,status:'FAIL',reason:safe(e.message).slice(0,700)});console.log('FAIL '+name+': '+safe(e.message).split('\n')[0]);}finally{await writeFile(out+'/results.json',JSON.stringify(results,null,2));}}
async function get(path){const r=await fetch(api+path,{headers:{Authorization:'Bearer '+token}});assert.equal(r.status,200,`GET ${path} status`);return (await r.json()).data;}
async function shot(name){if(await page.locator('input[type=password]').count())await page.locator('input[type=password]').fill('');await page.screenshot({path:out+'/'+name+'.png',fullPage:true});}
async function clickSave(name,method,path){const response=page.waitForResponse(r=>r.request().method()===method&&new URL(r.url()).pathname===('/v1'+path));await button(name).click();const r=await response;assert.equal(r.status(),200,`${name} HTTP${r.status()}`);return(await r.json()).data;}

let store,product;
async function request(method,path){assert.equal(method,'GET','Staging API helper is assertion-only');return get(path);}
async function submit(name,method,path,action){const response=page.waitForResponse(r=>r.request().method()===method&&new URL(r.url()).pathname==='/v1'+path);await action();const r=await response;assert.equal(r.status(),200,`${name} HTTP${r.status()}`);return(await r.json()).data;}

try{
 const response=page.waitForResponse(r=>r.url().endsWith('/auth/login')&&r.request().method()==='POST');await page.goto(admin+'/login');await page.getByLabel('Email',{exact:true}).fill(email);await page.getByLabel('Password',{exact:true}).fill(password);await button('Sign in').click();token=(await(await response).json()).data.access_token;await page.waitForURL(u=>u.pathname==='/');await button('Skip tour').click({timeout:3000}).catch(()=>{});
  await check("Catalogue category product variant price forms round-trip", async () => {
    await page.goto(`${admin}/catalogue`); await button("Categories").click();
    await page.getByLabel("Category name", { exact: true }).fill((process.env.CATEGORY_NAME || `Meals ${stamp}`));
    const category = await submit("Category", "POST", "/catalogue/categories", () => button("Add category").click());
    assert.ok((await request("GET", "/catalogue/categories")).some(row => row.id === category.id && row.name === (process.env.CATEGORY_NAME || `Meals ${stamp}`)));
    await page.goto(`${admin}/catalogue`); await button("Add product").click();
    await page.getByLabel("Name", { exact: true }).fill((process.env.PRODUCT_NAME || `Rice ${stamp}`));
    await page.getByLabel("Description", { exact: true }).fill(process.env.PRODUCT_DESCRIPTION || "Acceptance meal"); if(process.env.IMAGE_URL) await page.getByLabel("Image URL",{exact:true}).fill(process.env.IMAGE_URL);
    await page.getByRole("combobox", { name: /^Category\b/ }).selectOption(category.id);
    await page.getByLabel("Price (NGN)", { exact: true }).fill("1500");
    product = await submit("Product", "POST", "/catalogue/products", () => button("Create product").click());
    assert.equal(product.category_id, category.id);
    await page.goto(`${admin}/catalogue`); await button(('Edit '+(process.env.PRODUCT_NAME || `Rice ${stamp}`))).click();
    await page.getByLabel("Description", { exact: true }).fill("Edited acceptance meal");
    await submit("Product edit", "PATCH", `/catalogue/products/${product.id}`, () => button("Save product").click());
    product = await request("GET", `/catalogue/products/${product.id}`); assert.equal(product.description, "Edited acceptance meal");
    await page.goto(`${admin}/catalogue`); await button(('Edit '+(process.env.PRODUCT_NAME || `Rice ${stamp}`))).click();
    await page.getByLabel("Name", { exact: true }).last().fill("Large");
    await page.getByLabel("Price (NGN)", { exact: true }).fill("2000");
    const variant = await submit("Variant", "POST", `/catalogue/products/${product.id}/variants`, () => button("Add option").click());
    await page.goto(`${admin}/catalogue`); await button(('Edit '+(process.env.PRODUCT_NAME || `Rice ${stamp}`))).click();
    await page.getByLabel("Price for Large", { exact: true }).fill("2500");
    const row = page.getByLabel("Price for Large", { exact: true }).locator("..");
    await submit("Price", "PATCH", `/catalogue/variants/${variant.id}`, () => row.getByRole("button", { name: "Save price", exact: true }).click());
    product = await request("GET", `/catalogue/products/${product.id}`);
    assert.equal(product.variants.find(row => row.id === variant.id).price_minor, 250000);
    await page.reload(); await button(('Edit '+(process.env.PRODUCT_NAME || `Rice ${stamp}`))).click();
    assert.equal(await page.getByLabel("Price for Large", { exact: true }).inputValue(), "2500");
  });
  await check("Store create hours and edit forms round-trip", async () => {
    await page.goto(`${admin}/stores`); await button("Add store").first().click();
    await page.getByLabel("Store name", { exact: true }).fill((process.env.STORE_NAME || `Kitchen ${stamp}`));
    await page.getByLabel("Address", { exact: true }).last().fill(process.env.STORE_ADDRESS || "1 Acceptance Road");
    await page.getByLabel("City", { exact: true }).last().fill("Lagos");
    await page.getByLabel("Monday opening time", { exact: true }).fill("08:30");
    store = await submit("Store", "POST", "/stores", () => button("Create store").click());
    store = await request("GET", `/stores/${store.id}`);
    assert.ok(store.hours.find(hour => hour.day_of_week === 1).opens_at.startsWith("08:30"));
    await page.reload(); await page.getByRole("button").filter({ hasText: (process.env.STORE_NAME || `Kitchen ${stamp}`) }).click();
    await page.getByLabel("Name", { exact: true }).fill((process.env.STORE_NAME ? process.env.STORE_NAME+' Kitchen' : `Kitchen edited ${stamp}`));
    await submit("Store edit", "PATCH", `/stores/${store.id}`, () => button("Save store").click());
    store = await request("GET", `/stores/${store.id}`); assert.equal(store.name, (process.env.STORE_NAME ? process.env.STORE_NAME+' Kitchen' : `Kitchen edited ${stamp}`));
    assert.ok(store.hours.find(hour => hour.day_of_week === 1).opens_at.startsWith("08:30"));
    await page.reload();
    await page.getByRole("button").filter({ hasText: store.name }).click();
    assert.equal(await page.getByLabel("Name", { exact: true }).inputValue(), store.name);
  });
  await check("Inventory set and adjust forms round-trip", async () => {
    assert.ok(store && product, "Store/catalogue prerequisites failed");
    await page.goto(`${admin}/inventory`); await button("Set stock").first().click();
    await page.getByRole("combobox", { name: /^Store\b/ }).selectOption(store.id);
    await page.getByRole("combobox", { name: /^Product and variant\b/ }).selectOption(product.variants[0].id);
    await page.getByLabel("Quantity on hand", { exact: true }).fill("50");
    const stock = await submit("Stock", "POST", "/inventory", () => button("Save stock").click());
    await page.reload(); await page.getByRole("row").filter({hasText: product.name}).filter({hasText: store.name}).getByRole("button",{name:"Adjust",exact:true}).click();
    await page.getByLabel("New on-hand quantity", { exact: true }).fill("60");
    await button("Save quantity").click();
    await submit("Adjustment", "PATCH", `/inventory/${stock.id}`, () => page.getByRole("dialog", { name: "Save quantity", exact: true }).getByRole("button", { name: "Save quantity", exact: true }).click());
    const persisted = (await request("GET", "/inventory")).find(row => row.id === stock.id);
    assert.equal(persisted.on_hand, 60); await page.reload(); await page.getByRole("cell", { name: "60", exact: true }).first().waitFor();
  });

}finally{await browser.close();await writeFile(out+'/results.json',JSON.stringify(results,null,2));if(results.some(x=>x.status==='FAIL'))process.exitCode=1;}
