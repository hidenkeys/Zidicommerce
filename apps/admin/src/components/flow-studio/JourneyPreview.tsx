import { ArrowLeft, MessageSquare, Play, RotateCcw, Send, Smartphone } from "lucide-react";
import { Badge, Button, Flash, IconButton } from "../ui";
import { WhatsAppPhone } from "../whatsapp/WhatsAppPhone";
import { WhatsAppThread } from "../whatsapp/WhatsAppThread";
import { JourneyFlowPreview } from "../../pages/JourneyFlowPreview";
import { previewScenarios } from "../../lib/experience-modes";
import type { Compilation, Definition, Preview, PreviewLine } from "./types";

export function JourneyPreview({ draft, preview, lines, busy, dirty, previewMode, onPreviewMode, scenario, onScenario, compilation, answer, onAnswer, onStart, onSend, canControlledTest, onControlledTest, error }: {
  draft: Definition; preview?: Preview; lines: PreviewLine[]; busy: boolean; dirty: boolean;
  previewMode: string; onPreviewMode: (mode: string) => void; scenario: string; onScenario: (scenario: string) => void;
  compilation?: Compilation; answer: string; onAnswer: (answer: string) => void; onStart: () => void;
  onSend: (text: string, label?: string, command?: string) => void;
  canControlledTest: boolean; onControlledTest: () => void; error: string;
}) {
  const friendly = (value: string) => value.replace(/_/g, " ");
  const currentPrompt = preview?.result.messages[preview.result.messages.length - 1];
  const questionActive = preview?.result.status === "active" && !(previewMode === "flow" && compilation?.errors.length);
  const flowPrompt = preview?.result.messages.map(message => message.text).filter(Boolean).join("\n\n");
  return (<section className="journey-preview" aria-label="Interactive journey preview"><Flash message={error}/><p className="muted">Simulation · save draft changes to test them.</p>
          <div className="journey-section-heading"><h2>Preview</h2><IconButton label="Restart preview" icon={RotateCcw} disabled={busy || dirty} onClick={onStart} /></div>
          <div className="journey-preview-switch" role="group" aria-label="Preview presentation"><button type="button" aria-pressed={previewMode === "chat"} className={previewMode === "chat" ? "active" : ""} onClick={() => onPreviewMode("chat")}><MessageSquare size={15} />Chat</button>{draft.mode === "whatsapp_flow" ? <button type="button" aria-pressed={previewMode === "flow"} className={previewMode === "flow" ? "active" : ""} onClick={() => onPreviewMode("flow")}><Smartphone size={15} />Flow</button> : null}</div>

          <label className="journey-scenario">Scenario<select aria-label="Preview scenario" value={scenario} onChange={event => { onScenario(event.target.value); }}>{previewScenarios.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
          <WhatsAppPhone businessName={draft.name} label={preview?.controlled_test ? "Test records" : "Preview only"}>
            {preview ? previewMode === "chat" ? <WhatsAppThread lines={preview.result.handoff ? [...lines, { from: "bot", message: { type: "notice", text: "Conversation handed to the team" } }] : lines} busy={busy} active={questionActive} onReply={(id, label) => onSend(id, label)} onOpenFlow={draft.mode === "whatsapp_flow" ? () => onPreviewMode("flow") : undefined} /> : compilation?.errors.length ? <Flash message="This journey cannot be rendered as a Flow. Resolve the compilation errors first." /> : <JourneyFlowPreview resetKey={preview.turn} compilation={compilation?.json} screenId={preview.flow_screen} prompt={flowPrompt} options={currentPrompt?.options} busy={busy} submit={(id, label) => onSend(id, label)} onClose={() => onPreviewMode("chat")} /> : <div className="journey-preview-empty"><Button icon={Play} disabled={busy || dirty} onClick={onStart}>Start preview</Button></div>}
            {previewMode === "chat" && questionActive && !currentPrompt?.options?.length ? <form className="journey-answer" onSubmit={e => { e.preventDefault(); onSend(answer); }}><input aria-label="Customer answer" value={answer} disabled={busy} type={currentPrompt?.metadata?.answer_type === "date" ? "date" : currentPrompt?.metadata?.answer_type === "email" ? "email" : "text"} onChange={e => onAnswer(e.target.value)} /><IconButton label="Send preview answer" icon={Send} type="submit" disabled={busy || !answer.trim()} /></form> : null}
          </WhatsAppPhone>
          {preview ? <><div className="journey-preview-controls"><IconButton label="Back in preview" icon={ArrowLeft} disabled={busy} onClick={() => onSend("back", "Back", "back")} /><IconButton label="Restart journey" icon={RotateCcw} disabled={busy} onClick={() => onSend("restart", "Restart", "restart")} /><Button className="secondary" disabled={busy} onClick={() => onSend("cancel", "Cancel", "cancel")}>Cancel</Button><Badge>{friendly(preview.result.status)}</Badge></div><p className="muted journey-current">{draft.steps.find(s => s.key === preview.current_step)?.title ?? "Finished"}</p></> : null}
          {preview?.variables && <details className="journey-vars"><summary>Test variables</summary>{Object.entries(preview.variables).length ? <dl>{Object.entries(preview.variables).map(([key,value])=><div key={key}><dt>{key}</dt><dd>{typeof value==="string"?value:JSON.stringify(value)}</dd></div>)}</dl> : <p>No answers recorded yet.</p>}</details>}
          {preview?.cart?.length ? <div className="journey-cart"><h3>Preview cart</h3>{preview.cart.map((item, index) => <p key={index}>{item.quantity} × {item.name}<strong>{new Intl.NumberFormat("en-NG", { style: "currency", currency: item.currency ?? "NGN" }).format(item.quantity * item.unit_price_minor / 100)}</strong></p>)}</div> : null}
          {previewMode === "flow" && compilation?.errors.length ? <Flash message={compilation.errors.map(i => i.message).join(" ")} /> : null}
          {previewMode === "flow" && compilation?.warnings.length ? <details><summary>Flow warnings ({compilation.warnings.length})</summary>{compilation.warnings.map((warning, index) => <p key={index}>{warning.message}</p>)}</details> : null}
          {canControlledTest ? <Button className="secondary" disabled={busy || dirty} onClick={onControlledTest}>Controlled record test</Button> : null}
        </section>);
}
