import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeMessage, parseFlowJSON, parseFlowScreens, previewURL } from '../src/components/whatsapp/model.ts';

test('runtime text preserves catalogue and payment action output without inventing message types', () => {
  assert.equal(normalizeMessage({type:'text', text:'Catalogue: 1. Shirt'}).kind, 'text');
  const payment = normalizeMessage({type:'text', text:'Pay at https://pay.example/checkout.', metadata:{journey_checkout:true}});
  assert.equal(payment.url, 'https://pay.example/checkout');
  assert.equal(payment.text, 'Pay at https://pay.example/checkout.');
});
for (const type of ['buttons', 'list']) test(`runtime ${type} preserves IDs, labels and descriptions`, () => {
  const options = [{id:'sku-1', label:'Shirt', description:'In stock'}, {id:'broken'}, null];
  const mapped = normalizeMessage({type, text:'Choose', options, metadata:{help:'Choose one'}});
  assert.equal(mapped.kind, type);
  assert.deepEqual(mapped.options, [options[0]]);
  assert.equal(mapped.help, 'Choose one');
});
test('runtime media, location request and native Flow launch metadata', () => {
  const image = normalizeMessage({type:'image', text:'Photo', media_url:'https://example.com/photo.jpg'});
  assert.equal(image.kind, 'image'); assert.equal(image.mediaUrl, 'https://example.com/photo.jpg');
  assert.equal(normalizeMessage({type:'location_request', location_request:true}).kind, 'location_request');
  assert.equal(normalizeMessage({type:'flow', metadata:{flow_cta:'Open request'}}).flowLabel, 'Open request');
});
test('handoff remains runtime text; internal notices and customer answers map direction', () => {
  assert.equal(normalizeMessage({type:'text', text:'Preview handoff: a team member will help.'}).kind, 'text');
  assert.equal(normalizeMessage({type:'text', direction:'internal', text:'Paused'}).kind, 'notice');
  assert.equal(normalizeMessage({type:'text', from:'customer', text:'Yes'}).direction, 'outgoing');
});
test('unknown, absent and unsafe payloads have safe generic fallbacks', () => {
  for (const raw of [null, undefined, [], 42, {type:'unsupported'}]) assert.equal(normalizeMessage(raw).kind, 'fallback');
  assert.equal(normalizeMessage({type:'unknown', text:'Readable fallback'}).text, 'Readable fallback');
  assert.equal(normalizeMessage({type:'image', media_url:'javascript:alert(1)'}).mediaUrl, undefined);
  assert.equal(previewURL('data:text/html,bad'), undefined);
});
test('native Flow JSON retains nested components, navigation and data bindings', () => {
  const types = ['TextHeading','TextSubheading','TextBody','TextCaption','TextInput','TextArea','Dropdown','RadioButtonsGroup','CheckboxGroup','DatePicker','OptIn','Footer','EmbeddedLink','Image'];
  const doc = {screens:[{id:'FIRST', title:'Request', layout:{children:[{type:'Form', name:'form', children:types.map(type=>({type, name:type, text:'${data.prompt}', 'on-click-action':{name:'navigate', next:{name:'SECOND'}}}))}]}},{id:'SECOND', title:'Done', layout:{children:[]}}]};
  const parsed = parseFlowJSON(JSON.stringify(doc));
  assert.deepEqual(parsed.screens[0].layout.children[0].children.map(c=>c.type), types);
  assert.equal(parsed.screens[0].layout.children[0].children[11]['on-click-action'].next.name, 'SECOND');
  assert.deepEqual(parseFlowScreens(doc), parsed.screens);
});
test('malformed Flow JSON never throws and invalid children are ignored', () => {
  for (const raw of ['{', null, {}, {screens:[]}, {screens:[{id:'bad'}]}]) assert.equal(parseFlowJSON(raw), undefined);
  assert.deepEqual(parseFlowScreens({screens:[{id:'OK', layout:{children:[null, {type:'TextBody', text:'Hello'}]}}]})[0].layout.children, [{type:'TextBody', text:'Hello'}]);
});

test('unsupported-channel text fallback keeps runtime option identifiers actionable', () => {
  const message = normalizeMessage({type:'text', text:'Choose\n1. A long catalogue item label', options:[{id:'variant-42', label:'A long catalogue item label'}]});
  assert.equal(message.kind, 'text');
  assert.equal(message.options[0].id, 'variant-42');
});
