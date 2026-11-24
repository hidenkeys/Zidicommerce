// Run after build:admin. This local browser regression never reaches a real API.
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
const base=process.env.ADMIN_URL || 'http://127.0.0.1:4326';
assert.ok(['localhost','127.0.0.1'].includes(new URL(base).hostname),'Local preview only');
const browser=await chromium.launch({headless:true});
try {
 const context=await browser.newContext();
 await context.addInitScript(()=>localStorage.setItem('zidicommerce_token','local-mocked-session'));
 const page=await context.newPage();const patches=[];
 let store={id:'store-1',name:'Dessert kitchen',city:'Lagos',country:'NG',status:'active',latitude:6.45,longitude:3.4,fulfilment_modes:[{mode:'pickup',enabled:true,delivery_fee_minor:0}]};
 await context.route('**/v1/**',async route=>{
  const req=route.request();const headers={'Access-Control-Allow-Origin':'*','Access-Control-Allow-Headers':'*','Access-Control-Allow-Methods':'*'};
  if(req.method()==='OPTIONS'){await route.fulfill({status:204,headers});return;}
  const path=new URL(req.url()).pathname.slice(3);let data=[];
  if(path==='/auth/me')data={id:'owner',organization_id:'org',role:'merchant_admin',email:'owner@example.test'};
  else if(path==='/organizations/current')data={id:'org',name:'Test workspace'};
  else if(path==='/stores')data=[store];
  else if(path==='/stores/store-1'&&req.method()==='PATCH'){const patch=req.postDataJSON();patches.push(patch);store={...store,...patch};data=store;}
  await route.fulfill({status:200,contentType:'application/json',headers,body:JSON.stringify({data})});
 });
 await page.goto(base+'/stores');await page.getByRole('button',{name:'Skip tour',exact:true}).click({timeout:2000}).catch(()=>{});
 await page.getByRole('button').filter({hasText:'Dessert kitchen'}).click();
 async function save(){await Promise.all([page.waitForResponse(r=>r.request().method()==='PATCH'),page.waitForResponse(r=>r.request().method()==='GET'&&new URL(r.url()).pathname.endsWith('/stores')),page.getByRole('button',{name:'Save store',exact:true}).click()]);}
 await page.getByLabel('Name',{exact:true}).fill('Dessert kitchen updated');await save();
 assert.equal(patches[0].latitude,6.45);assert.equal(patches[0].longitude,3.4);
 assert.equal(Object.hasOwn(patches[0],'hours'),false,'Details save must not introduce a schedule');
 assert.equal(Object.hasOwn(patches[0],'fulfilment_modes'),false,'Details save must not rewrite fulfilment');
 await page.getByRole('button',{name:'Set opening hours',exact:true}).click();
 await page.getByLabel('Monday opening time',{exact:true}).fill('08:30');await save();
 assert.equal(patches[1].hours.find(h=>h.day_of_week===1).opens_at,'08:30');
 assert.deepEqual(patches[1].fulfilment_modes,[{mode:'pickup',enabled:true,delivery_fee_minor:0}]);
 await page.getByRole('button',{name:'Edit opening hours',exact:true}).click();
 await page.getByRole('button',{name:'Remove opening hours',exact:true}).click();await save();
 assert.deepEqual(patches[2].hours,[],'Removing hours is an explicit saved action');
 assert.deepEqual(patches[2].fulfilment_modes,patches[1].fulfilment_modes);
 console.log('PASS detail-only store saves preserve configuration; explicit hours edit and removal retain fulfilment');
 await context.close();
}finally{await browser.close();}
