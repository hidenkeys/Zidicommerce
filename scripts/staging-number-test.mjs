import assert from 'node:assert/strict';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const admin='https://zidicommerce-staging-admin-staging.up.railway.app';
const api='https://zidicommerce-staging-api-staging.up.railway.app/v1';
const out='.staging-evidence/2026-11-24-number-test';await mkdir(out,{recursive:true,mode:0o700});
const login=await readFile('/Users/oluwateniolasobande/.zidi-staging-login.YZZyMV/login.txt','utf8');
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

await check('Controlled test send policy and confirmation',async()=>{const recipient=String(process.env.WHATSAPP_TEST_RECIPIENT||'');if(!recipient)throw new Error('Set WHATSAPP_TEST_RECIPIENT to the designated tester number');assert.ok(recipient.endsWith('2209'));await page.goto(admin+'/settings/whatsapp/0dcec1fe-f469-43b2-9a62-a0f92836f1e7');await button('Test').click();await page.getByLabel('Approved test recipient',{exact:true}).fill(recipient);await page.getByLabel('Test message',{exact:true}).fill('Zidi staging acceptance test. Please reply menu to verify inbound delivery.');let confirmed=false;page.once('dialog',async d=>{assert.equal(d.type(),'confirm');assert.ok(d.message().includes(recipient));confirmed=true;await d.accept();});const response=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/whatsapp/test-message'));await button('Send test message').click();const r=await response;const data=await r.json();await shot('test-send-result');assert.ok(confirmed,'Explicit confirmation required');return{status:r.status(),confirmation:true,recipient:'designated tester ending 2209',result:r.status()===200?'accepted':safe(JSON.stringify(data)).slice(0,600)};});
}finally{await browser.close();await writeFile(out+'/results.json',JSON.stringify(results,null,2));}
