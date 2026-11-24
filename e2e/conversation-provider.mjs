// Run after build:admin, against the local preview. All API responses are mocked.
import assert from 'node:assert/strict';
import {chromium} from 'playwright';
const base=process.env.ADMIN_URL || 'http://127.0.0.1:4326';
assert.ok(['localhost','127.0.0.1'].includes(new URL(base).hostname),'Local preview only');
const browser=await chromium.launch({headless:true});
try {
 for(const provider of ['test','whatsapp']) {
  const context=await browser.newContext();
  await context.addInitScript(()=>localStorage.setItem('zidicommerce_token','local-mocked-session'));
  const page=await context.newPage();let policyRequests=0;
  const row={id:'conversation-1',channel_id:'channel-1',channel_provider:provider,customer_name:'Test conversation',conversation_status:'ai_handling',status:'active'};
  await context.route('**/v1/**',async route=>{
   if(route.request().method()==='OPTIONS'){await route.fulfill({status:204,headers:{'Access-Control-Allow-Origin':'*','Access-Control-Allow-Headers':'*','Access-Control-Allow-Methods':'*'}});return;}
   const path=new URL(route.request().url()).pathname.slice(3);let data=[];
   if(path==='/auth/me') data={id:'owner',organization_id:'org-1',role:'merchant_admin',email:'owner@example.test'};
   else if(path==='/organizations/current') data={id:'org-1',name:'Test workspace'};
   else if(path==='/runtime/conversations')data=[row];
   else if(path==='/runtime/conversations/conversation-1')data=row;
   else if(path.endsWith('/policy-context')){policyRequests++;data={consent_status:'unknown',service_window_open:false,can_send_freeform:false,template_required:true};}
   await route.fulfill({status:200,contentType:'application/json',headers:{'Access-Control-Allow-Origin':'*','Access-Control-Allow-Headers':'*','Access-Control-Allow-Methods':'*'},body:JSON.stringify({data})});
  });
  await page.goto(base+'/conversations');
  const skip=page.getByRole('button',{name:'Skip tour',exact:true});
  await skip.click({timeout:2000}).catch(()=>{});
  const reply=page.getByLabel('Reply to customer',{exact:true});await reply.waitFor();
  assert.equal(policyRequests,provider==='whatsapp'?1:0,provider+' policy requests');
  assert.equal(await reply.isDisabled(),provider==='whatsapp',provider+' messaging policy');
  console.log('PASS '+provider+' conversation uses the correct policy and composer state');
  await context.close();
 }
}finally{await browser.close();}
