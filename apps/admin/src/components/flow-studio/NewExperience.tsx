import { useEffect, useState } from "react";
import { ArrowLeft, ArrowRight, Plus } from "lucide-react";
import { Button, Flash, Modal } from "../ui";
import { WhatsAppThread } from "../whatsapp/WhatsAppThread";
import { creationModes, experienceMode } from "../../lib/experience-modes";
import { JourneyTemplates } from "../../pages/JourneyTemplates";
import "./flow-studio.css";

type Template = { id: string; name: string; description: string; mode?: string };
type Connection = { id: string; display_name: string; status: string };
export type NewExperienceInput = { template: string; name: string; mode: string; purpose: string; connection_id?: string };
const samples: Record<string, { from: "customer" | "bot"; message: unknown }[]> = {
  automated_commerce: [{ from: "bot", message: { type: "buttons", text: "Welcome! Ready to find something you love?", options: [{ id: "shop", label: "Browse products" }] } }],
  guided_questions: [{ from: "bot", message: { type: "buttons", text: "How can we help today?", options: [{ id: "order", label: "Track an order" }, { id: "team", label: "Talk to the team" }] } }],
  whatsapp_flow: [{ from: "bot", message: { type: "flow", text: "Complete your details in a few simple screens.", metadata: { flow_cta: "Open form" } } }],
  ai: [{ from: "customer", message: { type: "text", text: "Which product would suit a small workspace?" } }, { from: "bot", message: { type: "text", text: "I can help you compare the options in our catalogue." } }],
};
export function NewExperience({ open, onClose, templates, connections, busy, error, onCreate }: { open: boolean; onClose: () => void; templates: Template[]; connections: Connection[]; busy: boolean; error: string; onCreate: (input: NewExperienceInput) => void }) {
  const [stage, setStage] = useState(1), [mode, setMode] = useState("automated_commerce"), [name, setName] = useState(""), [purpose, setPurpose] = useState(""), [connection, setConnection] = useState(""), [template, setTemplate] = useState("order");
  useEffect(() => { if (!open) setStage(1); }, [open]);
  const available = templates.filter(item => !item.mode || item.mode === mode);
  const selectedTemplate = available.some(item => item.id === template) ? template : available[0]?.id || "";
  const close = () => { onClose(); setStage(1); };
  return <Modal open={open} title={stage === 1 ? "Choose an experience" : `New ${experienceMode(mode).label}`} description={stage === 1 ? "Choose how your customers will interact on WhatsApp." : "Give this draft a name, purpose and optional channel connection."} onClose={close} footer={stage === 1 ? <Button icon={ArrowRight} onClick={() => setStage(2)}>Continue</Button> : <><Button className="secondary" icon={ArrowLeft} onClick={() => setStage(1)}>Back</Button><Button icon={Plus} loading={busy} disabled={!name.trim() || !selectedTemplate} onClick={() => onCreate({ template: selectedTemplate, name: name.trim(), mode, purpose, connection_id: connection || undefined })}>Create draft</Button></>}>
    <Flash message={error}/>
    {stage === 1 ? <div className="flow-mode-cards">{creationModes.map(item => { const Icon = item.icon; return <div className={`flow-mode-card ${mode === item.id ? "selected" : ""}`} key={item.id}><button type="button" aria-pressed={mode === item.id} onClick={() => { setMode(item.id); setTemplate(({ automated_commerce: "order", guided_questions: "custom", whatsapp_flow: "order", ai: "product_enquiry" })[item.id]); }}><Icon size={22} style={{ color: item.accent }}/><strong>{item.label}</strong><span>{item.description}</span><small>Best for: {item.bestFor}</small></button><div className="flow-mode-example" aria-label={`${item.label} example`}><small>Example · static sample</small><WhatsAppThread lines={samples[item.id]}/></div></div>; })}</div> : <div className="flow-new-form"><label>Name<input autoFocus value={name} maxLength={120} placeholder="Customer ordering" onChange={event => setName(event.target.value)}/></label><label>Purpose<textarea value={purpose} placeholder="What should this experience help customers do?" onChange={event => setPurpose(event.target.value)}/></label><label>Channel connection<select value={connection} onChange={event => setConnection(event.target.value)}><option value="">Choose later · unbound preview</option>{connections.filter(item => item.status !== "archived").map(item => <option value={item.id} key={item.id}>{item.display_name}</option>)}</select></label><details open><summary>Start from a template</summary><JourneyTemplates mode={mode} templates={available} value={selectedTemplate} onChange={setTemplate}/></details></div>}
  </Modal>;
}
