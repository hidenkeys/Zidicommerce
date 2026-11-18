import test from "node:test";
import assert from "node:assert/strict";
import { inspectJourney } from "../src/pages/journey-insights.ts";

test("choice, retry cycles and handoff routes remain reachable",()=>{
 const graph=[
   {key:"start",title:"Question",kind:"buttons",text:"Which?",options:[{next:"buy"}],failure:"help"},
   {key:"buy",title:"Order",kind:"question",text:"How many?",next:"end",failure:"start"},
   {key:"help",title:"Staff",kind:"handoff"},
   {key:"end",title:"Done",kind:"end"},
 ];
 assert.deepEqual(inspectJourney("start",graph),[]);
});

test("terminal steps cannot make detached branches reachable",()=>{
 const graph=[{key:"end",kind:"end",title:"End",next:"condition"},{key:"condition",title:"Review",kind:"condition",next:"end"}];
 assert.deepEqual(inspectJourney("end",graph).map(i=>i.message),["No route from the starting step reaches Review.","Set both the matched and unmatched routes for Review."]);
});
