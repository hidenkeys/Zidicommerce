import test from 'node:test';
import assert from 'node:assert/strict';
import { channelStatus, providerAvailabilityState, isTestConnection, capabilityLabel } from '../src/lib/channel-status.ts';
test('one status combines connection, health and credential readiness', () => {
  assert.equal(channelStatus({status:'healthy',health_status:'healthy'}).label,'Connected');
  assert.equal(channelStatus({status:'connected',health_status:'degraded'}).label,'Needs attention');
  assert.match(channelStatus({status:'connected',credential_status:'expired'}).reason,/credentials/);
  assert.equal(channelStatus({status:'setup_required'}).label,'Setup incomplete');
  assert.equal(channelStatus({status:'disconnected',health_status:'healthy'}).label,'Disconnected');
});
test('capability failures retain the actionable provider reason', () => {
  const result=channelStatus({status:'connected',capability_states:[{capability:'outbound_text',status:'missing_permission',reason:'Grant messaging permission in Meta.'}]});
  assert.equal(result.label,'Needs attention');
  assert.equal(result.reason,'Grant messaging permission in Meta.');
});
test('a filtered controlled-beta catalog finishes loading as unavailable', () => {
  assert.equal(providerAvailabilityState(false,false,false),'Not available in this workspace yet');
  assert.doesNotMatch(providerAvailabilityState(false,true,false),/Checking/);
  assert.match(providerAvailabilityState(true,false,false),/Checking/);
});
test('sandbox WhatsApp numbers stay separate from local test provider', () => {
  assert.equal(isTestConnection({provider:'whatsapp',environment:'sandbox'}),false);
  assert.equal(isTestConnection({provider:'test'}),true);
  assert.equal(capabilityLabel('outbound_text'),'Send text');
});
test('required messaging blocks need attention without flagging unsupported optional features', () => {
  assert.equal(channelStatus({provider:'whatsapp',status:'connected',capability_states:[{capability:'outbound_text',status:'restricted',reason:'Only approved tester phones can receive messages.'}]}).reason,'Only approved tester phones can receive messages.');
  assert.equal(channelStatus({provider:'whatsapp',status:'connected',capability_states:[{capability:'content_publishing',status:'unsupported_by_provider'}]}).label,'Connected');
  assert.equal(channelStatus({provider:'tiktok',status:'connected',capability_states:[{capability:'outbound_text',status:'unsupported_by_provider'}]}).label,'Connected');
  assert.equal(isTestConnection({provider:'local_test'}),true);
  assert.equal(channelStatus({provider:'whatsapp',status:'setup_required',capability_states:[{capability:'outbound_text',status:'setup_required'}]}).label,'Setup incomplete');
});
