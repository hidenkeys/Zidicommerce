import test from 'node:test';
import assert from 'node:assert/strict';
import { journeyCapabilities } from '../src/components/flow-studio/lifecycle.ts';
const detail = (patch = {}) => ({journey:{status:'active'},version:{status:'draft'},...patch});
test('provider publication freezes edits without preventing journey publication', () => {
  const result = journeyCapabilities(detail({deployment:{status:'published'}}),true,true);
  assert.equal(result.edit,false);
  assert.equal(result.publish,true);
  assert.equal(result.activate,false);
});
test('archived experiences cannot be edited, published, cloned or resumed', () => {
  const archived = detail({journey:{status:'archived'},version:{status:'published',published_at:'2026-11-24'},binding:{paused:true}});
  assert.deepEqual(journeyCapabilities(archived,true,true),{edit:false,publish:false,activate:false,toggleSessions:false,clone:false});
});
test('viewers retain read-only access without lifecycle mutation rights', () => {
  assert.deepEqual(journeyCapabilities(detail(),false,false),{edit:false,publish:false,activate:false,toggleSessions:false,clone:false});
});
test('published versions may activate and resume but require a new draft for changes', () => {
  const published = detail({version:{status:'published',published_at:'2026-11-24'},binding:{paused:true}});
  assert.deepEqual(journeyCapabilities(published,true,true),{edit:false,publish:false,activate:true,toggleSessions:true,clone:true});
});
