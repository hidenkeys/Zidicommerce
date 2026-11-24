import type { ReactNode } from "react";
import { ArrowDown, ArrowUp, Copy, Lock, Plus, Trash2 } from "lucide-react";
import { Badge, EmptyState, IconButton } from "../ui";
import type { Definition, Step } from "./types";

const friendly = (value: string) => value.replace(/_/g, " ");
const questionKinds = ["question", "buttons", "list", "entry_menu", "store_selector", "category_selector", "product_selector", "variant_selector", "quantity", "fulfilment_selector", "address"];
const protectedKinds = ["check_inventory", "add_to_cart", "review_cart", "create_order", "payment", "payment_status", "notification", "complaint_action", "track_action"];
const destinationLabel = (value: string | undefined, steps: Step[]) => steps.find(s => s.destination === value)?.title ?? friendly(value ?? "");

export function StepInspector({ draft, step, editable, canPublish, patchStep, route, onDuplicate, onDelete }: {
  draft: Definition;
  step?: Step;
  editable: boolean;
  canPublish: boolean;
  patchStep: (patch: Partial<Step>) => void;
  route: (label: string, value: string | undefined, onChange: (value: string) => void) => ReactNode;
  onDuplicate: () => void;
  onDelete: () => void;
}) {
  const fieldOptions = draft.steps.filter(s => s.destination).map(s => ({ value: s.destination!, label: s.title }));
  return (<section className="journey-editor" aria-label="Step editor">
          {step ? <><div className="journey-section-heading"><h2>{step.title || "Untitled step"}</h2><div className="page-actions">{step.locked ? <Badge><Lock size={12} />Protected</Badge> : editable ? <><IconButton label="Duplicate step" icon={Copy} onClick={onDuplicate} /><IconButton label="Delete step" icon={Trash2} disabled={draft.steps.length === 1} onClick={onDelete} /></> : null}</div></div>
            <fieldset disabled={!editable}>
              <label>Step name<input value={step.title} maxLength={80} onChange={e => patchStep({ title: e.target.value })} /></label>
              <label>Customer message<textarea aria-label="Customer message" value={step.text} rows={4} maxLength={4096} onChange={e => patchStep({ text: e.target.value })} /></label>
              {questionKinds.includes(step.kind) ? <>
                <label>Help text<input value={step.help} onChange={e => patchStep({ help: e.target.value })} /></label>
                <label className="journey-check"><input type="checkbox" checked={step.required} onChange={e => patchStep({ required: e.target.checked })} />Required answer</label>
                {!step.source && !["quantity", "address"].includes(step.kind) ? <label>Answer type<select value={step.answer_type} onChange={e => patchStep({ answer_type: e.target.value })}>{["text", "single_choice", "yes_no", "number", "email", "phone", "date", "location", "media"].map(value => <option key={value} value={value}>{friendly(value)}</option>)}</select></label> : null}
                <label>Presentation<select value={step.presentation} onChange={e => patchStep({ presentation: e.target.value })}><option value="free_text">Typed answer</option><option value="buttons">Buttons</option><option value="list">List</option></select></label>
                {step.source ? <label>Data source<input value={`Current ${friendly(step.source)}`} readOnly /></label> : null}
                {!step.source ? <div className="journey-options"><div className="journey-section-heading"><h3>Options</h3><IconButton label="Add option" icon={Plus} disabled={(step.options ?? []).length >= 10} onClick={() => patchStep({ options: [...(step.options ?? []), { id: `option_${crypto.randomUUID().slice(0, 8)}`, label: "New option", value: `choice_${(step.options ?? []).length + 1}` }] })} /></div>{(step.options ?? []).map((option, index) => <div className="journey-option" key={option.id}><label>Label<input value={option.label} maxLength={72} onChange={e => patchStep({ options: step.options.map((o, i) => i === index ? { ...o, label: e.target.value } : o) })} /></label><label>Saved answer<input value={option.value} maxLength={120} onChange={e => patchStep({ options: step.options.map((o, i) => i === index ? { ...o, value: e.target.value } : o) })} /></label>{route("Then go to", option.next, value => patchStep({ options: step.options.map((o, i) => i === index ? { ...o, next: value } : o) }))}<div className="page-actions"><IconButton label={`Move option ${index + 1} up`} icon={ArrowUp} disabled={index === 0} onClick={() => { const options = [...step.options]; [options[index - 1], options[index]] = [options[index], options[index - 1]]; patchStep({ options }); }} /><IconButton label={`Move option ${index + 1} down`} icon={ArrowDown} disabled={index === step.options.length - 1} onClick={() => { const options = [...step.options]; [options[index + 1], options[index]] = [options[index], options[index + 1]]; patchStep({ options }); }} /><IconButton label={`Remove option ${index + 1}`} icon={Trash2} onClick={() => patchStep({ options: step.options.filter((_, i) => i !== index) })} /></div></div>)}</div> : null}
                <div className="journey-field-pair"><label>Minimum<input type="number" value={step.min ?? ""} onChange={e => patchStep({ min: e.target.value === "" ? undefined : Number(e.target.value) })} /></label><label>Maximum<input type="number" value={step.max ?? ""} onChange={e => patchStep({ max: e.target.value === "" ? undefined : Number(e.target.value) })} /></label></div>
                <label>Maximum answer length<input type="number" min={0} max={4096} value={step.max_length} onChange={e => patchStep({ max_length: Number(e.target.value) })} /></label>
                <label>Retry message<input value={step.retry_message} onChange={e => patchStep({ retry_message: e.target.value })} /></label>
                <label>Maximum retries<input type="number" min={1} max={10} value={step.max_retries} onChange={e => patchStep({ max_retries: Number(e.target.value) })} /></label>
              </> : null}
              {step.kind === "condition" ? <><label>Answer to check<select value={step.condition_field} disabled={step.locked} onChange={e => patchStep({ condition_field: e.target.value })}><option value="">Choose an answer</option>{fieldOptions?.map(field => <option key={field.value} value={field.value}>{field.label}</option>)}{step.condition_field && !fieldOptions?.some(f => f.value === step.condition_field) ? <option value={step.condition_field}>{destinationLabel(step.condition_field, draft.steps)}</option> : null}</select></label><label>Comparison<select value={step.condition_operator ?? "equals"} disabled={step.locked} onChange={e => patchStep({ condition_operator: e.target.value })}><option value="equals">Is equal to</option><option value="not_equals">Is not equal to</option><option value="contains">Contains</option></select></label><label>Expected answer<input value={step.condition_value ?? ""} disabled={step.locked} onChange={e => patchStep({ condition_value: e.target.value })} /></label></> : null}
              {route(step.kind === "condition" ? "When matched" : "Next step", step.next, next => patchStep({ next }))}
              {route(step.kind === "condition" ? "When not matched" : "Failure or handoff", step.failure, failure => patchStep({ failure }))}
              {!step.locked && !protectedKinds.includes(step.kind) ? <label>Show only after an answer<select value={step.visible_when ?? ""} onChange={e => patchStep({ visible_when: e.target.value })}><option value="">Always</option>{fieldOptions?.map(field => <option key={field.value} value={field.value}>{field.label}</option>)}</select></label> : null}
              <label>Analytics label<input value={step.analytics_label} onChange={e => patchStep({ analytics_label: e.target.value })} /></label>
              {canPublish ? <details className="journey-advanced"><summary>Advanced</summary><label>Stable step identifier<input value={step.key} readOnly /></label><label>Answer destination<input value={step.destination ?? ""} readOnly={Boolean(step.source) || protectedKinds.includes(step.kind)} onChange={e => patchStep({ destination: e.target.value })} /></label></details> : null}
            </fieldset>
          </> : <EmptyState title="Select a step" />}
        </section>);
}
