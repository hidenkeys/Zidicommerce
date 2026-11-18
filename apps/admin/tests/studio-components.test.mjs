import test from "node:test";
import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ts from "typescript";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { runInThisContext } from "node:vm";

test("published graph displays choice labels and excludes graph mutation controls", async () => {
  const source=readFileSync(new URL("../src/pages/JourneyCanvas.tsx",import.meta.url),"utf8");
  const { outputText }=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022}});
  const exports={};
  runInThisContext("(function(require,exports){"+outputText+"})")(createRequire(import.meta.url),exports);
  const { JourneyCanvas }=exports;
    const props={start:"welcome",selected:"welcome",editable:false,nodes:[
      {key:"welcome",title:"Start shopping",kind:"buttons",next:"",failure:"",options:[{next:"finish",label:"Go to checkout"}]},
      {key:"finish",title:"Finish",kind:"end",next:"",failure:""}
    ],select(){},arrange(){},paste(){},connect(){}};
    const html=renderToStaticMarkup(createElement(JourneyCanvas,props));
    assert.match(html,/Go to checkout/);assert.match(html,/Start shopping/);assert.match(html,/marker-end=/);
    assert.doesNotMatch(html,/Auto-layout workflow|Connect next|Select multiple/);
    const draft=renderToStaticMarkup(createElement(JourneyCanvas,{...props,editable:true}));
    assert.match(draft,/Connect next/);assert.match(draft,/Select multiple/);

});
