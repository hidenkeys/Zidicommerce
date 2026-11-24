import test from "node:test";
import assert from "node:assert/strict";
import { availableTourSteps, tourStorageKey, shouldStartTour, persistTour } from "../src/lib/onboarding.ts";
import { hasPermission } from "../src/lib/permissions.ts";
import { checklistProgress } from "../src/lib/readiness.ts";
test("tour includes only visible permission-filtered navigation, in tour order", () => {
  assert.deepEqual(availableTourSteps(["/team", "/", "/conversations"]).map(s => s.id), ["overview", "conversations", "team"]);
  assert.deepEqual(availableTourSteps([]), []);
  assert.equal(availableTourSteps(["/setup"]).at(-1).id, "setup");
});
test("tour flags are scoped to user and organization and suppress auto start", () => {
  const values = new Map(); const storage = {getItem: k => values.get(k) ?? null, setItem: (k,v) => values.set(k,v)};
  const key = tourStorageKey("user", "org");
  assert.equal(key, "zidi.tour.v1.user.org");
  assert.equal(shouldStartTour(storage, key), true);
  for (const flag of ["started", "completed", "dismissed"]) { persistTour(storage,key,flag); assert.equal(shouldStartTour(storage,key),false); }
  assert.equal(shouldStartTour(storage,tourStorageKey("other","org")),true);
  assert.equal(shouldStartTour(storage,tourStorageKey("user","other")),true);
  const blocked = {getItem() {throw Error();}, setItem() {throw Error();}};
  assert.equal(shouldStartTour(blocked,key),true); assert.doesNotThrow(()=>persistTour(blocked,key,"dismissed"));
});
test("checklist chooses next incomplete item in readiness order, including optional preparation", () => {
  const items = [{key:"whatsapp",complete:false},{key:"stores",complete:false},{key:"organization",complete:true}];
  const status = {items};
  assert.equal(checklistProgress(status).next.key,"stores");
  assert.equal(checklistProgress(status).done,1); assert.equal(checklistProgress(status).total,3);
  assert.equal(checklistProgress({items:items.map(i=>({...i,complete:true}))}).next,undefined);
  assert.deepEqual(checklistProgress(null),{total:0,done:0,next:undefined});
});

test("restricted roles never receive tour steps for denied navigation permissions", () => {
  const nav = [
    {path:"/", permission:"organization.view"}, {path:"/catalogue",permission:"catalogue.view"},
    {path:"/settings/payments",permission:"payments.view"}, {path:"/settings/whatsapp",permission:"channels.view"},
    {path:"/journeys",permission:"bot.view"}, {path:"/conversations",permission:"conversations.view"},
    {path:"/team",permission:"staff.view"},
  ];
  const stepsFor = role => availableTourSteps(nav.filter(item=>hasPermission(role,item.permission)).map(item=>item.path));
  assert.deepEqual(stepsFor("support_agent").map(step=>step.id),["overview","conversations"]);
  assert.deepEqual(stepsFor("store_manager").map(step=>step.id),["overview","catalogue","flows","conversations","team"]);
  assert.equal(stepsFor("viewer").length,7);
  assert.deepEqual(stepsFor("unknown"),[]);
});
