import { useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { ChevronRight, GitBranch, MessageSquare, ShoppingBag, Smartphone } from "lucide-react";
import { Button, Drawer, EmptyState } from "../ui";
import "./flow-studio.css";

type EditableStep = { key: string; kind: string; title: string; text: string; help: string; required: boolean; locked: boolean; answer_type: string; source: string; options: { id: string; label: string; value: string; next?: string }[]; next: string; failure: string };
type EditableDefinition = { name: string; mode: string; purpose: string; steps: EditableStep[] };
const questions = new Set(["question", "buttons", "list", "entry_menu", "store_selector", "category_selector", "product_selector", "variant_selector", "fulfilment_selector", "quantity", "address"]);
const groups = [
  { label: "Store", kinds: ["store_selector"] },
  { label: "Catalogue", kinds: ["category_selector", "product_selector", "variant_selector", "quantity", "check_inventory"] },
  { label: "Checkout", kinds: ["add_to_cart", "review_cart", "fulfilment_selector", "address", "create_order", "payment", "payment_status"] },
  { label: "Handoff", kinds: ["handoff", "notification", "end"] },
];

export function ModeEditor<T extends EditableDefinition>({ draft, editable, onChange, onSelectStep, selected, advanced, inspector, inspectorOpen, onInspectorClose }: {
  draft: T; editable: boolean; onChange: (next: T) => void; onSelectStep: (key: string) => void;
  selected: string; advanced: ReactNode; inspector: ReactNode; inspectorOpen?: boolean; onInspectorClose?: () => void;
}) {
  const [inspecting, setInspecting] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const step = draft.steps.find(item => item.key === selected);
  const patch = (key: string, values: Partial<EditableStep>) => onChange({ ...draft, steps: draft.steps.map(item => item.key === key ? { ...item, ...values } : item) });
  const inspect = (key: string) => { onSelectStep(key); setInspecting(true); };
  const guided = ["guided_questions", "guided_chat"].includes(draft.mode);
  const screens = draft.steps.filter(item => questions.has(item.kind) && item.kind !== "entry_menu");
  const screen = screens.find(item => item.key === selected) ?? screens[0];
  return <div className={`flow-mode-editor mode-${draft.mode}`}>
    <div className="flow-mode-heading"><div>{draft.mode === "whatsapp_flow" ? <Smartphone size={19}/> : draft.mode === "automated_commerce" ? <ShoppingBag size={19}/> : <MessageSquare size={19}/>}<h2>{guided ? "Conversation canvas" : draft.mode === "whatsapp_flow" ? "Flow screens" : draft.mode === "ai" ? "AI conversation" : "Commerce settings"}</h2></div>{step && <Button className="secondary" onClick={() => setInspecting(true)}>Inspect step</Button>}</div>
    {guided ? advanced : draft.mode === "automated_commerce" ? <>
      <p className="muted">Configure the customer prompts around your current store, catalogue and checkout. Protected actions continue to use live business data.</p>
      {groups.map(group => <section className="flow-commerce-group" key={group.label}><h3>{group.label}</h3>{draft.steps.filter(item => group.kinds.includes(item.kind)).map(item => <div className="flow-config-step" key={item.key}><div><strong>{item.title}</strong><button type="button" onClick={() => inspect(item.key)} aria-label={`Inspect ${item.title}`}><ChevronRight size={17}/></button></div><fieldset disabled={!editable}><label>Customer message<textarea value={item.text} onChange={event => patch(item.key, { text: event.target.value })}/></label>{item.source && <p className="muted">Uses current {item.source.replace(/_/g, " ")}</p>}{questions.has(item.kind) && <label>Help text<input value={item.help} onChange={event => patch(item.key, { help: event.target.value })}/></label>}</fieldset></div>)}{!draft.steps.some(item => group.kinds.includes(item.kind)) && <p className="muted">This draft has no {group.label.toLowerCase()} steps. Add them in Advanced.</p>}</section>)}
    </> : draft.mode === "whatsapp_flow" ? <>
      {draft.steps.some(item => item.kind === "entry_menu") && <p role="status">Entry menus cannot compile into native Flow screens. Keep the entry menu in guided chat and launch a separately published Flow.</p>}
      <p className="muted">Each customer question compiles into a native WhatsApp screen. Titles, prompts, help and input types below are saved in the journey definition.</p>
      <div className="flow-screen-editor"><nav aria-label="Flow screens">{screens.map((item, index) => <button key={item.key} type="button" aria-current={screen?.key === item.key ? "step" : undefined} onClick={() => onSelectStep(item.key)}><span>{index + 1}</span>{item.title}</button>)}</nav>{screen ? <fieldset disabled={!editable} className="flow-screen-fields"><label>Screen title<input value={screen.title} maxLength={80} onChange={event => patch(screen.key, { title: event.target.value })}/></label><label>Prompt<textarea value={screen.text} maxLength={4096} onChange={event => patch(screen.key, { text: event.target.value })}/></label><label>Help text<input value={screen.help} onChange={event => patch(screen.key, { help: event.target.value })}/></label><label className="flow-check"><input type="checkbox" checked={screen.required} onChange={event => patch(screen.key, { required: event.target.checked })}/>Required answer</label><label>Input component<select className="flow-select" disabled={Boolean(screen.source || screen.options?.length || screen.kind === "quantity")} value={screen.source || screen.options?.length ? "dropdown" : screen.kind === "quantity" ? "number" : ["text", "number", "email", "phone", "date", "yes_no"].includes(screen.answer_type) ? screen.answer_type : "text"} onChange={event => patch(screen.key, { answer_type: event.target.value })}>{screen.source || screen.options?.length ? <option value="dropdown">Dropdown from options</option> : [ ["text", "Text input"], ["number", "Number input"], ["email", "Email input"], ["phone", "Phone input"], ["date", "Date picker"], ["yes_no", "Yes / no buttons"] ].map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label><Button className="secondary" onClick={() => inspect(screen.key)}>Options, validation and routing</Button></fieldset> : <EmptyState title="No input screens yet" body="Add a customer question in Advanced to create the first screen."/>}</div>
    </> : <>
      <section className="flow-commerce-group"><h3>Conversation purpose</h3><fieldset disabled={!editable}><label>Purpose<textarea value={draft.purpose} onChange={event => onChange({ ...draft, purpose: event.target.value })}/></label></fieldset><p className="muted">This is the experience description. Persona, instructions and editable AI guardrails are not available in the journey API.</p></section>
      <section className="flow-commerce-group"><h3>Business knowledge</h3><p>The grounded assistant uses your existing business knowledge and catalogue.</p><Link to="/knowledge">Open Business knowledge</Link></section>
      <section className="flow-commerce-group"><h3>Fallback and human handoff</h3><p className="muted">The assistant uses the existing journey when a guided response or team handoff is needed.</p>{draft.steps.filter(item => ["handoff", "message", "end"].includes(item.kind)).map(item => <div className="flow-config-step" key={item.key}><strong>{item.title}</strong><fieldset disabled={!editable}><label>Customer message<textarea value={item.text} onChange={event => patch(item.key, { text: event.target.value })}/></label></fieldset><Button className="secondary" onClick={() => inspect(item.key)}>Routing and fallback</Button></div>)}<Link to="/assistant/ai-chat">Open AI test workspace</Link></section>
    </>}
    {!guided && <div className="flow-advanced"><Button className="secondary" icon={GitBranch} onClick={() => setAdvancedOpen(value => !value)} aria-expanded={advancedOpen}>Advanced · steps and routing</Button>{advancedOpen && advanced}</div>}
    <Drawer open={(inspectorOpen || inspecting) && Boolean(step)} title={step?.title || "Step inspector"} onClose={() => { setInspecting(false); onInspectorClose?.(); }}><div className="flow-inspector">{inspector}</div></Drawer>
  </div>;
}
