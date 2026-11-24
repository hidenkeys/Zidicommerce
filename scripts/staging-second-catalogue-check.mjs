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

await check('Dessert catalogue with product photographs',async()=>{const reference=JSON.parse(await readFile('.staging-evidence/second-catalogue-reference.json','utf8'));const existing=await get('/catalogue/products');const ids=[];for(const p of reference){if(existing.some(x=>x.name===p.name))continue;await page.goto(admin+'/catalogue');await button('Add product').click();await page.getByLabel('Name',{exact:true}).fill(p.name);await page.getByLabel('Description',{exact:true}).fill(p.description||p.name+' prepared for staging orders.');await page.getByLabel('Price (NGN)',{exact:true}).fill(String(p.price/100));await page.getByLabel('Image URL',{exact:true}).fill(p.images[0].url);const c=await clickSave('Create product','POST','/catalogue/products');ids.push(c.id);}await page.goto(admin+'/catalogue');await shot('dessert-catalogue');return{created:ids};});
await check('Workspace setup and next steps',async()=>{await page.goto(admin+'/');await page.getByRole('heading',{name:'Get your store ready',exact:true}).waitFor();await shot('readiness');await page.goto(admin+'/settings/whatsapp');await page.getByText('Connect WhatsApp',{exact:true}).first().waitFor();await shot('channels-empty');return{readiness:true};});
}finally{await browser.close();await writeFile(out+'/results.json',JSON.stringify(results,null,2));if(results.some(x=>x.status==='FAIL'))process.exitCode=1;}
