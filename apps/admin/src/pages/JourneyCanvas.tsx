import { useMemo, useRef, useState } from "react";
import { GitBranch, Maximize2, Minus, Plus, Wand2 } from "lucide-react";

export type Point = { x: number; y: number };
type Node = { key: string; title: string; kind: string; next: string; failure: string; options?: { label: string; next?: string }[]; position?: Point };
const width = 196, height = 76;
function layout(nodes: Node[], start: string) {
  const levels = new Map<string, number>();
  const byKey = new Map(nodes.map(node => [node.key, node]));
  const queue = [start];
  levels.set(start, 0);
  for (let i = 0; i < queue.length; i++) {
    const node = byKey.get(queue[i]);
    if (!node) continue;
    for (const target of [node.next, ...(node.options ?? []).map(o => o.next), node.failure]) {
      if (target && byKey.has(target) && !levels.has(target)) {
        levels.set(target, (levels.get(node.key) ?? 0) + 1);
        queue.push(target);
      }
    }
  }
  const rows = new Map<number, number>();
  const last = Math.max(0, ...levels.values()) + 1;
  return Object.fromEntries(nodes.map(node => {
    const level = levels.get(node.key) ?? last, row = rows.get(level) ?? 0;
    rows.set(level, row + 1);
    return [node.key, { x: 32 + level * 304, y: 48 + row * 144 }];
  }));
}

export function JourneyCanvas({ nodes, start, selected, editable, select, arrange }: {
  nodes: Node[]; start: string; selected: string; editable: boolean;
  select: (id: string) => void; arrange: (positions: Record<string, Point>) => void;
}) {
  const auto = useMemo(() => layout(nodes, start), [nodes, start]);
  const [zoom, setZoom] = useState(1);
  const [drag, setDrag] = useState<{key:string; origin:Point; mouse:Point; at:Point}>();
  const pan = useRef<{ mouse:Point; x:number; y:number }>();
  const viewport = useRef<HTMLDivElement>(null);
  const points = Object.fromEntries(nodes.map(node => [node.key, drag?.key === node.key ? drag.at : node.position ?? auto[node.key]]));
  const w = Math.max(600, ...Object.values(points).map(p => p.x + width + 48));
  const h = Math.max(360, ...Object.values(points).map(p => p.y + height + 48));
  return <section className="flow-canvas" aria-label="Workflow canvas">
    <header><h2><GitBranch size={16}/>Workflow</h2><div>
      <button aria-label="Zoom out" disabled={zoom <= .2} onClick={() => setZoom(Math.max(.2, zoom - .15))}><Minus size={15}/></button>
      <output>{Math.round(zoom * 100)}%</output>
      <button aria-label="Zoom in" disabled={zoom >= 2} onClick={() => setZoom(Math.min(2, zoom + .15))}><Plus size={15}/></button>
      <button aria-label="Fit workflow to view" onClick={() => { if (viewport.current) { setZoom(Math.min(1, viewport.current.clientWidth / w, viewport.current.clientHeight / h)); viewport.current.scrollTo(0,0); } }}><Maximize2 size={15}/></button>
      {editable && <button aria-label="Auto-layout workflow" onClick={() => arrange(auto)}><Wand2 size={15}/></button>}
    </div></header>
    <div className="flow-viewport" ref={viewport} tabIndex={0} aria-label="Scrollable workflow"
      onPointerDown={e => { if (e.button || (e.target as Element).closest("button")) return; pan.current = {mouse:{x:e.clientX,y:e.clientY},x:e.currentTarget.scrollLeft,y:e.currentTarget.scrollTop}; e.currentTarget.setPointerCapture(e.pointerId); }}
      onPointerMove={e => { const p = pan.current; if (p) e.currentTarget.scrollTo(p.x+p.mouse.x-e.clientX,p.y+p.mouse.y-e.clientY); }}
      onLostPointerCapture={() => { pan.current = undefined; }}>
      <div style={{width:w*zoom,height:h*zoom,position:"relative",overflow:"hidden"}}><div style={{width:w,height:h,position:"absolute",transformOrigin:"top left",transform:'scale('+zoom+')'}}>
        <svg width={w} height={h} aria-hidden="true" className="flow-links"><defs><marker id="workflow-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M0 0L10 5L0 10" fill="#65857b"/></marker></defs>
        {nodes.flatMap(node => [{id:node.next,label:node.kind==="condition"?"Yes":""}, ...(node.options??[]).map(o=>({id:o.next,label:o.label})),{id:node.failure,label:node.kind==="condition"?"No":"Fallback"}].map((edge,index) => {
          const a=points[node.key], b=edge.id && points[edge.id]; if (!b) return null;
          const x=a.x+width,y=a.y+height/2,endY=b.y+height/2,bend=Math.max(42,Math.abs(b.x-x)*.45);
          return <g key={node.key+":"+index}><path d={`M${x} ${y} C${x+bend} ${y} ${b.x-bend} ${endY} ${b.x} ${endY}`} fill="none" stroke="#65857b" strokeDasharray={edge.label==="Fallback"?"4 4":undefined} markerEnd="url(#workflow-arrow)"/>{edge.label && <text x={(x+b.x)/2} y={(y+endY)/2-5} textAnchor="middle">{edge.label}</text>}</g>;
        }))}</svg>
        {nodes.map(node=><button type="button" key={node.key} className="flow-node" aria-pressed={selected===node.key} style={{left:points[node.key].x,top:points[node.key].y,width,height}} onClick={()=>select(node.key)}
          onPointerDown={e=>{if (!editable || e.button) return; const at=points[node.key]; e.currentTarget.setPointerCapture(e.pointerId); setDrag({key:node.key,origin:at,mouse:{x:e.clientX,y:e.clientY},at});}}
          onPointerMove={e=>{if (drag?.key!==node.key) return; const snap=(n:number)=>Math.min(10000,Math.max(0,Math.round(n/16)*16)); setDrag({...drag,at:{x:snap(drag.origin.x+(e.clientX-drag.mouse.x)/zoom),y:snap(drag.origin.y+(e.clientY-drag.mouse.y)/zoom)}});}}
          onPointerUp={e=>{if (drag?.key===node.key && Math.hypot(e.clientX-drag.mouse.x,e.clientY-drag.mouse.y)>4) arrange({[node.key]:drag.at}); setDrag(undefined);}}
          onPointerCancel={()=>setDrag(undefined)}>
          <small>{node.key===start?"Start · ":""}{node.kind.replace(/_/g," ")}</small><strong>{node.title}</strong>
        </button>)}
      </div></div>
    </div>
  </section>;
}
