import test from 'node:test';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { StaticRouter } from 'react-router-dom';
import ts from 'typescript';
import { readFileSync, existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { createRequire } from 'node:module';
import { runInThisContext } from 'node:vm';
import { fileURLToPath } from 'node:url';
const require = createRequire(import.meta.url), cache = new Map();
// Load actual TSX components in Node, omitting only CSS imports.
function component(filename) {
  if (cache.has(filename)) return cache.get(filename);
  const exports = {}; cache.set(filename, exports);
  const { outputText } = ts.transpileModule(readFileSync(filename, 'utf8'), { compilerOptions: {module:ts.ModuleKind.CommonJS,jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022} });
  runInThisContext('(function(require,exports){' + outputText + '\n})', {filename})(specifier => {
    if (specifier.endsWith('.css')) return {};
    if (!specifier.startsWith('.')) return require(specifier);
    const path = resolve(dirname(filename), specifier);
    const source = [path, path + '.tsx', path + '.ts'].find(existsSync);
    assert.ok(source, `Missing component ${specifier}`);
    return component(source);
  }, exports);
  return exports;
}
const root = fileURLToPath(new URL('../src/components/flow-studio/', import.meta.url));
const { JourneyPreview } = component(resolve(root, 'JourneyPreview.tsx'));
const { ModeEditor } = component(resolve(root, 'ModeEditor.tsx'));
const step = {key:'question',kind:'question',title:'Your request',text:'What do you need?',help:'Choose a service',required:true,locked:false,answer_type:'text',source:'',options:[],next:'handoff',failure:'handoff'};
const draft = {name:'Service desk',mode:'guided_questions',purpose:'Help customers',start:'question',steps:[step,{...step,key:'handoff',kind:'handoff',title:'Human support',text:'A colleague will help.'}]};
const noop = () => {};
const props = {draft,previewMode:'chat',scenario:'normal',lines:[],busy:false,dirty:false,answer:'',error:'',onPreviewMode:noop,onScenario:noop,onAnswer:noop,onStart:noop,onSend:noop,canControlledTest:false,onControlledTest:noop};
test('starting simulation renders returned runtime messages as shared WhatsApp bubbles', () => {
  const preview = {session_id:'preview',current_step:'question',turn:'one',flow_screen:'',cart:[],result:{status:'active',messages:[{type:'buttons',text:'Which service?',options:[{id:'help',label:'Get help'}]}]}};
  const html = renderToStaticMarkup(createElement(JourneyPreview,{...props,preview,lines:preview.result.messages.map(message => ({from:'bot',message}))}));
  assert.match(html,/wa-bubble/); assert.match(html,/Which service\?/); assert.match(html,/Get help/);
  assert.doesNotMatch(html,/>Start preview</); assert.match(html,/Happy path/);
  assert.doesNotMatch(html,/>Flow<\/button>/);
  const flow = renderToStaticMarkup(createElement(JourneyPreview,{...props,draft:{...draft,mode:'whatsapp_flow'}}));
  assert.match(flow,/>Flow<\/button>/);
});
test('AI editor names API limitations and retains knowledge and handoff access', () => {
  const html = renderToStaticMarkup(createElement(StaticRouter,{location:"/journeys"},createElement(ModeEditor,{draft:{...draft,mode:'ai'},editable:true,onChange:noop,onSelectStep:noop,selected:'question',advanced:null,inspector:null})));
  assert.match(html,/Persona, instructions and editable AI guardrails are not available/);
  assert.match(html,/href="\/knowledge"/); assert.match(html,/A colleague will help\./);
});
test('Flow screens exclude unsupported entry menus and retain supported input fields', () => {
  const html = renderToStaticMarkup(createElement(StaticRouter,{location:"/journeys"},createElement(ModeEditor,{draft:{...draft,mode:'whatsapp_flow',steps:[{...step,key:'entry',kind:'entry_menu',title:'Unsupported menu'},step]},editable:true,onChange:noop,onSelectStep:noop,selected:'question',advanced:null,inspector:null})));
  assert.match(html,/Screen title/); assert.match(html,/Input component/); assert.match(html,/Date picker/);
  assert.doesNotMatch(html,/Unsupported menu/);
});
