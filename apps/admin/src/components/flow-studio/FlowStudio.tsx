import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { ArrowDown, ArrowLeft, ArrowUp, Copy, GitBranch, GripVertical, History, Lock, MessageSquare, Pause, Play, Plus, Redo2, RotateCcw, Save, Settings2, ShieldCheck, Smartphone, Trash2, Undo2 } from "lucide-react";
import { apiDelete, apiGet, apiPost, apiPut } from "../../api/client";
import { useAuth } from "../../auth";
import { Badge, Button, EmptyState, Flash, IconButton, LoadingState, Modal, Page } from "../ui";
import { hasPermission } from "../../lib/permissions";
import "./flow-studio.css";
import { experienceMode, experienceModeLabel, experienceStatus } from "../../lib/experience-modes";
import { journeyCapabilities } from "./lifecycle";
import { JourneyPreview } from "./JourneyPreview";
import { StepInspector } from "./StepInspector";
import { AIConversationPreview, useAIConversationPreview } from "./AIConversationPreview";
import { ModeEditor } from "./ModeEditor";
import { ExperienceLibrary } from "./ExperienceLibrary";
import { NewExperience } from "./NewExperience";
import { Drawer } from "../ui";
import { PreviewSheet } from "../whatsapp/PreviewSheet";

import { inspectJourney } from "../../pages/journey-insights";
import { JourneyCanvas } from "../../pages/JourneyCanvas";

const experienceHelp: Record<string, string> = {
  automated_commerce: "Customers place orders using your catalogue, stock, cart and payment workflow.",
  guided_questions: "Customers follow your published questions, choices and branches.",
  guided_chat: "Customers follow questions, buttons and lists in chat. Shopping and payment use your current catalogue and order data.",
  whatsapp_flow: "Customers shop through visual WhatsApp screens. After order confirmation, a secure checkout link is generated automatically. Test the published Flow on WhatsApp before enabling it for customers.",
  ai: "Customers ask questions naturally using the existing grounded assistant, with a journey fallback. This is not yet a fully autonomous ordering assistant.",
};

import type { Step, Definition, Version, Journey, Detail, Issue, Validation, Compilation, Preview, Library, Connection, Template, PreviewLine, Deployment } from "./types";

const friendly = (value: string) => value.replace(/_/g, " ");
const protectedKinds = ["check_inventory", "add_to_cart", "review_cart", "create_order", "payment", "payment_status", "notification", "complaint_action", "track_action"];
const keyFor = () => `step_${crypto.randomUUID().replace(/-/g, "").slice(0, 12)}`;

export function FlowStudio() {
  const { user } = useAuth();
  const aiPreview = useAIConversationPreview(user.organization_id);
  const [searchParams] = useSearchParams();
  const linkedJourney = searchParams.get("journey") || "";
  const openedLink = useRef("");
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
  const [loadFailure,setLoadFailure] = useState("");
  const [reload,setReload] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [newOpen, setNewOpen] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const closePreview = useCallback(() => setPreviewOpen(false), []);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [architectOpen,setArchitectOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [confirmAction, setConfirmAction] = useState<{ title: string; body: string; run: () => Promise<void> }>();
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const openInspector = () => setInspectorOpen(true);
  const [nodeSearch,setNodeSearch] = useState("");
  const [issues, setIssues] = useState<Issue[]>([]);
  const [preview, setPreview] = useState<Preview>();
  const [lines, setLines] = useState<PreviewLine[]>([]);
  const [answer, setAnswer] = useState("");

  const [previewMode, setPreviewMode] = useState("chat");
  const [scenario, setScenario] = useState("normal");
  const [compilation, setCompilation] = useState<Compilation>();
  const [dragged, setDragged] = useState("");
  const dirty = Boolean(draft && JSON.stringify(draft) !== saved);
  const capabilities = journeyCapabilities(detail, canEdit, canPublish);
  const editable = capabilities.edit;
  const step = draft?.steps.find(s => s.key === selected);

  async function run(work: () => Promise<void>) {
    setBusy(true); setError(""); setNotice("");
    try { await work(); } catch (e) { setError(e instanceof Error ? e.message : "This action could not be completed."); }
    finally { setBusy(false); }
  }
  async function loadList() { const response = await apiGet<Journey[]>("/journeys"); setRows(response.data); }
  function accept(next: Detail) {
    setDetail(next); setDraft(next.definition); setSaved(JSON.stringify(next.definition)); setPast([]); setFuture([]);
    setInspectorOpen(false); setPreviewOpen(false); aiPreview.reset();
    setSelected(next.definition.start || next.definition.steps[0]?.key || ""); setIssues([]); setPreview(undefined); setLines([]); setCompilation(undefined); setPreviewMode(next.definition.mode === "whatsapp_flow" ? "flow" : "chat");
  }
  async function openJourney(id: string, version?: string) { accept((await apiGet<Detail>(`/journeys/${id}${version ? `?version=${version}` : ""}`)).data); }
  function leaving(work: () => Promise<void>) {
    if (dirty) setConfirmAction({ title: "Discard unsaved changes?", body: "The last saved draft will remain available.", run: work });
    else void run(work);
  }
  useEffect(() => {
    if (!hasPermission(user.role, "bot.view")) { setLoading(false); return; }
    let current = true;
    setLoading(true);
    setLoadFailure("");
    Promise.all([
      apiGet<Journey[]>("/journeys"), apiGet<Template[]>("/journeys/templates"),
      apiGet<Library[]>("/journeys/step-library"),
      hasPermission(user.role,"channels.view") ? apiGet<Connection[]>("/channels") : Promise.resolve({data:[]}),
    ]).then(([journeys, availableTemplates, steps, channelRows]) => {
      if (!current) return;
      setRows(journeys.data); setTemplates(availableTemplates.data); setLibrary(steps.data); setConnections(channelRows.data);
    }).catch(reason => { if(current)setLoadFailure(reason instanceof Error ? reason.message : "Could not load Flow Studio."); })
      .finally(()=>{ if(current)setLoading(false); });
    return () => { current = false; };
  }, [user.organization_id, user.role, reload]);
  useEffect(() => {
    if (loading || loadFailure || !linkedJourney || !hasPermission(user.role, "bot.view")) return;
    const key = `${user.organization_id}:${linkedJourney}`;
    if (openedLink.current === key) return;
    openedLink.current = key;
    if (!rows.some(row => row.id === linkedJourney)) { setError("This journey is unavailable in this workspace."); return; }
    let current = true;
    let settled = false;
    void apiGet<Detail>(`/journeys/${encodeURIComponent(linkedJourney)}`)
      .then(response => { settled = true; if (current) accept(response.data); })
      .catch(reason => { settled = true; if (current) setError(reason instanceof Error ? reason.message : "Could not open the linked journey."); });
    return () => { current = false; if (!settled && openedLink.current === key) openedLink.current = ""; };
  }, [linkedJourney, loading, loadFailure, user.organization_id, user.role]);
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
  function addStep(chosen: string) {
    if (!draft || !editable) return;
    const key = keyFor();
    const item = library.find(l => `${l.kind}${l.answer_type ? `:${l.answer_type}` : ""}` === chosen);
    const kind = item?.kind ?? chosen;
    const binding: Record<string, [string, string]> = { entry_menu: ["journeys", "journey_intent"], store_selector: ["stores", "store_id"], category_selector: ["categories", "category_id"], product_selector: ["products", "product_id"], variant_selector: ["variants", "variant_id"], fulfilment_selector: ["fulfilment", "fulfilment_mode"], quantity: ["", "quantity"], address: ["", "delivery_address"] };
    const [source, destination] = binding[kind] ?? ["", `answer_${key}`];
    const created: Step = { key, kind, title: item?.name ?? friendly(kind), text: "", help: "", required: false, locked: protectedKinds.includes(kind), answer_type: item?.answer_type ?? (kind === "quantity" ? "number" : "text"), presentation: kind === "buttons" ? "buttons" : source || kind === "list" ? "list" : "free_text", destination, source, options: [], max_length: 1000, max_retries: 3, retry_message: "Please check your answer and try again.", next: step?.next ?? "end", failure: draft.steps.some(s => s.kind === "handoff") ? draft.steps.find(s => s.kind === "handoff")!.key : "", analytics_label: "" };
    const index = Math.max(0, draft.steps.findIndex(s => s.key === selected));
    if (draft.steps.length === 1 && step?.kind === "end") {
      created.next = step.key;
      change({...draft, start:key, steps:[created,step]});
      setSelected(key);
      openInspector();
      return;
    }
    const steps = [...draft.steps]; steps.splice(index + 1, 0, created);
    if (steps[index].next && !["condition", "end", "handoff"].includes(steps[index].kind)) steps[index] = { ...steps[index], next: key };
    change({ ...draft, steps }); setSelected(key); openInspector();
  }
  function pasteSteps(keys: string[]) {
    if (!draft || !editable) return;
    const originals = draft.steps.filter(s => keys.includes(s.key));
    if (!originals.length || originals.some(s => s.locked)) return;
    const ids = new Map(originals.map(s => [s.key,keyFor()]));
    const vars = new Map(originals.filter(s => s.destination && !s.source).map(s => [s.destination!, "answer_"+ids.get(s.key)]));
    const copies = originals.map(s => ({
      ...s, key:ids.get(s.key)!, title:s.title+" copy", destination:vars.get(s.destination ?? "") ?? s.destination,
      condition_field:vars.get(s.condition_field ?? "") ?? s.condition_field, visible_when:vars.get(s.visible_when ?? "") ?? s.visible_when,
      next:ids.get(s.next) ?? s.next, failure:ids.get(s.failure) ?? s.failure,
      position:s.position ? {x:Math.min(10000,s.position.x+32),y:Math.min(10000,s.position.y+96)} : undefined,
      options:(s.options ?? []).map(o=>({...o,next:ids.get(o.next ?? "") ?? o.next})),
    }));
    change({...draft,steps:[...draft.steps,...copies]});setSelected(copies[0].key);
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
  const architectIssues = draft && architectOpen ? inspectJourney(draft.start,draft.steps) : [];

  const inspector = draft ? <StepInspector draft={draft} step={step} editable={editable && !busy} canPublish={canPublish} patchStep={patchStep} route={route} onDuplicate={() => {
    if (!step || !editable) return;
    const copy = { ...step, key: keyFor(), title: `${step.title} copy`, destination: step.destination ? `answer_${keyFor()}` : undefined, options: (step.options ?? []).map(o => ({ ...o })) };
    const index = draft.steps.findIndex(s => s.key === step.key); const steps = [...draft.steps]; steps.splice(index + 1, 0, copy); change({ ...draft, steps }); setSelected(copy.key);
  }} onDelete={() => { if (step) setConfirmAction({ title: `Delete ${step.title}?`, body: "Incoming routes will move to this step’s next route.", run: async () => { deleteStep(); } }); }} /> : null;

  const advanced = draft && detail ? <div className="studio-advanced"><JourneyCanvas key={detail.version.id} paste={pasteSteps} connect={next=>patchStep({next})} nodes={draft.steps} invalidKeys={issues.flatMap(issue => issue.path.startsWith("steps.") ? [issue.path.slice(6).split(".")[0]] : [])} start={draft.start} editable={editable && !busy} selected={selected} select={key => { setSelected(key); openInspector(); }} arrange={positions => change({ ...draft, steps: draft.steps.map(s => positions[s.key] ? {...s,position:positions[s.key]} : s) })} />
        <aside className="journey-steps" aria-label="Journey steps">
          <div className="journey-section-heading"><h2>Steps</h2><span>{draft.steps.length}</span></div>
          <ol>{draft.steps.map((s, index) => <li key={s.key} className={selected === s.key ? "selected" : ""} draggable={editable && !busy && !s.locked} onDragStart={() => setDragged(s.key)} onDragOver={event => event.preventDefault()} onDrop={event => { event.preventDefault(); move(draft.steps.findIndex(item => item.key === dragged), index); setDragged(""); }}>
            <button type="button" className="journey-step-select" onClick={() => { setSelected(s.key); openInspector(); }}><span className="journey-step-num">{s.locked ? <Lock size={13} /> : <GripVertical size={15} />}</span><span><strong>{s.title || friendly(s.kind)}</strong><small>{friendly(s.kind)}{draft.start === s.key ? " · Start" : ""}</small>{s.next && s.next !== draft.steps[index + 1]?.key ? <small className="journey-branch"><GitBranch size={12} />{draft.steps.find(item => item.key === s.next)?.title ?? "Missing route"}</small> : null}</span>{issues.some(issue => issue.path === `steps.${s.key}`) ? <span className="journey-error-dot" /> : null}</button>
            {editable && !s.locked ? <div className="journey-step-move"><IconButton label={`Move ${s.title} up`} icon={ArrowUp} disabled={index === 0} onClick={() => move(index, index - 1)} /><IconButton label={`Move ${s.title} down`} icon={ArrowDown} disabled={index === draft.steps.length - 1} onClick={() => move(index, index + 1)} /></div> : null}
          </li>)}</ol>
          {editable ? <div className="journey-add"><h3>Node library</h3><input type="search" aria-label="Search node library" placeholder="Search node types" value={nodeSearch} onChange={e=>setNodeSearch(e.target.value)} />
            <div className="node-library">{library.filter(item=>item.name.toLowerCase().includes(nodeSearch.trim().toLowerCase())).map(item=>{const value=item.kind+(item.answer_type ? ":"+item.answer_type : "");return <button type="button" key={value} onClick={()=>addStep(value)}><Plus size={13}/>{item.name}</button>;})}</div>
            {!library.some(item=>item.name.toLowerCase().includes(nodeSearch.trim().toLowerCase())) && <p role="status">No matching node types.</p>}</div> : null}
        </aside></div> : null;

  const simulationPanel = draft && detail ? <JourneyPreview draft={draft} preview={preview} lines={lines} busy={busy} dirty={dirty} previewMode={previewMode} onPreviewMode={setPreviewMode} scenario={scenario} onScenario={value => { setScenario(value); setPreview(undefined); setLines([]); }} compilation={compilation} answer={answer} onAnswer={setAnswer} onStart={() => void run(startPreview)} onSend={(text, label, command) => void run(() => send(text, label, command))} error={error} canControlledTest={Boolean(canPublish && detail.version.published_at && connections.find(c => c.id === draft.connection_id)?.provider === "test")} onControlledTest={() => { setPreviewOpen(false); setConfirmAction({ title: "Create controlled test records?", body: "This mode creates test customers, carts, orders and payments against this organization's inventory. It uses only the local test payment provider and suppresses notifications. Normal preview remains read-only.", run: async () => { const response = (await apiPost<Preview>(`/journey-versions/${detail.version.id}/controlled-test`, { acknowledge_test_records: true })).data; setPreview(response); setLines(response.result.messages.map(message => ({ from: "bot", message }))); setPreviewMode("chat"); setPreviewOpen(true); } }); }} /> : null;

  const previewPanel = draft?.mode === "ai" ? <section className="journey-preview" aria-label="AI conversation preview"><h2>Live preview</h2>{hasPermission(user.role, "ai.use") ? <AIConversationPreview key={detail?.version.id} name={draft.name} chat={aiPreview} /> : <p>AI testing requires permission to use the assistant.</p>}<details open={Boolean(preview)}><summary>Guided fallback simulation</summary>{simulationPanel}</details></section> : simulationPanel;

  const ModeIcon = experienceMode(draft?.mode)?.icon ?? MessageSquare;
  if (!hasPermission(user.role, "bot.view")) return <Page title="Flow Studio"><EmptyState title="Access required" body="You need permission to view customer experiences." /></Page>;

  if (loading) return <Page title="Flow Studio"><LoadingState label="Loading journeys" /></Page>;
  if (loadFailure) return <Page title="Flow Studio"><Flash message={loadFailure}/><Button className="secondary" onClick={()=>setReload(n=>n+1)}>Retry loading journeys</Button></Page>;
  return <Page description={detail ? "Build, validate and preview this customer experience." : "Build customer experiences from a template or start with your own flow."} title={detail ? draft?.name ?? "Flow Studio" : "Flow Studio"} breadcrumbs={detail ? [{ label: "Flow Studio", to: "/journeys" }, { label: `Version ${detail.version.version_number}` }] : undefined} actions={detail ? <>
    <Badge><ModeIcon size={14} style={{ color: experienceMode(draft?.mode)?.accent }} />{experienceModeLabel(draft?.mode)}</Badge>
    <Badge tone={dirty || experienceStatus(detail.journey, detail.binding) === "Paused" ? "warning" : experienceStatus(detail.journey, detail.binding) === "Live" ? "success" : "neutral"}>{dirty ? "Unsaved changes" : experienceStatus(detail.journey, detail.binding)}</Badge>
    <IconButton label="Experience settings" icon={Settings2} onClick={() => setSettingsOpen(true)} />
    <IconButton label="Version history" icon={History} onClick={() => setHistoryOpen(true)} />
    {editable ? <Button icon={Save} loading={busy} onClick={() => void run(save)} disabled={!dirty}>Save draft</Button> : capabilities.clone ? <Button icon={Copy} loading={busy} onClick={() => leaving(async () => { accept((await apiPost<Detail>(`/journeys/${detail.journey.id}/drafts`, { source_version_id: detail.version.id })).data); })}>New draft</Button> : null}
    {canEdit && detail.journey.status !== "archived" ? <Button icon={ShieldCheck} className="secondary" disabled={busy || dirty} onClick={() => void run(validate)}>Validate</Button> : null}
    {capabilities.publish ? <Button icon={Play} className="secondary" disabled={busy || dirty} onClick={() => setConfirmAction({ title: "Publish this journey?", body: "New customers will use this version. Existing sessions keep their original version.", run: async () => { accept((await apiPost<Detail>(`/journey-versions/${detail.version.id}/publish`, {})).data); await loadList(); } })}>Publish</Button> : null}
    {canPublish ? <Button className="secondary" disabled={busy || dirty || !capabilities.activate || !draft?.connection_id} title={!draft?.connection_id ? "Choose a channel connection in draft settings before publishing." : undefined} onClick={() => setConfirmAction({ title: "Activate on number?", body: "New sessions will use this published version and its saved channel connection.", run: () => activate(detail.version, draft?.mode) })}>Activate on number</Button> : null}
    <Button className="secondary" disabled={busy || dirty || (draft?.mode === "ai" && (aiPreview.busy || !hasPermission(user.role, "ai.use")))} onClick={() => { setPreviewOpen(true); if (draft?.mode === "ai") void aiPreview.start(); else void run(startPreview); }}>Test</Button>
  </> : canEdit ? <Button className="primary" icon={Plus} onClick={() => setNewOpen(true)}>New experience</Button> : undefined}>
    <Flash message={error} /><Flash message={notice} tone="success" />
    {!detail || !draft ? <ExperienceLibrary rows={rows} connections={connections} onOpen={id => void run(() => openJourney(id))} onCreate={canEdit ? () => setNewOpen(true) : undefined} /> : <>
      <div className="journey-toolbar"><IconButton label="Back to Flow Studio" icon={ArrowLeft} disabled={busy} onClick={() => leaving(async () => { setDetail(undefined); setDraft(undefined); })} /><span className="muted">Version {detail.version.version_number} · {friendly(detail.version.status)}</span><span className="journey-spacer" />
        <Button icon={GitBranch} className="secondary" onClick={()=>setArchitectOpen(true)}>Zidi Architect</Button>
        <IconButton label="Undo" icon={Undo2} disabled={busy || !editable || !past.length} onClick={() => { const previous = past[past.length - 1]; if (previous) { setFuture(old => [draft, ...old]); setDraft(previous); setPast(old => old.slice(0, -1)); setPreview(undefined); setLines([]); setCompilation(undefined); } }} />
        <IconButton label="Redo" icon={Redo2} disabled={busy || !editable || !future.length} onClick={() => { const next = future[0]; if (next) { setPast(old => [...old, draft]); setDraft(next); setFuture(old => old.slice(1)); setPreview(undefined); setLines([]); setCompilation(undefined); } }} />
        {capabilities.toggleSessions && detail.binding ? <IconButton label={detail.binding.paused ? "Resume new sessions" : "Pause new sessions"} icon={detail.binding.paused ? Play : Pause} disabled={busy || dirty} onClick={() => void run(async () => { const version = detail.versions.find(v => v.id === detail.binding!.version_id); if (version) await activate(version, detail.binding!.mode, !detail.binding!.paused); })} /> : null}
      </div>
      {detail.binding?.paused && detail.journey.status !== "archived" ? <div className="studio-paused" role="status"><div><strong>New sessions paused</strong><p>Existing conversations continue. Resume to let customers start this experience.</p></div>{canPublish ? <Button disabled={busy || dirty} onClick={() => void run(async () => { const version = detail.versions.find(v => v.id === detail.binding!.version_id); if (version) await activate(version, detail.binding!.mode, false); })}>Resume new sessions</Button> : null}</div> : null}
      {issues.length ? <div className="journey-issues" role="alert"><strong>{issues.length} validation {issues.length === 1 ? "issue" : "issues"}</strong>{issues.map((issue, index) => <button key={index} type="button" onClick={() => { setSelected(issue.path.replace(/^steps\./, "")); openInspector(); }}>{draft.steps.find(s => `steps.${s.key}` === issue.path)?.title ?? "Journey"}: {issue.message}</button>)}</div> : null}
      <Button className="secondary studio-preview-toggle" icon={Smartphone} onClick={() => setPreviewOpen(true)}>Open live preview</Button>
      <div className="journey-studio" data-testid="journey-studio">
        <ModeEditor key={detail.version.id} draft={draft} editable={editable && !busy} onChange={change} selected={selected} onSelectStep={key => { setSelected(key); if (draft.mode !== "whatsapp_flow") setInspectorOpen(true); }} advanced={advanced} inspector={inspector} inspectorOpen={inspectorOpen} onInspectorClose={() => setInspectorOpen(false)} />
        {!previewOpen && <div className="studio-preview-desktop">{previewPanel}</div>}
        {previewOpen ? <div className="studio-preview-sheet"><PreviewSheet title="Live preview" full onClose={closePreview}>{previewPanel}</PreviewSheet></div> : null}
      </div>
    </>}
    <NewExperience open={newOpen} onClose={() => setNewOpen(false)} templates={templates} connections={connections} busy={busy} error={error} onCreate={input => void run(async () => {
      const created = (await apiPost<Detail>("/journeys", { template: input.template, name: input.name })).data;
      accept(created); setNewOpen(false);
      const connection = connections.find(item => item.id === input.connection_id);
      const definition = { ...created.definition, name: input.name, mode: input.mode, purpose: input.purpose || created.definition.purpose, connection_id: input.connection_id || undefined, channel: connection?.provider ?? created.definition.channel };
      // Keep the created draft open if configuration cannot be saved, so it can be recovered.
      setDraft(definition);
      accept((await apiPut<Detail>(`/journey-versions/${created.version.id}`, definition)).data);
      await loadList();
    })} />
    <Drawer open={settingsOpen && Boolean(draft)} title="Experience settings" onClose={() => setSettingsOpen(false)}>
      {draft ? <><Flash message={error} /><fieldset disabled={!editable}><label>Name<input value={draft.name} maxLength={120} onChange={e => change({ ...draft, name: e.target.value })} /></label><label>Purpose<textarea value={draft.purpose} onChange={e => change({ ...draft, purpose: e.target.value })} /></label><p><strong>{experienceModeLabel(draft.mode)}</strong></p><p role="status">{experienceHelp[draft.mode]}</p><p>Messaging charges depend on applicable Meta rules and your provider. A Flow is not a guaranteed single charge. AI token usage and payment processing fees are separate.</p><label>Channel connection<select value={draft.connection_id ?? ""} onChange={e => { const connection = connections.find(c => c.id === e.target.value); change({ ...draft, connection_id: e.target.value || undefined, channel: connection?.provider ?? "whatsapp" }); }}><option value="">Unbound preview</option>{connections.filter(c => c.status !== "archived").map(c => <option key={c.id} value={c.id}>{c.display_name} · {c.provider}</option>)}</select></label><label>Entry button label<input maxLength={20} value={draft.entry_cta ?? ""} onChange={e => change({...draft, entry_cta:e.target.value})} /></label><label>Entry keyword<input value={draft.trigger} maxLength={48} pattern="[a-z][a-z0-9_]*" onChange={e => change({ ...draft, trigger: e.target.value })} /></label>{route("Start step", draft.start, start => change({ ...draft, start }))}<label>Session expiry (minutes)<input type="number" min={5} max={10080} value={draft.ttl_minutes} onChange={e => change({ ...draft, ttl_minutes: Number(e.target.value) })} /></label></fieldset>
        {detail?.binding && canPublish && detail.journey.status !== "archived" ? <label>Active presentation<select value={detail.binding.mode} disabled={busy || dirty} onChange={e => { const mode = e.target.value; setConfirmAction({ title: "Switch presentation?", body: "Only new sessions will change. Active sessions keep their current presentation.", run: async () => { const version = detail.versions.find(v => v.id === detail.binding!.version_id); if (version) await activate(version, mode, detail.binding!.paused); } }); }}><option value="automated_commerce">Automated Commerce</option><option value="guided_questions">Guided Questions</option><option value="guided_chat">Legacy guided chat</option><option value="whatsapp_flow">WhatsApp Flow</option><option value="ai">AI Conversation</option></select></label> : null}
        {detail?.channel_default && <p role="status">Channel default · {rows.find(row => row.id === detail.channel_default?.journey_id)?.name ?? experienceModeLabel(detail.channel_default.mode)}{detail.channel_default.journey_id === detail.journey.id ? " · Version " + (detail.versions.find(v => v.id === detail.channel_default?.version_id)?.version_number ?? "unknown") : ""}</p>}
        {detail?.binding && canPublish && detail.journey.status !== "archived" ? <Button disabled={busy || dirty || detail.binding.paused || detail.channel_default?.version_id === detail.binding.version_id} onClick={() => setConfirmAction({ title: "Use as the default conversation mode?", body: "New conversations on this channel will use this published journey. Existing conversations retain their current version.", run: async () => { await apiPost(`/journeys/${detail.journey.id}/default`, {}); await openJourney(detail.journey.id, detail.version.id); setNotice("Channel default updated."); setSettingsOpen(false); } })}>Use as channel default</Button> : null}
        {(draft.mode === "whatsapp_flow" || detail?.binding?.mode === "whatsapp_flow" || detail?.deployment) && <div className="journey-provider"><h3>WhatsApp Flow deployment</h3><Badge tone={detail?.deployment?.status === "published" ? "success" : "neutral"}>{detail?.deployment ? `${friendly(detail.deployment.status)} · Revision ${detail.deployment.revision}` : "Not deployed"}</Badge>
          {canDeploy && detail?.journey.status !== "archived" ? <div className="journey-provider-actions"><Button className="secondary" disabled={busy || !draft.connection_id || dirty} onClick={() => void run(() => provider("key"))}>Register endpoint key</Button><Button className="secondary" disabled={busy || !draft.connection_id || dirty || ["published", "deprecated"].includes(detail?.deployment?.status ?? "")} onClick={() => void run(() => provider("prepare"))}>Upload draft</Button><Button className="secondary" disabled={busy || !detail?.deployment} onClick={() => void run(() => provider("refresh"))}>Refresh validation</Button><Button disabled={busy || detail?.deployment?.status !== "validated" || dirty} onClick={() => setConfirmAction({ title: "Publish provider Flow?", body: "The provider revision becomes immutable. This is separate from activating the Zidi journey.", run: () => provider("publish") })}>Publish provider Flow</Button>{detail?.deployment?.status === "published" ? <Button className="secondary" disabled={busy} onClick={() => setConfirmAction({ title: "Deprecate provider Flow?", body: "This prevents future launches. Current customer sessions may be interrupted by the provider.", run: () => provider("deprecate") })}>Deprecate</Button> : null}</div> : null}
          {detail?.deployment?.preview_url ? <a href={detail.deployment.preview_url} target="_blank" rel="noreferrer">Open provider preview</a> : null}
          {detail?.deployment?.errors && detail.deployment.errors !== "[]" ? <Flash message="Provider validation did not pass. Check the draft in the Meta test account." /> : null}
        </div>}
        {canEdit && detail && detail.journey.status !== "archived" ? <Button className="secondary" icon={Trash2} onClick={() => setConfirmAction({ title: "Archive this journey?", body: "New sessions will stop. Existing orders are not deleted.", run: async () => { await apiDelete(`/journeys/${detail.journey.id}`); setSettingsOpen(false); setDetail(undefined); setDraft(undefined); await loadList(); } })}>Archive journey</Button> : null}
      </> : null}
    </Drawer>
    <Modal open={architectOpen} title="Zidi Architect" description="Review your draft’s routes and questions." onClose={()=>setArchitectOpen(false)}>
      {draft && <>{step && <p><strong>{step.title}</strong> · {friendly(step.kind)}{step.destination ? " · Saves to "+step.destination : ""}</p>}{architectIssues.length ? <ul className="architect-list">{architectIssues.map(issue=><li key={issue.message}><p>{issue.message}</p><button onClick={()=>{setSelected(issue.key);openInspector();setArchitectOpen(false);}}>Inspect step</button></li>)}</ul> : <p>All steps are reachable and each question has a prompt. Use Validate for the full publishing checks.</p>}</>}
    </Modal>
    <Drawer open={historyOpen} title="Version history" onClose={() => setHistoryOpen(false)}>{detail?.versions.map(version => <div className="journey-version" key={version.id}><div><strong>Version {version.version_number}</strong><Badge>{version.status}</Badge>{detail.binding?.version_id === version.id ? <Badge tone="success">Active</Badge> : null}</div><div className="page-actions"><Button className="secondary" disabled={busy} onClick={() => leaving(async () => { await openJourney(detail.journey.id, version.id); setHistoryOpen(false); })}>View</Button>{canPublish && detail.journey.status !== "archived" && version.published_at && detail.binding?.version_id !== version.id ? <Button icon={RotateCcw} disabled={busy} onClick={() => setConfirmAction({ title: `Roll back to version ${version.version_number}?`, body: "New sessions will use this version. Existing sessions remain unchanged.", run: () => activate(version) })}>Roll back</Button> : null}</div></div>)}</Drawer>
    <Modal open={Boolean(confirmAction)} title={confirmAction?.title ?? "Confirm"} description={confirmAction?.body} onClose={() => setConfirmAction(undefined)} footer={<><Button className="secondary" disabled={busy} onClick={() => setConfirmAction(undefined)}>Keep working</Button><Button loading={busy} onClick={() => { const action = confirmAction; setConfirmAction(undefined); if (action) void run(action.run); }}>Confirm</Button></>} />
  </Page>;
}
