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

const rows=await get('/journeys');
for(const [mode,label] of [['whatsapp_flow','WhatsApp Flow'],['ai','AI Conversation']]){
await check('Recover edit validate publish preview '+mode,async()=>{const row=rows.find(x=>x.name.startsWith('E2E ')&&x.name.endsWith(label)&&x.status!=='archived'&&!x.published_version_id);assert.ok(row,'Recover existing QA draft');const id=row.id;const detail=await get('/journeys/'+id);await page.goto(admin+'/journeys?journey='+id);await page.getByRole('heading',{name:row.name,exact:true}).waitFor();
await page.getByRole('heading',{name:({automated_commerce:'Commerce settings',guided_questions:'Conversation canvas',whatsapp_flow:'Flow screens',ai:'AI conversation'})[mode],exact:true}).waitFor();let edited=false;
   if(mode==='whatsapp_flow'){await page.locator('.flow-screen-fields').getByLabel(/^Prompt/).fill('Acceptance: choose a store.');edited=true;}else if(mode==='ai'){await page.locator('.flow-mode-editor').getByLabel(/^Purpose/).fill('Acceptance: answer approved business facts.');edited=true;}else{const input=page.getByLabel('Customer message',{exact:true}).first();if(await input.isVisible()){await input.fill('Acceptance: welcome to this test.');edited=true;}}
   if(edited)await clickSave('Save draft','PUT','/journey-versions/'+detail.version.id);
   const validation=await clickSave('Validate','POST','/journey-versions/'+detail.version.id+'/validate');assert.equal(validation.valid,true,'Journey validation must pass');
   await button('Publish').click();const publishResponse=page.waitForResponse(r=>r.url().includes('/journey-versions/')&&r.url().endsWith('/publish')&&r.request().method()==='POST');await page.getByRole('dialog',{name:'Publish this journey?',exact:true}).getByRole('button',{name:'Confirm',exact:true}).click();const pr=await publishResponse;assert.equal(pr.status(),200,'Publish response: '+safe(await pr.text()).slice(0,1500));const published=await get('/journeys/'+id);assert.equal(published.version.status,'published');assert.ok(!published.binding?.connection_id,'Published unbound draft must retain no channel binding');await button('Test').click();await page.waitForTimeout(2500);await shot('experience-'+mode);return{id,version:published.version.id,mode,edited,validation,published:true,activated:false,alerts:await page.getByRole('alert').allTextContents()};
});}
}finally{await browser.close();await writeFile(out+'/results.json',JSON.stringify(results,null,2));if(results.some(x=>x.status==='FAIL'))process.exitCode=1;}
