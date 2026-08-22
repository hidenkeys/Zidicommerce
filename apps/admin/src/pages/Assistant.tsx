import { FormEvent, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet, apiPatch, apiPost } from "../api/client";
import { Card, EmptyState, Flash, Page, StatusDot } from "../components/ui";
import { moduleHelp, parseJSON, relativeTime, type Row } from "../lib/format";

type Bot = Row & { id: string; name: string; status: string; published_version_id?: string; updated_at?: string };
type BotVersion = Row & { id: string; bot_id: string; version_number: number; status: string };
type BotModule = Row & { id: string; module_key: string; name: string; parameters?: string; metadata?: string; sort_order?: number; description?: string };
type BotConfig = { version: BotVersion; modules: BotModule[] };
type Channel = Row & { provider: string; display_name: string; display_number?: string; status: string };
type RuntimeMessage = { from: "customer" | "bot" | "debug"; text: string };

function enabled(module: BotModule) {
  return parseJSON<Record<string, unknown>>(module.metadata, {}).enabled !== false;
}

export function AssistantPage() {
  const [bots, setBots] = useState<Bot[]>([]);
  const [versions, setVersions] = useState<BotVersion[]>([]);
  const [config, setConfig] = useState<BotConfig | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [name, setName] = useState("");
  const [assistantType, setAssistantType] = useState("commerce");
  const [creating, setCreating] = useState(false);
  const [testSessionID, setTestSessionID] = useState("");
  const [testInput, setTestInput] = useState("");
  const [testMessages, setTestMessages] = useState<RuntimeMessage[]>([]);
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

  const bot = bots[0];
  const published = versions.find((version) => version.status === "published");
  const draft = versions.find((version) => version.status === "draft" || version.status === "validated") ?? versions[0];
  const whatsapp = channels.find((channel) => channel.provider === "whatsapp");
  const editable = config?.version.status !== "published" && config?.version.status !== "archived";

  async function load() {
    try {
      const [botResponse, channelResponse] = await Promise.all([apiGet<Bot[]>("/bots"), apiGet<Channel[]>("/channels")]);
      setBots(botResponse.data);
      setChannels(channelResponse.data);
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
    }
  }

  useEffect(() => {
    void load();
  }, []);

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
    setFlash("Published. New WhatsApp conversations will use this version. Existing chats stay on the version they started with.");
    await load();
  }

  async function startTest() {
    if (!bot) return;
    const channelID = channels[0]?.id;
    if (!channelID) {
      setMessage("Connect WhatsApp first, then you can test the assistant.");
      return;
    }
    const response = await apiPost<Row>("/runtime/test/start", {
      bot_id: bot.id,
      channel_id: channelID,
      external_conversation_id: `admin-test-${Date.now()}`,
      sender: "2348000000000",
    });
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

  if (!bot) {
    return (
      <Page title="Your assistant" description="Teach your assistant what to do for customers on WhatsApp.">
        <Flash message={message} />
        <Card>
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
        </Card>
      </Page>
    );
  }

  return (
    <Page
      title={bot.name}
      description="Teach your assistant what to do. Customers see changes after you publish."
      actions={
        <>
          <button type="button" onClick={() => void startTest()}>Test assistant</button>
          <button type="button" className="primary" onClick={() => void publish()}>Publish changes</button>
        </>
      }
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <div className="metrics">
        <article className="metric-card">
          <span>Status</span>
          <strong><StatusDot live={Boolean(bot.published_version_id) && bot.status === "active"} label={bot.published_version_id ? "Live" : "Draft"} /></strong>
        </article>
        <article className="metric-card">
          <span>WhatsApp</span>
          <strong>{whatsapp?.status === "active" ? whatsapp.display_number || "Connected" : "Not connected"}</strong>
        </article>
        <article className="metric-card">
          <span>Published</span>
          <strong>{published ? `v${published.version_number}` : "Never"}</strong>
          <p>{published ? relativeTime(published.updated_at ?? bot.updated_at) : "Publish when you are ready"}</p>
        </article>
        <article className="metric-card">
          <span>Editing</span>
          <strong>{editable ? `Draft v${config?.version.version_number}` : "Published copy"}</strong>
          <p>{editable ? "Safe to change" : "Create a draft by toggling a capability"}</p>
        </article>
      </div>

      <div className="split">
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
            <Link to="/settings/whatsapp">WhatsApp settings</Link>
            <Link to="/advanced/bot-builder">Advanced</Link>
          </div>
        </Card>
        <Card>
          <h3>Test your assistant</h3>
          <p className="muted">This uses the published version if one exists, otherwise the draft. It never sends WhatsApp messages.</p>
          {!testSessionID ? <button type="button" onClick={() => void startTest()}>Start test</button> : (
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
      </div>

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
              <div className="page-actions">
                <button type="button" disabled={index === 0} onClick={() => void moveModule(module, -1)}>Up</button>
                <button type="button" disabled={index === modules.length - 1} onClick={() => void moveModule(module, 1)}>Down</button>
                <button type="button" className={on ? undefined : "primary"} onClick={() => void toggleModule(module, !on)}>{on ? "Disable" : "Enable"}</button>
              </div>
            </article>
          );
        })}
      </div>
    </Page>
  );
}
