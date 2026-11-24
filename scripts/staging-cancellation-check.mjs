import assert from 'node:assert/strict';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const admin='https://zidicommerce-staging-admin-staging.up.railway.app';
const api='https://zidicommerce-staging-api-staging.up.railway.app/v1';
const out=process.env.OUT_DIR || '.staging-evidence/2026-11-24-mode-recovery';await mkdir(out,{recursive:true,mode:0o700});
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

try{
 const response=page.waitForResponse(r=>r.url().endsWith('/auth/login')&&r.request().method()==='POST');await page.goto(admin+'/login');await page.getByLabel('Email',{exact:true}).fill(email);await page.getByLabel('Password',{exact:true}).fill(password);await button('Sign in').click();token=(await(await response).json()).data.access_token;await page.waitForURL(u=>u.pathname==='/');await button('Skip tour').click({timeout:3000}).catch(()=>{});

const reference='ZC-20261124-7A43B0E3';
await check('Cancel identified unpaid QA order and release its reservation',async()=>{
 const order=(await get('/orders')).find(x=>x.order_number===reference);assert.ok(order);assert.equal(order.store_id,'b7cc1907-1335-4dbf-8927-f0522eea1ecd');
 const before=await get('/orders/'+order.id+'/operations');assert.equal(before.order.status,'awaiting_payment');assert.notEqual(before.payment.status,'paid');
 const items=before.order.items;assert.equal(items.length,1);assert.equal(items[0].product_name,'Grape Smoothie');assert.equal(items[0].quantity,1);
 const stock=(await get('/inventory')).find(x=>x.store_id===order.store_id&&x.variant_id===items[0].variant_id);assert.ok(stock);assert.ok(stock.on_hand>=0);
 await page.goto(admin+'/orders?open='+order.id);await page.getByRole('heading',{name:reference,exact:true}).waitFor();await button('Cancel order').click();
 const response=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname==='/v1/orders/'+order.id+'/transition');
 await page.getByRole('dialog',{name:'Cancel order',exact:true}).getByRole('button',{name:'Cancel order',exact:true}).click();assert.equal((await response).status(),200);
 const after=await get('/orders/'+order.id+'/operations');assert.equal(after.order.status,'cancelled');const persisted=(await get('/inventory')).find(x=>x.id===stock.id);assert.equal(persisted.reserved,stock.reserved);assert.equal(persisted.on_hand,stock.on_hand+items[0].quantity);await shot('cancelled-order');return{reference,id:order.id,status:after.order.status,reservedBefore:stock.reserved,reservedAfter:persisted.reserved,onHandBefore:stock.on_hand,onHandAfter:persisted.on_hand,stockRestored:true};
});
}finally{await browser.close();await writeFile(out+'/results.json',JSON.stringify(results,null,2));if(results.some(x=>x.status==='FAIL'))process.exitCode=1;}
