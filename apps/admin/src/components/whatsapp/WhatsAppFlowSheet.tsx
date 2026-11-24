import { useCallback, useEffect, useState, type FormEvent } from "react";
import { ImageIcon } from "lucide-react";
import { parseFlowScreens, previewURL, type FlowComponent } from "./model";
import { PreviewSheet } from "./PreviewSheet";
import "./whatsapp-preview.css";

type Props = { compilation?: Record<string, unknown>; screenId?: string; prompt?: string; options?: { id: string; label: string }[]; busy: boolean; submit: (id: string, label: string) => void; onClose?: () => void; resetKey?: string };
const text = (value: unknown) => typeof value === "string" ? value : "";
export function WhatsAppFlowSheet({ compilation, screenId, prompt, options, busy, submit, onClose, resetKey }: Props) {
  const screens = parseFlowScreens(compilation);
  const [localScreen, setLocalScreen] = useState<string>();
  const [values, setValues] = useState<Record<string, string | string[] | boolean>>({});
  const [open, setOpen] = useState(true);
  const [validation, setValidation] = useState("");
  useEffect(() => { setValues({}); setLocalScreen(undefined); setValidation(""); setOpen(true); }, [screenId, prompt, resetKey]);
  const close = useCallback(() => { setOpen(false); onClose?.(); }, [onClose]);
  const screen = screens.find(item => item.id === (localScreen ?? screenId)) ?? screens[0];
  const flatten = (items: FlowComponent[]): FlowComponent[] => items.flatMap(item => item.type === "Form" ? flatten(item.children ?? []) : [item]);
  const components = flatten(screen?.layout.children ?? []);
  const footer = components.find(item => item.type === "Footer");
  const resolve = (value: unknown) => {
    const source = text(value);
    if (source === "${data.prompt}") return prompt ?? "";
    if (source === "${data.error}") return "";
    if (source.startsWith("${")) return "";
    return source;
  };
  const choiceSource = (component: FlowComponent) => Array.isArray(component["data-source"]) ? (component["data-source"] as Record<string, unknown>[]).map(row => ({ id: text(row.id), label: text(row.title) || text(row.label) })) : options ?? [];
  const update = (name: string, value: string | string[] | boolean) => setValues(old => ({ ...old, [name]: value }));
  function render(component: FlowComponent, index: number) {
    const name = text(component.name) || `field-${index}`;
    const label = resolve(component.label);
    const value = values[name];
    const required = component.required === true;
    const common = { name, required, disabled: busy };
    switch (component.type) {
      case "TextHeading": return <h2 key={index}>{resolve(component.text)}</h2>;
      case "TextSubheading": return <h3 key={index}>{resolve(component.text)}</h3>;
      case "TextCaption": case "TextBody": { const body = resolve(component.text); return body ? <p key={index}>{body}</p> : null; }
      case "TextInput": case "TextArea": case "DatePicker": return <label key={index}>{label}{required && " *"}{component.type === "TextArea" ? <textarea {...common} value={typeof value === "string" ? value : ""} onChange={e => update(name, e.target.value)}/> : <input {...common} type={component.type === "DatePicker" ? "date" : ["email", "number", "password"].includes(text(component["input-type"])) ? text(component["input-type"]) : component["input-type"] === "phone" ? "tel" : "text"} value={typeof value === "string" ? value : ""} onChange={e => update(name, e.target.value)}/>}</label>;
      case "Dropdown": return <label key={index}>{label}{required && " *"}<select {...common} value={typeof value === "string" ? value : ""} onChange={e => update(name, e.target.value)}><option value="">Select an option</option>{choiceSource(component).map(option => <option key={option.id} value={option.id}>{option.label}</option>)}</select></label>;
      case "RadioButtonsGroup": case "CheckboxGroup": return <fieldset key={index}><legend>{label}{required && " *"}</legend>{choiceSource(component).map(option => <label className="wa-flow-option" key={option.id}><span>{option.label}</span><input name={name} disabled={busy} type={component.type === "CheckboxGroup" ? "checkbox" : "radio"} required={component.type === "RadioButtonsGroup" && required} checked={Array.isArray(value) ? value.includes(option.id) : value === option.id} onChange={e => update(name, component.type === "CheckboxGroup" ? e.target.checked ? [...(Array.isArray(value) ? value : []), option.id] : (Array.isArray(value) ? value : []).filter(id => id !== option.id) : option.id)}/></label>)}</fieldset>;
      case "OptIn": return <label className="wa-flow-option" key={index}><span>{label}</span><input {...common} type="checkbox" checked={value === true} onChange={e => update(name, e.target.checked)}/></label>;
      case "EmbeddedLink": {
        const action = component["on-click-action"] as Record<string, unknown> | undefined;
        const next = action?.next as Record<string, unknown> | undefined;
        const target = typeof next?.name === "string" ? next.name : undefined;
        const url = action?.name === "open_url" ? previewURL(action.url) : undefined;
        if (url) return <a className="wa-action" key={index} href={url} target="_blank" rel="noopener noreferrer">{resolve(component.text) || label}</a>;
        return <button className="wa-action" key={index} type="button" disabled={busy || !target || !screens.some(item => item.id === target)} onClick={() => { setLocalScreen(target); setValues({}); }}>{resolve(component.text) || label}</button>;
      }
      case "Image": return <div key={index} className="wa-media" role="img" aria-label={text(component["alt-text"]) || "Flow image"}><ImageIcon size={36}/><small>{text(component["alt-text"]) || "Image preview"}</small></div>;
      case "Footer": return null;
      default: return <p className="wa-notice" key={index}>This Flow component is unavailable in preview.</p>;
    }
  }
  function advance(event: FormEvent) {
    event.preventDefault();
    const missingGroup = components.find(component => component.type === "CheckboxGroup" && component.required === true && (!Array.isArray(values[text(component.name)]) || !(values[text(component.name)] as string[]).length));
    if (missingGroup) { setValidation(`Choose at least one option for ${resolve(missingGroup.label) || "the required field"}.`); return; }
    setValidation("");
    const action = footer?.["on-click-action"] as Record<string, unknown> | undefined;
    const next = action?.next as Record<string, unknown> | undefined;
    if (action?.name === "navigate" && typeof next?.name === "string" && screens.some(item => item.id === next.name)) { setLocalScreen(next.name); setValues({}); return; }
    if (action?.name === "complete") { close(); return; }
    const answer = values.answer ?? Object.values(values)[0];
    const id = Array.isArray(answer) ? answer.join(", ") : answer === undefined ? "" : String(answer);
    const option = components.flatMap(choiceSource).find(item => item.id === id);
    submit(id, option?.label ?? id);
  }
  if (!open) return <div className="wa-flow-reopen"><button type="button" onClick={() => setOpen(true)}>Open Flow preview</button></div>;
  return <PreviewSheet title={screen?.title || "WhatsApp Flow"} full onClose={close}><form className="wa-flow-form" onSubmit={advance} data-testid="compiled-flow-screen" data-screen={screen?.id}><div className="wa-flow-body">{validation && <p role="alert">{validation}</p>}{components.length ? components.map(render) : <><h2>{screen?.title || "Request received"}</h2><p>{prompt}</p>{options?.map(option => <button type="button" disabled={busy} key={option.id} onClick={() => submit(option.id, option.label)}>{option.label}</button>)}</>}</div>{footer && <div className="wa-flow-footer"><button type="submit" disabled={busy}>{resolve(footer.label) || "Continue"}</button><small>WhatsApp Flow · preview only</small></div>}</form></PreviewSheet>;
}
