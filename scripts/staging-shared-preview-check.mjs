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

await check('Shared Chat and Flow renderer completes a custom form',async()=>{const id=process.env.JOURNEY_ID||'8a5c92c3-0c40-42c6-9619-46479596882b';const d=await get('/journeys/'+id);await page.goto(admin+'/journeys?journey='+id);await page.getByRole('heading',{name:d.journey.name,exact:true}).waitFor();await page.getByRole('group',{name:'Preview presentation',exact:true}).getByRole('button',{name:'Chat',exact:true}).click();const started=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/preview'));await button('Test').click();assert.equal((await started).status(),200);await page.getByRole('log',{name:'WhatsApp preview messages',exact:true}).waitFor();await shot('shared-chat');await page.getByRole('group',{name:'Preview presentation',exact:true}).getByRole('button',{name:'Flow',exact:true}).click();const screen=page.getByTestId('compiled-flow-screen');await screen.waitFor();await shot('shared-native-form');await screen.getByRole('textbox').first().fill('Strawberry Sundae');const response=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().includes('/journey-previews/')&&r.url().endsWith('/answer'));await screen.getByRole('button',{name:'Continue',exact:true}).click();const r=await response;assert.equal(r.status(),200);const result=(await r.json()).data;assert.equal(result.result.status,'completed');await shot('shared-form-complete');return{journey:id,status:result.result.status,screen:result.flow_screen,variables:result.variables};});
}finally{await browser.close();await writeFile(out+'/results.json',JSON.stringify(results,null,2));if(results.some(x=>x.status==='FAIL'))process.exitCode=1;}
