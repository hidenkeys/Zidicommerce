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

await check('Stock all dessert products at the new store',async()=>{const store=(await get('/stores')).find(x=>x.name==='Bing Chuan Lekki Kitchen');assert.ok(store);const products=await get('/catalogue/products');const levels=await get('/inventory');const created=[];for(const product of products){const d=await get('/catalogue/products/'+product.id);const variant=d.variants[0];if(levels.some(x=>x.store_id===store.id&&x.variant_id===variant.id))continue;await page.goto(admin+'/inventory');await button('Set stock').first().click();await page.getByRole('combobox',{name:/^Store/}).selectOption(store.id);await page.getByRole('combobox',{name:/^Product and variant/}).selectOption(variant.id);await page.getByLabel('Quantity on hand',{exact:true}).fill('50');await clickSave('Save stock','POST','/inventory');created.push(product.name);}return{store:store.id,stocked:created};});
await check('Second workspace Paystack test configuration',async()=>{await page.goto(admin+'/settings/payments');await page.getByRole('heading',{name:'Payment settings',exact:true}).waitFor();const existing=(await get('/payment-configurations')).find(x=>x.provider==='paystack');if(!existing){await page.getByLabel(/^Mode/).selectOption('test');await clickSave('Connect Paystack','POST','/payment-configurations');}const configured=(await get('/payment-configurations')).find(x=>x.provider==='paystack');const config=typeof configured.public_config==='string'?JSON.parse(configured.public_config):configured.public_config;assert.equal(config.mode,'test');await shot('payments-test-mode');return{mode:config.mode,enabled:configured.enabled,status:configured.status};});
}finally{await browser.close();await writeFile(out+'/results.json',JSON.stringify(results,null,2));if(results.some(x=>x.status==='FAIL'))process.exitCode=1;}
