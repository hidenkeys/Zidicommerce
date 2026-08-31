import { FormEvent, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet, apiPatch, apiPost, apiPut } from "../api/client";
import { useAuth } from "../auth";
import { Card, EmptyState, Flash, LoadingState, Page, StatusDot, Tabs } from "../components/ui";
import { moduleHelp, parseJSON, relativeTime, type Row } from "../lib/format";
import { hasPermission } from "../lib/permissions";
import { isUsableChannelStatus } from "../lib/channels";

type Bot = Row & { id: string; name: string; status: string; published_version_id?: string; updated_at?: string };
type BotVersion = Row & { id: string; bot_id: string; version_number: number; status: string };
type BotModule = Row & { id: string; module_key: string; name: string; parameters?: string; metadata?: string; sort_order?: number; description?: string };
type BotConfig = { version: BotVersion; modules: BotModule[] };
type Channel = Row & { provider: string; display_name: string; display_number?: string; status: string };
type RuntimeMessage = { from: "customer" | "bot" | "debug"; text: string };
type FieldMessage = Row & { id: string; author_type: string; body: string };
type WorkflowConfig = Row & {
  status: string;
  bot_display_name: string;
  greeting: string;
  tone: string;
  ordering_enabled: boolean;
  payment_enabled: boolean;
  human_handoff_enabled: boolean;
  store_selection_strategy: string;
  enabled_action_list: string[];
  supported_fulfilment_mode_list: string[];
  post_payment_step_list: string[];
};

const fulfilmentModes = [
  { value: "pickup", label: "Store pickup" },
  { value: "customer_rider", label: "Customer-arranged rider" },
  { value: "merchant_rider", label: "Merchant delivery" },
];

const postPaymentSteps = [
  { value: "merchant_prepares", label: "Prepare order" },
  { value: "notify_store", label: "Notify store" },
  { value: "notify_customer", label: "Notify customer" },
  { value: "assign_internal_rider", label: "Assign internal rider" },
  { value: "third_party_delivery", label: "Use third-party delivery" },
  { value: "customer_pickup", label: "Prepare for customer pickup" },
  { value: "enable_tracking", label: "Enable tracking context" },
  { value: "request_human_handoff", label: "Request human handoff" },
];

const workflowActionGroups = {
  product_discovery: ["get_store", "get_stores", "select_store", "get_categories", "select_category", "get_products", "select_product", "get_store_catalogue", "get_product", "get_variant", "get_inventory", "check_inventory"],
  order_tracking: ["get_order", "get_order_status", "get_customer_orders", "select_order"],
};

const workflowStages = [
  { group: "conversation", label: "Welcome and intent", detail: "AI, menu, or knowledge" },
  { group: "deterministic", label: "Store and products", detail: "Current catalogue" },
  { group: "deterministic", label: "Variant and quantity", detail: "Current stock" },
  { group: "deterministic", label: "Cart and fulfilment", detail: "Verified total" },
  { group: "deterministic", label: "Create order", detail: "Duplicate-safe" },
  { group: "external", label: "Payment confirmation", detail: "Provider verification" },
  { group: "operations", label: "Prepare and fulfil", detail: "Store operations" },
  { group: "support", label: "Human handoff", detail: "When requested" },
  { group: "conversation", label: "Complete or continue", detail: "Return to conversation" },
];

function enabled(module: BotModule) {
  return parseJSON<Record<string, unknown>>(module.metadata, {}).enabled !== false;
}

export function AssistantPage() {
  const { user } = useAuth();
  const canManageAssistant = hasPermission(user.role, "bot.manage");
  const canPublishAssistant = hasPermission(user.role, "bot.publish");
  const canViewChannels = hasPermission(user.role, "channels.view");
  const [bots, setBots] = useState<Bot[]>([]);
  const [versions, setVersions] = useState<BotVersion[]>([]);
  const [config, setConfig] = useState<BotConfig | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [workflow, setWorkflow] = useState<WorkflowConfig | null>(null);
  const [savingWorkflow, setSavingWorkflow] = useState(false);
  const [name, setName] = useState("");
  const [assistantType, setAssistantType] = useState("commerce");
  const [creating, setCreating] = useState(false);
  const [testSessionID, setTestSessionID] = useState("");
  const [testInput, setTestInput] = useState("");
  const [testMessages, setTestMessages] = useState<RuntimeMessage[]>([]);
  const seenFieldMessages = useRef(new Set<string>());
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");
  const [view, setView] = useState("workflow");
  const [loading, setLoading] = useState(true);

  const bot = bots[0];
  const published = versions.find((version) => version.status === "published");
  const draft = versions.find((version) => version.status === "draft" || version.status === "validated") ?? versions[0];
  const whatsapp = channels.find((channel) => channel.provider === "whatsapp");
  const editable = config?.version.status !== "published" && config?.version.status !== "archived";

  async function load() {
    try {
      const [botResponse, channelResponse, workflowResponse] = await Promise.all([
        apiGet<Bot[]>("/bots"),
        canViewChannels ? apiGet<Channel[]>("/channels") : Promise.resolve({ data: [] as Channel[] }),
        apiGet<WorkflowConfig>("/bot-workflow"),
      ]);
      setBots(botResponse.data);
      setChannels(channelResponse.data);
      setWorkflow(workflowResponse.data);
      const nextBot = botResponse.data[0];
      if (!nextBot) {
        setVersions([]);
        setConfig(null);
        return;
      }
      const versionResponse = await apiGet<BotVersion[]>(`/bots/${nextBot.id}/versions`);
      setVersions(versionResponse.data);
      const nextVersion = versionResponse.data.find((version) => version.status === "draft" || version.status === "validated") ?? versionResponse.data[0];
      if (nextVersion) {
        const configResponse = await apiGet<BotConfig>(`/bot-versions/${nextVersion.id}/configuration`);
        setConfig(configResponse.data);
      }
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load assistant");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [canViewChannels]);

  useEffect(() => {
    if (!testSessionID) return;
    let cancelled = false;
    async function syncFieldMessages() {
      try {
        const response = await apiGet<FieldMessage[]>(`/field/test-sessions/${testSessionID}/messages`);
        const fresh = response.data.filter((item) => item.author_type !== "customer" && !seenFieldMessages.current.has(item.id));
        fresh.forEach((item) => seenFieldMessages.current.add(item.id));
        if (!cancelled && fresh.length > 0) {
          setTestMessages((items) => [
            ...items,
            ...fresh
              .filter((item) => !items.some((existing) => existing.from === "bot" && existing.text === item.body))
              .map((item) => ({ from: "bot" as const, text: item.author_type === "provider" ? `Handyman: ${item.body}` : item.body })),
          ]);
        }
      } catch {
        // The request is created part-way through the simulator flow.
      }
    }
    void syncFieldMessages();
    const timer = window.setInterval(() => void syncFieldMessages(), 1500);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [testSessionID]);

  async function createAssistant(event: FormEvent) {
    event.preventDefault();
    setCreating(true);
    try {
      const fieldService = assistantType === "field_service";
      await apiPost<Bot>(fieldService ? "/bots/service-booking" : "/bots/self-service", fieldService ? {
        name: name || "Service assistant",
        welcome_message: "Welcome. I can help you book a trusted professional for your home-service request.",
      } : {
        name: name || "Store assistant",
        description: "Customer assistant",
        welcome_message: "Welcome. I can help you place an order, track an order, answer questions, or contact support.",
        require_payment: true,
      });
      setFlash("Assistant created. Enable the capabilities you need, then publish.");
      await load();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not create assistant");
    } finally {
      setCreating(false);
    }
  }

  async function ensureDraft() {
    if (!bot) return null;
    if (draft && (draft.status === "draft" || draft.status === "validated")) {
      const configResponse = await apiGet<BotConfig>(`/bot-versions/${draft.id}/configuration`);
      setConfig(configResponse.data);
      return configResponse.data;
    }
    const response = await apiPost<BotVersion>(`/bots/${bot.id}/versions`, { source_version_id: published?.id || draft?.id });
    const configResponse = await apiGet<BotConfig>(`/bot-versions/${response.data.id}/configuration`);
    setVersions((current) => [response.data, ...current]);
    setConfig(configResponse.data);
    return configResponse.data;
  }

  async function toggleModule(module: BotModule, next: boolean) {
    const nextConfig = await ensureDraft();
    if (!nextConfig) return;
    const target = nextConfig.modules.find((item) => item.module_key === module.module_key) ?? nextConfig.modules.find((item) => item.id === module.id);
    if (!target) return;
    await apiPost<BotModule>(`/bot-modules/${target.id}/${next ? "enable" : "disable"}`, {});
    setFlash(next ? "Capability enabled. Publish to make it live." : "Capability hidden from new conversations after you publish.");
    await load();
  }

  async function moveModule(module: BotModule, direction: -1 | 1) {
    const nextConfig = await ensureDraft();
    if (!nextConfig) return;
    const sorted = [...nextConfig.modules].sort((left, right) => Number(left.sort_order ?? 0) - Number(right.sort_order ?? 0));
    const index = sorted.findIndex((item) => item.module_key === module.module_key || item.id === module.id);
    const nextIndex = index + direction;
    if (index < 0 || nextIndex < 0 || nextIndex >= sorted.length) return;
    [sorted[index], sorted[nextIndex]] = [sorted[nextIndex], sorted[index]];
    await apiPost<BotModule[]>(`/bot-versions/${nextConfig.version.id}/modules/reorder`, {
      modules: sorted.map((item, position) => ({ module_id: item.id, sort_order: (position + 1) * 10 })),
    });
    await load();
  }

  async function publish() {
    const nextConfig = await ensureDraft();
    if (!nextConfig) return;
    await apiPost<Row>(`/bot-versions/${nextConfig.version.id}/validate`, {});
    await apiPost<Row>(`/bot-versions/${nextConfig.version.id}/publish`, {});
    setFlash("Published. New conversations will use this version. Existing conversations stay on the version they started with.");
    await load();
  }

  function updateWorkflow<K extends keyof WorkflowConfig>(key: K, value: WorkflowConfig[K]) {
    setWorkflow((current) => current ? { ...current, [key]: value } : current);
  }

  function toggleWorkflowList(key: "supported_fulfilment_mode_list" | "post_payment_step_list", value: string, checked: boolean) {
    setWorkflow((current) => {
      if (!current) return current;
      const values = current[key] ?? [];
      return { ...current, [key]: checked ? [...new Set([...values, value])] : values.filter((item) => item !== value) };
    });
  }

  function toggleActionGroup(group: keyof typeof workflowActionGroups, checked: boolean) {
    setWorkflow((current) => {
      if (!current) return current;
      const target = workflowActionGroups[group];
      const existing = current.enabled_action_list ?? [];
      const next = checked ? [...new Set([...existing, ...target])] : existing.filter((action) => !target.includes(action));
      return { ...current, enabled_action_list: next };
    });
  }

  async function saveWorkflow(event: FormEvent) {
    event.preventDefault();
    if (!workflow) return;
    setSavingWorkflow(true);
    try {
      const response = await apiPut<WorkflowConfig>("/bot-workflow", {
        status: workflow.status || "active",
        bot_display_name: workflow.bot_display_name,
        greeting: workflow.greeting,
        tone: workflow.tone,
        ordering_enabled: workflow.ordering_enabled,
        payment_enabled: workflow.payment_enabled,
        human_handoff_enabled: workflow.human_handoff_enabled,
        store_selection_strategy: workflow.store_selection_strategy,
        enabled_actions: workflow.enabled_action_list,
        supported_fulfilment_modes: workflow.supported_fulfilment_mode_list,
        post_payment_steps: workflow.post_payment_step_list,
      });
      setWorkflow(response.data);
      setFlash("Commerce workflow settings saved and applied to new runtime actions.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not save commerce workflow settings");
    } finally {
      setSavingWorkflow(false);
    }
  }

  async function startTest() {
    if (!bot) return;
    const channelID = channels[0]?.id;
    if (!channelID) {
      setMessage("A customer channel configuration is required before this runtime test can start.");
      return;
    }
    const response = await apiPost<Row>("/runtime/test/start", {
      bot_id: bot.id,
      channel_id: channelID,
      external_conversation_id: `admin-test-${Date.now()}`,
      sender: "2348000000000",
    });
    seenFieldMessages.current.clear();
    setTestSessionID(String(response.data.id));
    setTestMessages([{ from: "debug", text: "Test started. This does not message real customers." }]);
  }

  async function sendTest(event: FormEvent) {
    event.preventDefault();
    if (!testSessionID || !testInput.trim()) return;
    const text = testInput.trim();
    setTestInput("");
    setTestMessages((items) => [...items, { from: "customer", text }]);
    const response = await apiPost<Row>("/runtime/test/message", {
      session_id: testSessionID,
      external_message_id: `admin-message-${Date.now()}`,
      text,
    });
    const outbound = Array.isArray(response.data.messages) ? (response.data.messages as Row[]) : [];
    setTestMessages((items) => [
      ...items,
      ...outbound.map((msg) => ({
        from: "bot" as const,
        text: [String(msg.text ?? ""), Array.isArray(msg.options) ? (msg.options as Row[]).map((option) => option.label ?? option.id).join(" · ") : ""].filter(Boolean).join("\n"),
      })),
    ]);
  }

  const modules = [...(config?.modules ?? [])].sort((left, right) => Number(left.sort_order ?? 0) - Number(right.sort_order ?? 0));
  const menu = modules.filter(enabled);

  if (loading) {
    return <Page title="Your assistant" description="Set up how your assistant helps customers."><LoadingState label="Loading assistant" /></Page>;
  }

  if (!bot) {
    return (
      <Page title="Your assistant" description="Set up how your assistant welcomes customers and helps with commerce or service requests.">
        <Flash message={message} />
        <Card>
          {canManageAssistant ? <>
            <h3>Create your assistant</h3>
            <p>Choose the starter that matches how your business serves customers. You can adjust and publish it next.</p>
            <form className="form-grid" onSubmit={createAssistant}>
            <label className="full">Assistant name<input value={name} onChange={(event) => setName(event.target.value)} placeholder="Store assistant" /></label>
            <label className="full">Business flow
              <select value={assistantType} onChange={(event) => setAssistantType(event.target.value)}>
                <option value="commerce">Products and orders</option>
                <option value="field_service">Field service / handyman bookings</option>
              </select>
            </label>
            <div className="full"><button type="submit" disabled={creating}>Create assistant</button></div>
            </form>
          </> : <EmptyState title="No assistant configured" body="An organization administrator must create and publish the customer assistant." />}
        </Card>
      </Page>
    );
  }

  return (
    <Page
      title={bot.name}
      description={canManageAssistant ? "Choose what your assistant can do. Customers see changes after you publish." : "Review what the customer assistant is currently configured to do."}
      actions={canManageAssistant ?
        <>
          <button type="button" onClick={() => void startTest()}>Test assistant</button>
          {canPublishAssistant ? <button type="button" className="primary" onClick={() => void publish()}>Publish changes</button> : null}
        </>
      : undefined}
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <div className="metrics">
        <article className="metric-card">
          <span>Status</span>
          <strong><StatusDot live={Boolean(bot.published_version_id) && bot.status === "active"} label={bot.published_version_id ? "Live" : "Draft"} /></strong>
        </article>
        <article className="metric-card">
          <span>Customer channel</span>
          <strong>{isUsableChannelStatus(whatsapp?.status) ? whatsapp?.display_number || "Connected" : "Not connected"}</strong>
        </article>
        <article className="metric-card">
          <span>Published</span>
          <strong>{published ? `v${published.version_number}` : "Never"}</strong>
          <p>{published ? relativeTime(published.updated_at ?? bot.updated_at) : "Publish when you are ready"}</p>
        </article>
        <article className="metric-card">
          <span>{canManageAssistant ? "Editing" : "Access"}</span>
          <strong>{canManageAssistant ? editable ? `Draft v${config?.version.version_number}` : "Published copy" : "View only"}</strong>
          <p>{canManageAssistant ? editable ? "Safe to change" : "Create a draft by toggling a capability" : "Configuration controls are hidden"}</p>
        </article>
      </div>

      <Tabs
        value={view}
        onChange={setView}
        label="Assistant configuration sections"
        items={[
          { value: "workflow", label: "Identity and flow" },
          { value: "capabilities", label: "Capabilities", count: modules.filter(enabled).length },
          { value: "test", label: "Preview and test" },
        ]}
      />

      {view === "workflow" && workflow ? (
        <section className="workflow-section">
          <div className="section-heading">
            <div>
              <h2>Commerce workflow</h2>
              <p>Choose how the assistant guides customers and where verified business actions take over.</p>
            </div>
            <StatusDot live={workflow.status === "active"} label={workflow.status === "active" ? "Active" : "Inactive"} />
          </div>
          <div className="workflow-map" aria-label="Commerce workflow stages">
            {workflowStages.map((stage, index) => (
              <div className={`workflow-stage ${stage.group}`} key={stage.label}>
                <span>{index + 1}</span>
                <strong>{stage.label}</strong>
                <small>{stage.detail}</small>
              </div>
            ))}
          </div>
          <div className="workflow-legend" aria-label="Workflow stage types">
            <span className="conversation">Conversation</span>
            <span className="deterministic">Verified commerce step</span>
            <span className="external">Payment provider</span>
            <span className="operations">Store operation</span>
            <span className="support">Human support</span>
          </div>
          <form className="workflow-config" onSubmit={saveWorkflow}>
            <fieldset className="workflow-editor-fieldset" disabled={!canManageAssistant}>
            <legend className="sr-only">Assistant workflow configuration</legend>
            <div className="workflow-fields">
              <label>Assistant display name<input value={workflow.bot_display_name} onChange={(event) => updateWorkflow("bot_display_name", event.target.value)} /></label>
              <label>Tone
                <select value={workflow.tone} onChange={(event) => updateWorkflow("tone", event.target.value)}>
                  <option value="professional">Professional</option>
                  <option value="helpful">Helpful</option>
                  <option value="friendly">Friendly</option>
                  <option value="concise">Concise</option>
                </select>
              </label>
              <label>Store selection
                <select value={workflow.store_selection_strategy} onChange={(event) => updateWorkflow("store_selection_strategy", event.target.value)}>
                  <option value="customer_choice">Customer chooses</option>
                  <option value="single_store">Use the only store</option>
                  <option value="first_available">First available</option>
                  <option value="nearest">Nearest (customer choice fallback)</option>
                  <option value="merchant_rule">Merchant rule (customer choice fallback)</option>
                </select>
              </label>
              <label className="full">Greeting<textarea value={workflow.greeting} onChange={(event) => updateWorkflow("greeting", event.target.value)} /></label>
            </div>
            <div className="workflow-options">
              <fieldset>
                <legend>Capabilities</legend>
                <label className="inline-check"><input type="checkbox" checked={workflow.status === "active"} onChange={(event) => updateWorkflow("status", event.target.checked ? "active" : "inactive")} />Workflow active</label>
                <label className="inline-check"><input type="checkbox" checked={workflowActionGroups.product_discovery.every((action) => workflow.enabled_action_list.includes(action))} onChange={(event) => toggleActionGroup("product_discovery", event.target.checked)} />Product discovery</label>
                <label className="inline-check"><input type="checkbox" checked={workflow.ordering_enabled} onChange={(event) => updateWorkflow("ordering_enabled", event.target.checked)} />Ordering</label>
                <label className="inline-check"><input type="checkbox" checked={workflow.payment_enabled} onChange={(event) => updateWorkflow("payment_enabled", event.target.checked)} />Payment</label>
                <label className="inline-check"><input type="checkbox" checked={workflowActionGroups.order_tracking.every((action) => workflow.enabled_action_list.includes(action))} onChange={(event) => toggleActionGroup("order_tracking", event.target.checked)} />Order tracking</label>
                <label className="inline-check"><input type="checkbox" checked={workflow.human_handoff_enabled} onChange={(event) => updateWorkflow("human_handoff_enabled", event.target.checked)} />Human handoff</label>
              </fieldset>
              <fieldset>
                <legend>Fulfilment offered</legend>
                {fulfilmentModes.map((mode) => <label className="inline-check" key={mode.value}><input type="checkbox" checked={workflow.supported_fulfilment_mode_list.includes(mode.value)} onChange={(event) => toggleWorkflowList("supported_fulfilment_mode_list", mode.value, event.target.checked)} />{mode.label}</label>)}
              </fieldset>
              <fieldset>
                <legend>After verified payment</legend>
                {postPaymentSteps.map((step) => <label className="inline-check" key={step.value}><input type="checkbox" checked={workflow.post_payment_step_list.includes(step.value)} onChange={(event) => toggleWorkflowList("post_payment_step_list", step.value, event.target.checked)} />{step.label}</label>)}
              </fieldset>
            </div>
            </fieldset>
            {canManageAssistant ? <div className="page-actions"><button type="submit" className="primary" disabled={savingWorkflow}>{savingWorkflow ? "Saving..." : "Save workflow settings"}</button></div> : <p className="read-only-note">Assistant workflow settings are read-only for your role.</p>}
          </form>
        </section>
      ) : null}

      {view === "test" ? <div className="split">
        <Card>
          <h3>What customers see first</h3>
          {menu.length === 0 ? <p className="muted">Enable at least one capability below.</p> : (
            <ol>
              {menu.map((module, index) => (
                <li key={module.id}>{index + 1}. {moduleHelp[module.module_key]?.title ?? module.name}</li>
              ))}
            </ol>
          )}
          <div className="page-actions" style={{ marginTop: 16 }}>
            <Link to="/conversations">View conversations</Link>
            <Link to="/knowledge">Knowledge</Link>
            <Link to="/settings/whatsapp">Channel settings</Link>
            <Link to="/advanced/bot-builder">Advanced</Link>
          </div>
        </Card>
        <Card>
          <h3>Test your assistant</h3>
          <p className="muted">This uses the published version if one exists, otherwise the draft. It does not contact real customers.</p>
          {!canManageAssistant ? <p className="read-only-note">Testing and publishing are managed by an organization administrator.</p> : !testSessionID ? <button type="button" onClick={() => void startTest()}>Start test</button> : (
            <>
              <div className="test-transcript">
                {testMessages.map((item, index) => (
                  <div key={index} className={`test-message ${item.from}`}>{item.text}</div>
                ))}
              </div>
              <form className="test-send" onSubmit={sendTest}>
                <input value={testInput} onChange={(event) => setTestInput(event.target.value)} placeholder="Type as a customer" />
                <button type="submit">Send</button>
              </form>
            </>
          )}
        </Card>
      </div> : null}

      {view === "capabilities" ? <>
      <h3>What should your assistant help with?</h3>
      <p className="help-text">Turn capabilities on or off, then publish. Order in this list is the order customers see in the menu.</p>
      <div className="capability-grid">
        {modules.map((module, index) => {
          const help = moduleHelp[module.module_key] ?? { title: module.name, summary: "A capability your assistant can perform." };
          const on = enabled(module);
          return (
            <article className="capability" key={module.id}>
              <div>
                <StatusDot live={on} label={on ? "On" : "Off"} />
                <h3>{help.title}</h3>
                <p>{help.summary}</p>
              </div>
              {canManageAssistant ? <div className="page-actions">
                <button type="button" disabled={index === 0} onClick={() => void moveModule(module, -1)}>Up</button>
                <button type="button" disabled={index === modules.length - 1} onClick={() => void moveModule(module, 1)}>Down</button>
                <button type="button" className={on ? undefined : "primary"} onClick={() => void toggleModule(module, !on)}>{on ? "Disable" : "Enable"}</button>
              </div> : null}
            </article>
          );
        })}
      </div>
      </> : null}
    </Page>
  );
}
