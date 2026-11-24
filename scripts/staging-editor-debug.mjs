import assert from 'node:assert/strict';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const admin='https://zidicommerce-staging-admin-staging.up.railway.app';
const api='https://zidicommerce-staging-api-staging.up.railway.app/v1';
const out='.staging-evidence/2026-11-24-editor-debug';await mkdir(out,{recursive:true,mode:0o700});
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

for(const mode of ['WhatsApp Flow','AI Conversation']){const rows=(await get('/journeys')).filter(x=>x.name.startsWith('E2E ')&&x.name.endsWith(mode));const row=rows[0];const d=await get('/journeys/'+row.id);await page.goto(admin+'/journeys?journey='+row.id);await page.getByRole('heading',{name:row.name,exact:true}).waitFor();await page.waitForTimeout(1000);await shot(mode.replaceAll(' ','-'));console.log(JSON.stringify({name:row.name,id:row.id,mode:d.definition.mode,status:d.version.status,steps:d.definition.steps.map(s=>({kind:s.kind,title:s.title})),fields:await page.locator('.flow-mode-editor label').evaluateAll(ls=>ls.map(l=>({text:l.textContent,control:l.querySelector('input,textarea,select')?.tagName,disabled:l.querySelector('input,textarea,select')?.disabled}))),promptCount:await page.getByLabel('Prompt',{exact:true}).count(),purposeCount:await page.getByLabel('Purpose',{exact:true}).count()}));}
}finally{await browser.close();}
