import { useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowLeft, ArrowUp, Copy, GitBranch, GripVertical, History, Lock, MessageSquare, Pause, Play, Plus, Redo2, RotateCcw, Save, Send, Settings2, ShieldCheck, Smartphone, Trash2, Undo2 } from "lucide-react";
import { apiDelete, apiGet, apiPost, apiPut } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Button, EmptyState, Flash, IconButton, LoadingState, Modal, Page, Tabs } from "../components/ui";
import { hasPermission } from "../lib/permissions";
import "./journeys.css";
import { JourneyFlowPreview } from "./JourneyFlowPreview";

type Option = { id: string; label: string; value: string; next?: string };
type Step = { key: string; kind: string; title: string; text: string; help: string; required: boolean; locked: boolean; answer_type: string; presentation: string; destination?: string; source: string; options: Option[]; min?: number; max?: number; max_length: number; retry_message: string; max_retries: number; next: string; failure: string; condition_field?: string; condition_operator?: string; condition_value?: string; visible_when?: string; analytics_label: string };
type Definition = { name: string; purpose: string; category: string; mode: string; channel: string; connection_id?: string; trigger: string; start: string; ttl_minutes: number; steps: Step[] };
type Version = { id: string; version_number: number; status: string; published_at?: string };
type Journey = { id: string; name: string; description: string; published_version_id?: string; status: string };
type Deployment = { status: string; revision: number; errors: string; warnings: string; preview_url?: string; provider_flow_id: string };
type Detail = { journey: Journey; version: Version; definition: Definition; versions: Version[]; binding?: { paused: boolean; mode: string; version_id: string }; deployment?: Deployment };
type Issue = { path: string; message: string };
type Validation = { valid: boolean; issues: Issue[] };
type Compilation = { json: Record<string, unknown>; errors: Issue[]; warnings: Issue[] };
type Message = { type: string; text: string; options?: { id: string; label: string }[]; metadata?: { answer_type?: string; help?: string } };
type Preview = { session_id: string; current_step: string; turn: string; flow_screen: string; controlled_test?: boolean; result: { status: string; messages: Message[]; handoff?: boolean }; cart: { name: string; quantity: number; unit_price_minor: number; currency: string }[] };
type Library = { kind: string; name: string; answer_type?: string };
type Connection = { id: string; provider: string; display_name: string; status: string };
type Template = { id: string; name: string; description: string };
type PreviewLine = { from: "customer" | "bot"; message: Message };

const friendly = (value: string) => value.replace(/_/g, " ");
const questionKinds = ["question", "buttons", "list", "entry_menu", "store_selector", "category_selector", "product_selector", "variant_selector", "quantity", "fulfilment_selector", "address"];
const protectedKinds = ["check_inventory", "add_to_cart", "review_cart", "create_order", "payment", "payment_status", "notification", "complaint_action", "track_action"];
const keyFor = () => `step_${crypto.randomUUID().replace(/-/g, "").slice(0, 12)}`;
const destinationLabel = (value: string | undefined, steps: Step[]) => steps.find(s => s.destination === value)?.title ?? friendly(value ?? "");

export function JourneysPage() {
  const { user } = useAuth();
  const canEdit = hasPermission(user.role, "bot.manage");
  const canPublish = hasPermission(user.role, "bot.publish");
  const canDeploy = canPublish && hasPermission(user.role, "channels.manage");
  const [rows, setRows] = useState<Journey[]>([]);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [library, setLibrary] = useState<Library[]>([]);
  const [connections, setConnections] = useState<Connection[]>([]);
  const [detail, setDetail] = useState<Detail>();
  const [draft, setDraft] = useState<Definition>();
  const [saved, setSaved] = useState("");
  const [selected, setSelected] = useState("");
  const [past, setPast] = useState<Definition[]>([]);
  const [future, setFuture] = useState<Definition[]>([]);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [newOpen, setNewOpen] = useState(false);
  const [newTemplate, setNewTemplate] = useState("order");
  const [newName, setNewName] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [confirmAction, setConfirmAction] = useState<{ title: string; body: string; run: () => Promise<void> }>();
  const [tab, setTab] = useState("edit");
  const [addKind, setAddKind] = useState("message");
  const [issues, setIssues] = useState<Issue[]>([]);
  const [preview, setPreview] = useState<Preview>();
  const [lines, setLines] = useState<PreviewLine[]>([]);
  const [answer, setAnswer] = useState("");
  const [previewMode, setPreviewMode] = useState("chat");
  const [scenario, setScenario] = useState("normal");
  const [compilation, setCompilation] = useState<Compilation>();
  const [dragged, setDragged] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const dirty = Boolean(draft && JSON.stringify(draft) !== saved);
  const editable = canEdit && (detail?.version.status === "draft" || detail?.version.status === "validated") && detail?.deployment?.status !== "published" && detail?.deployment?.status !== "deprecated";
  const step = draft?.steps.find(s => s.key === selected);

  async function run(work: () => Promise<void>) {
    setBusy(true); setError(""); setNotice("");
    try { await work(); } catch (e) { setError(e instanceof Error ? e.message : "This action could not be completed."); }
    finally { setBusy(false); }
  }
  async function loadList() { const response = await apiGet<Journey[]>("/journeys"); setRows(response.data); }
  function accept(next: Detail) {
    setDetail(next); setDraft(next.definition); setSaved(JSON.stringify(next.definition)); setPast([]); setFuture([]);
    setSelected(next.definition.steps[0]?.key ?? ""); setIssues([]); setPreview(undefined); setLines([]); setCompilation(undefined);
  }
  async function openJourney(id: string, version?: string) { accept((await apiGet<Detail>(`/journeys/${id}${version ? `?version=${version}` : ""}`)).data); }
  function leaving(work: () => Promise<void>) {
    if (dirty) setConfirmAction({ title: "Discard unsaved changes?", body: "The last saved draft will remain available.", run: work });
    else void run(work);
  }
  useEffect(() => { void run(async () => {
    await loadList();
    setTemplates((await apiGet<Template[]>("/journeys/templates")).data);
    setLibrary((await apiGet<Library[]>("/journeys/step-library")).data);
    if (hasPermission(user.role, "channels.view")) setConnections((await apiGet<Connection[]>("/channels")).data);
  }).finally(() => setLoading(false)); }, [user.organization_id]);
  useEffect(() => {
    const handler = (event: BeforeUnloadEvent) => { if (dirty) { event.preventDefault(); event.returnValue = ""; } };
    const navigate = (event: MouseEvent) => {
      const link = event.target instanceof Element ? event.target.closest("a[href]") as HTMLAnchorElement | null : null;
      if (!dirty || !link || link.target === "_blank" || event.ctrlKey || event.metaKey || event.button !== 0) return;
      if (!window.confirm("Discard unsaved journey changes?")) { event.preventDefault(); event.stopPropagation(); }
    };
    window.addEventListener("beforeunload", handler);
    document.addEventListener("click", navigate, true);
    return () => { window.removeEventListener("beforeunload", handler); document.removeEventListener("click", navigate, true); };
  }, [dirty]);
  useEffect(() => { scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" }); }, [lines]);

  function change(next: Definition) {
    if (!draft || !editable) return;
    setPast(value => [...value.slice(-39), draft]); setFuture([]); setDraft(next); setIssues([]);
    setPreview(undefined); setLines([]); setCompilation(undefined);
  }
  function patchStep(patch: Partial<Step>) { if (draft && step) change({ ...draft, steps: draft.steps.map(s => s.key === step.key ? { ...s, ...patch } : s) }); }
  function move(from: number, to: number) {
    if (!draft || !editable || from === to || to < 0 || to >= draft.steps.length || draft.steps[from].locked) return;
    const steps = [...draft.steps]; const [moving] = steps.splice(from, 1); steps.splice(to, 0, moving); change({ ...draft, steps });
  }
  function addStep() {
    if (!draft || !editable) return;
    const key = keyFor();
    const item = library.find(l => `${l.kind}${l.answer_type ? `:${l.answer_type}` : ""}` === addKind);
    const kind = item?.kind ?? addKind;
    const binding: Record<string, [string, string]> = { entry_menu: ["journeys", "journey_intent"], store_selector: ["stores", "store_id"], category_selector: ["categories", "category_id"], product_selector: ["products", "product_id"], variant_selector: ["variants", "variant_id"], fulfilment_selector: ["fulfilment", "fulfilment_mode"], quantity: ["", "quantity"], address: ["", "delivery_address"] };
    const [source, destination] = binding[kind] ?? ["", `answer_${key}`];
    const created: Step = { key, kind, title: item?.name ?? friendly(kind), text: "", help: "", required: false, locked: protectedKinds.includes(kind), answer_type: item?.answer_type ?? (kind === "quantity" ? "number" : "text"), presentation: kind === "buttons" ? "buttons" : source || kind === "list" ? "list" : "free_text", destination, source, options: [], max_length: 1000, max_retries: 3, retry_message: "Please check your answer and try again.", next: step?.next ?? "end", failure: draft.steps.some(s => s.kind === "handoff") ? draft.steps.find(s => s.kind === "handoff")!.key : "", analytics_label: "" };
    const index = Math.max(0, draft.steps.findIndex(s => s.key === selected));
    const steps = [...draft.steps]; steps.splice(index + 1, 0, created);
    if (steps[index].next && !["condition", "end", "handoff"].includes(steps[index].kind)) steps[index] = { ...steps[index], next: key };
    change({ ...draft, steps, trigger: kind === "entry_menu" ? "default" : draft.trigger }); setSelected(key); setTab("edit");
  }
  function deleteStep() {
    if (!draft || !step || step.locked || !editable) return;
    const steps = draft.steps.filter(s => s.key !== step.key).map(s => ({ ...s, next: s.next === step.key ? step.next : s.next, failure: s.failure === step.key ? step.failure : s.failure, options: (s.options ?? []).map(o => ({ ...o, next: o.next === step.key ? step.next : o.next })) }));
    change({ ...draft, start: draft.start === step.key ? step.next : draft.start, steps }); setSelected(steps[0]?.key ?? "");
  }
  async function save() {
    if (!draft || !detail || !editable) return;
    const response = await apiPut<Detail>(`/journey-versions/${detail.version.id}`, draft);
    const selectedKey = selected; accept(response.data); setSelected(selectedKey); await loadList(); setNotice("Draft saved.");
  }
  async function startPreview() {
    if (!detail) return;
    if (dirty) { setError("Save your changes before starting a preview."); return; }
    const compiled = (await apiPost<Compilation>(`/journey-versions/${detail.version.id}/compile`, {})).data;
    setCompilation(compiled);
    if (previewMode === "flow" && compiled.errors.length) { setIssues(compiled.errors); throw new Error("Resolve Flow compilation errors before starting this preview."); }
    const response = (await apiPost<Preview>(`/journey-versions/${detail.version.id}/preview`, { scenario })).data;
    setPreview(response); setLines(response.result.messages.map(message => ({ from: "bot", message }))); setAnswer("");
  }
  async function send(text: string, label?: string, command?: string) {
    if (!preview) return;
    const response = (await apiPost<Preview>(`/${preview.controlled_test ? "journey-tests" : "journey-previews"}/${preview.session_id}/answer`, { event_id: crypto.randomUUID(), turn: preview.turn, text, command })).data;
    setPreview(response); setLines(old => [...old, { from: "customer", message: { type: "text", text: label ?? text } }, ...response.result.messages.map(message => ({ from: "bot" as const, message }))]); setAnswer("");
  }
  async function validate() {
    if (!detail) return;
    if (dirty) { setError("Save your draft before validation."); return; }
    const response = (await apiPost<Validation>(`/journey-versions/${detail.version.id}/validate`, {})).data;
    setIssues(response.issues); if (response.valid) setNotice("Validation passed.");
    const next = (await apiGet<Detail>(`/journeys/${detail.journey.id}?version=${detail.version.id}`)).data; setDetail(next);
  }
  async function activate(version: Version, mode = detail?.binding?.mode ?? draft?.mode ?? "guided_chat", paused = false) {
    if (!detail) return;
    accept((await apiPost<Detail>(`/journeys/${detail.journey.id}/activate`, { version_id: version.id, mode, paused })).data); await loadList(); setHistoryOpen(false);
  }
  async function provider(operation: string) {
    if (!detail || !draft?.connection_id) { setError("Select a WhatsApp connection in journey settings first."); return; }
    if (dirty) { setError("Save the draft before a provider operation."); return; }
    const path = `/channel-platform/connections/${draft.connection_id}/whatsapp`;
    if (operation === "key") await apiPost(`${path}/flow-key`, {});
    else await apiPost<Deployment>(`${path}/journey-flows/${detail.version.id}/${operation}`, {});
    setDetail((await apiGet<Detail>(`/journeys/${detail.journey.id}?version=${detail.version.id}`)).data);
    setNotice("Provider operation completed.");
  }
  const route = (label: string, value: string | undefined, onChange: (value: string) => void) => <label>{label}<select aria-label={label} value={value ?? ""} onChange={event => onChange(event.target.value)} disabled={!editable}><option value="">None</option>{draft?.steps.map(s => <option key={s.key} value={s.key}>{s.title || friendly(s.kind)}</option>)}</select></label>;
  const fieldOptions = draft?.steps.filter(s => s.destination).map(s => ({ value: s.destination!, label: s.title }));
  const currentPrompt = preview?.result.messages[preview.result.messages.length - 1];
  const questionActive = preview?.result.status === "active" && !(previewMode === "flow" && compilation?.errors.length);
  const flowPrompt = preview?.result.messages.map(message => message.text).filter(Boolean).join("\n\n");

  if (loading) return <Page title="Journey Studio"><LoadingState label="Loading journeys" /></Page>;
  return <Page title={detail ? draft?.name ?? "Journey Studio" : "Journey Studio"} breadcrumbs={detail ? [{ label: "Journeys", to: "/journeys" }, { label: `Version ${detail.version.version_number}` }] : undefined} actions={detail ? <>
    <Badge tone={dirty ? "warning" : detail.version.status === "published" ? "success" : "neutral"}>{dirty ? "Unsaved" : friendly(detail.version.status)}</Badge>
    <IconButton label="Journey settings" icon={Settings2} onClick={() => setSettingsOpen(true)} />
    <IconButton label="Version history" icon={History} onClick={() => setHistoryOpen(true)} />
    {editable ? <Button icon={Save} loading={busy} onClick={() => void run(save)} disabled={!dirty}>Save draft</Button> : canEdit ? <Button icon={Copy} loading={busy} onClick={() => leaving(async () => { accept((await apiPost<Detail>(`/journeys/${detail.journey.id}/drafts`, { source_version_id: detail.version.id })).data); })}>New draft</Button> : null}
    {canEdit ? <Button icon={ShieldCheck} className="secondary" disabled={busy || dirty} onClick={() => void run(validate)}>Validate</Button> : null}
    {canPublish && editable ? <Button icon={Play} className="secondary" disabled={busy || dirty} onClick={() => setConfirmAction({ title: "Publish this journey?", body: "New customers will use this version. Existing sessions keep their original version.", run: async () => { accept((await apiPost<Detail>(`/journey-versions/${detail.version.id}/publish`, {})).data); await loadList(); } })}>Publish</Button> : null}
  </> : canEdit ? <Button icon={Plus} onClick={() => setNewOpen(true)}>New journey</Button> : undefined}>
    <Flash message={error} /><Flash message={notice} tone="success" />
    {!detail || !draft ? rows.length ? <div className="journey-list"><table><thead><tr><th>Journey</th><th>Publication</th><th>Status</th><th /></tr></thead><tbody>{rows.map(row => <tr key={row.id}><td><strong>{row.name}</strong><div className="muted">{row.description}</div></td><td>{row.published_version_id ? <Badge tone="success">Published version available</Badge> : <Badge>Draft only</Badge>}</td><td><Badge>{friendly(row.status)}</Badge></td><td><Button className="secondary" onClick={() => void run(() => openJourney(row.id))}>Open</Button></td></tr>)}</tbody></table></div> : <EmptyState title="No journeys yet" action={canEdit ? <Button icon={Plus} onClick={() => setNewOpen(true)}>Create a journey</Button> : undefined} /> : <>
      <div className="journey-toolbar"><IconButton label="Back to journeys" icon={ArrowLeft} onClick={() => leaving(async () => { setDetail(undefined); setDraft(undefined); })} /><Badge tone={detail.binding?.paused ? "warning" : "neutral"}>{detail.binding?.paused ? "New sessions paused" : `${draft.category.replace(/_/g, " ")} · ${detail.binding?.mode ? friendly(detail.binding.mode) : "Not activated"}`}</Badge><span className="journey-spacer" />
        <IconButton label="Undo" icon={Undo2} disabled={!editable || !past.length} onClick={() => { const previous = past[past.length - 1]; if (previous) { setFuture(old => [draft, ...old]); setDraft(previous); setPast(old => old.slice(0, -1)); setPreview(undefined); } }} />
        <IconButton label="Redo" icon={Redo2} disabled={!editable || !future.length} onClick={() => { const next = future[0]; if (next) { setPast(old => [...old, draft]); setDraft(next); setFuture(old => old.slice(1)); setPreview(undefined); } }} />
        {canPublish && detail.binding ? <IconButton label={detail.binding.paused ? "Resume new sessions" : "Pause new sessions"} icon={detail.binding.paused ? Play : Pause} disabled={busy} onClick={() => void run(async () => { const version = detail.versions.find(v => v.id === detail.binding!.version_id); if (version) await activate(version, detail.binding!.mode, !detail.binding!.paused); })} /> : null}
      </div>
      {issues.length ? <div className="journey-issues" role="alert"><strong>{issues.length} validation {issues.length === 1 ? "issue" : "issues"}</strong>{issues.map((issue, index) => <button key={index} type="button" onClick={() => { setSelected(issue.path.replace(/^steps\./, "")); setTab("edit"); }}>{draft.steps.find(s => `steps.${s.key}` === issue.path)?.title ?? "Journey"}: {issue.message}</button>)}</div> : null}
      <div className="journey-mobile-tabs"><Tabs label="Journey panels" value={tab} onChange={setTab} items={[{ value: "steps", label: "Steps" }, { value: "edit", label: "Edit" }, { value: "preview", label: "Preview" }]} /></div>
      <div className="journey-studio" data-testid="journey-studio">
        <aside className={`journey-steps ${tab === "steps" ? "mobile-current" : ""}`} aria-label="Journey steps">
          <div className="journey-section-heading"><h2>Steps</h2><span>{draft.steps.length}</span></div>
          <ol>{draft.steps.map((s, index) => <li key={s.key} className={selected === s.key ? "selected" : ""} draggable={editable && !s.locked} onDragStart={() => setDragged(s.key)} onDragOver={event => event.preventDefault()} onDrop={event => { event.preventDefault(); move(draft.steps.findIndex(item => item.key === dragged), index); setDragged(""); }}>
            <button type="button" className="journey-step-select" onClick={() => { setSelected(s.key); setTab("edit"); }}><span className="journey-step-num">{s.locked ? <Lock size={13} /> : <GripVertical size={15} />}</span><span><strong>{s.title || friendly(s.kind)}</strong><small>{friendly(s.kind)}{draft.start === s.key ? " · Start" : ""}</small>{s.next && s.next !== draft.steps[index + 1]?.key ? <small className="journey-branch"><GitBranch size={12} />{draft.steps.find(item => item.key === s.next)?.title ?? "Missing route"}</small> : null}</span>{issues.some(issue => issue.path === `steps.${s.key}`) ? <span className="journey-error-dot" /> : null}</button>
            {editable && !s.locked ? <div className="journey-step-move"><IconButton label={`Move ${s.title} up`} icon={ArrowUp} disabled={index === 0} onClick={() => move(index, index - 1)} /><IconButton label={`Move ${s.title} down`} icon={ArrowDown} disabled={index === draft.steps.length - 1} onClick={() => move(index, index + 1)} /></div> : null}
          </li>)}</ol>
          {editable ? <div className="journey-add"><select aria-label="New step type" value={addKind} onChange={e => setAddKind(e.target.value)}>{library.map(item => { const value = `${item.kind}${item.answer_type ? `:${item.answer_type}` : ""}`; return <option key={value} value={value}>{item.name}</option>; })}</select><Button icon={Plus} className="secondary" onClick={addStep}>Add step</Button></div> : null}
        </aside>
        <section className={`journey-editor ${tab === "edit" ? "mobile-current" : ""}`} aria-label="Step editor">
          {step ? <><div className="journey-section-heading"><h2>{step.title || "Untitled step"}</h2><div className="page-actions">{step.locked ? <Badge><Lock size={12} />Protected</Badge> : editable ? <><IconButton label="Duplicate step" icon={Copy} onClick={() => { const copy = { ...step, key: keyFor(), title: `${step.title} copy`, destination: step.destination ? `answer_${keyFor()}` : undefined, options: (step.options ?? []).map(o => ({ ...o })) }; const index = draft.steps.findIndex(s => s.key === step.key); const steps = [...draft.steps]; steps.splice(index + 1, 0, copy); change({ ...draft, steps }); setSelected(copy.key); }} /><IconButton label="Delete step" icon={Trash2} disabled={draft.steps.length === 1} onClick={() => setConfirmAction({ title: `Delete ${step.title}?`, body: "Incoming routes will move to this step’s next route.", run: async () => { deleteStep(); } })} /></> : null}</div></div>
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
        </section>
        <section className={`journey-preview ${tab === "preview" ? "mobile-current" : ""}`} aria-label="Interactive journey preview">
          <div className="journey-section-heading"><h2>Preview</h2><IconButton label="Restart preview" icon={RotateCcw} disabled={busy || dirty} onClick={() => void run(startPreview)} /></div>
          <div className="journey-preview-switch" role="group" aria-label="Preview presentation"><button type="button" className={previewMode === "chat" ? "active" : ""} onClick={() => setPreviewMode("chat")}><MessageSquare size={15} />Chat</button><button type="button" className={previewMode === "flow" ? "active" : ""} onClick={() => setPreviewMode("flow")}><Smartphone size={15} />Flow</button></div>
          <label className="journey-scenario">Scenario<select value={scenario} onChange={e => { setScenario(e.target.value); setPreview(undefined); setLines([]); }}>{["normal", "empty_catalogue", "unavailable_stock", "payment_failure", "handoff", "unsupported_channel"].map(value => <option key={value} value={value}>{friendly(value)}</option>)}</select></label>
          <div className="journey-phone"><div className="journey-phone-header"><strong>{draft.name}</strong><Badge tone={preview?.controlled_test ? "warning" : "info"}>{preview?.controlled_test ? "Test records" : "Preview only"}</Badge></div>
            {preview ? previewMode === "chat" ? <div className="journey-chat" ref={scrollRef}>{lines.map((line, index) => <div key={index} className={`journey-bubble ${line.from}`}><p>{line.message.text}</p>{line.message.metadata?.help ? <small>{line.message.metadata.help}</small> : null}{index === lines.length - 1 && questionActive ? line.message.options?.map(option => <button key={option.id} type="button" disabled={busy} onClick={() => void run(() => send(option.id, option.label))}>{option.label}</button>) : null}</div>)}</div> : compilation?.errors.length ? <Flash message="This journey cannot be rendered as a Flow. Resolve the compilation errors first." /> : <JourneyFlowPreview compilation={compilation?.json} screenId={preview.flow_screen} prompt={flowPrompt} options={currentPrompt?.options} busy={busy} submit={(id, label) => void run(() => send(id, label))} /> : <div className="journey-preview-empty"><Button icon={Play} disabled={busy || dirty} onClick={() => void run(startPreview)}>Start preview</Button></div>}
            {questionActive && !currentPrompt?.options?.length ? <form className="journey-answer" onSubmit={e => { e.preventDefault(); void run(() => send(answer)); }}><input aria-label="Customer answer" value={answer} disabled={busy} type={currentPrompt?.metadata?.answer_type === "date" ? "date" : currentPrompt?.metadata?.answer_type === "email" ? "email" : "text"} onChange={e => setAnswer(e.target.value)} /><IconButton label="Send preview answer" icon={Send} type="submit" disabled={busy || !answer.trim()} /></form> : null}
          </div>
          {preview ? <><div className="journey-preview-controls"><IconButton label="Back in preview" icon={ArrowLeft} disabled={busy} onClick={() => void run(() => send("back", "Back", "back"))} /><IconButton label="Restart journey" icon={RotateCcw} disabled={busy} onClick={() => void run(() => send("restart", "Restart", "restart"))} /><Button className="secondary" disabled={busy} onClick={() => void run(() => send("cancel", "Cancel", "cancel"))}>Cancel</Button><Badge>{friendly(preview.result.status)}</Badge></div><p className="muted journey-current">{draft.steps.find(s => s.key === preview.current_step)?.title ?? "Finished"}</p></> : null}
          {preview?.cart?.length ? <div className="journey-cart"><h3>Preview cart</h3>{preview.cart.map((item, index) => <p key={index}>{item.quantity} × {item.name}<strong>{new Intl.NumberFormat("en-NG", { style: "currency", currency: item.currency ?? "NGN" }).format(item.quantity * item.unit_price_minor / 100)}</strong></p>)}</div> : null}
          {previewMode === "flow" && compilation?.errors.length ? <Flash message={compilation.errors.map(i => i.message).join(" ")} /> : null}
          {previewMode === "flow" && compilation?.warnings.length ? <details><summary>Flow warnings ({compilation.warnings.length})</summary>{compilation.warnings.map((warning, index) => <p key={index}>{warning.message}</p>)}</details> : null}
          {canPublish && detail.version.published_at && connections.find(c => c.id === draft.connection_id)?.provider === "test" ? <Button className="secondary" disabled={busy || dirty} onClick={() => setConfirmAction({ title: "Create controlled test records?", body: "This mode creates test customers, carts, orders and payments against this organization's inventory. It uses only the local test payment provider and suppresses notifications. Normal preview remains read-only.", run: async () => { const response = (await apiPost<Preview>(`/journey-versions/${detail.version.id}/controlled-test`, { acknowledge_test_records: true })).data; setPreview(response); setLines(response.result.messages.map(message => ({ from: "bot", message }))); setPreviewMode("chat"); } })}>Controlled record test</Button> : null}
        </section>
      </div>
    </>}
    <Modal open={newOpen} title="Create a journey" onClose={() => setNewOpen(false)} footer={<Button icon={Plus} loading={busy} onClick={() => void run(async () => { const response = await apiPost<Detail>("/journeys", { template: newTemplate, name: newName }); accept(response.data); await loadList(); setNewOpen(false); })}>Create journey</Button>}>
      <label>Journey name<input value={newName} maxLength={120} placeholder="Customer ordering" onChange={e => setNewName(e.target.value)} /></label><div className="journey-templates">{templates.map(template => <label key={template.id} className={newTemplate === template.id ? "selected" : ""}><input type="radio" name="template" value={template.id} checked={newTemplate === template.id} onChange={() => setNewTemplate(template.id)} /><span><strong>{template.name}</strong><small>{template.description}</small></span></label>)}</div>
    </Modal>
    <Modal open={settingsOpen && Boolean(draft)} title="Journey settings" onClose={() => setSettingsOpen(false)} footer={<Button onClick={() => setSettingsOpen(false)}>Done</Button>}>
      {draft ? <><fieldset disabled={!editable}><label>Name<input value={draft.name} maxLength={120} onChange={e => change({ ...draft, name: e.target.value })} /></label><label>Purpose<textarea value={draft.purpose} onChange={e => change({ ...draft, purpose: e.target.value })} /></label><label>Presentation<select value={draft.mode} onChange={e => change({ ...draft, mode: e.target.value })}><option value="guided_chat">Guided chat</option><option value="whatsapp_flow">WhatsApp Flow</option><option value="ai">AI with journey fallback</option></select></label><label>Channel connection<select value={draft.connection_id ?? ""} onChange={e => { const connection = connections.find(c => c.id === e.target.value); change({ ...draft, connection_id: e.target.value || undefined, channel: connection?.provider ?? "whatsapp" }); }}><option value="">Unbound preview</option>{connections.filter(c => c.status !== "archived").map(c => <option key={c.id} value={c.id}>{c.display_name} · {c.provider}</option>)}</select></label><label>Entry intent<select value={draft.trigger} onChange={e => change({ ...draft, trigger: e.target.value })}>{["order", "track_order", "complaint", "catalogue", "support", "custom", "default"].map(value => <option key={value} value={value}>{friendly(value)}</option>)}</select></label>{route("Start step", draft.start, start => change({ ...draft, start }))}<label>Session expiry (minutes)<input type="number" min={5} max={10080} value={draft.ttl_minutes} onChange={e => change({ ...draft, ttl_minutes: Number(e.target.value) })} /></label></fieldset>
        {detail?.binding && canPublish ? <label>Active presentation<select value={detail.binding.mode} disabled={busy || dirty} onChange={e => { const mode = e.target.value; setConfirmAction({ title: "Switch presentation?", body: "Only new sessions will change. Active sessions keep their current presentation.", run: async () => { const version = detail.versions.find(v => v.id === detail.binding!.version_id); if (version) await activate(version, mode, detail.binding!.paused); } }); }}><option value="guided_chat">Guided chat</option><option value="whatsapp_flow">WhatsApp Flow</option><option value="ai">AI with journey fallback</option></select></label> : null}
        <div className="journey-provider"><h3>WhatsApp Flow deployment</h3><Badge tone={detail?.deployment?.status === "published" ? "success" : "neutral"}>{detail?.deployment ? `${friendly(detail.deployment.status)} · Revision ${detail.deployment.revision}` : "Not deployed"}</Badge>
          {canDeploy ? <div className="journey-provider-actions"><Button className="secondary" disabled={busy || !draft.connection_id || dirty} onClick={() => void run(() => provider("key"))}>Register endpoint key</Button><Button className="secondary" disabled={busy || !draft.connection_id || dirty || ["published", "deprecated"].includes(detail?.deployment?.status ?? "")} onClick={() => void run(() => provider("prepare"))}>Upload draft</Button><Button className="secondary" disabled={busy || !detail?.deployment} onClick={() => void run(() => provider("refresh"))}>Refresh validation</Button><Button disabled={busy || detail?.deployment?.status !== "validated" || dirty} onClick={() => setConfirmAction({ title: "Publish provider Flow?", body: "The provider revision becomes immutable. This is separate from activating the Zidi journey.", run: () => provider("publish") })}>Publish provider Flow</Button>{detail?.deployment?.status === "published" ? <Button className="secondary" disabled={busy} onClick={() => setConfirmAction({ title: "Deprecate provider Flow?", body: "This prevents future launches. Current customer sessions may be interrupted by the provider.", run: () => provider("deprecate") })}>Deprecate</Button> : null}</div> : null}
          {detail?.deployment?.preview_url ? <a href={detail.deployment.preview_url} target="_blank" rel="noreferrer">Open provider preview</a> : null}
          {detail?.deployment?.errors && detail.deployment.errors !== "[]" ? <Flash message="Provider validation did not pass. Check the draft in the Meta test account." /> : null}
        </div>
        {canEdit && detail ? <Button className="secondary" icon={Trash2} onClick={() => setConfirmAction({ title: "Archive this journey?", body: "New sessions will stop. Existing orders are not deleted.", run: async () => { await apiDelete(`/journeys/${detail.journey.id}`); setSettingsOpen(false); setDetail(undefined); setDraft(undefined); await loadList(); } })}>Archive journey</Button> : null}
      </> : null}
    </Modal>
    <Modal open={historyOpen} title="Version history" onClose={() => setHistoryOpen(false)}>{detail?.versions.map(version => <div className="journey-version" key={version.id}><div><strong>Version {version.version_number}</strong><Badge>{version.status}</Badge>{detail.binding?.version_id === version.id ? <Badge tone="success">Active</Badge> : null}</div><div className="page-actions"><Button className="secondary" disabled={busy} onClick={() => leaving(async () => { await openJourney(detail.journey.id, version.id); setHistoryOpen(false); })}>View</Button>{canPublish && version.published_at && detail.binding?.version_id !== version.id ? <Button icon={RotateCcw} disabled={busy} onClick={() => setConfirmAction({ title: `Roll back to version ${version.version_number}?`, body: "New sessions will use this version. Existing sessions remain unchanged.", run: () => activate(version) })}>Roll back</Button> : null}</div></div>)}</Modal>
    <Modal open={Boolean(confirmAction)} title={confirmAction?.title ?? "Confirm"} description={confirmAction?.body} onClose={() => setConfirmAction(undefined)} footer={<><Button className="secondary" disabled={busy} onClick={() => setConfirmAction(undefined)}>Keep working</Button><Button loading={busy} onClick={() => { const action = confirmAction; setConfirmAction(undefined); if (action) void run(action.run); }}>Confirm</Button></>} />
  </Page>;
}
