import { FormEvent, useEffect, useMemo, useState } from "react";
import { Route, Routes } from "react-router-dom";
import { apiGet, apiPatch, apiPost } from "./api/client";
import { RequireAuth } from "./auth";
import { PasswordRecovery } from "./components/PasswordRecovery";
import { LoginPage } from "./components/LoginPage";
import { Shell } from "./components/Shell";
import { Page, FormGrid } from "./components/ui";
import { AIChatPage } from "./pages/AIChat";
import { AssistantPage } from "./pages/Assistant";
import { BusinessPage } from "./pages/Business";
import { CataloguePage } from "./pages/Catalogue";
import { ConversationsPage } from "./pages/Conversations";
import { CustomersPage } from "./pages/Customers";
import { InventoryPage } from "./pages/Inventory";
import { KnowledgePage } from "./pages/Knowledge";
import { OrdersPage } from "./pages/Orders";
import { OrganizationsPage } from "./pages/Organizations";
import { OverviewPage } from "./pages/Overview";
import { PaymentsPage } from "./pages/Payments";
import { SetupPage } from "./pages/Setup";
import { StoresPage } from "./pages/Stores";
import { TeamPage } from "./pages/Team";
import { InviteAcceptPage } from "./pages/InviteAccept";
import { ChannelsPage } from "./pages/Channels";
import { JourneysPage } from "./pages/Journeys";
import { isUsableChannelStatus } from "./lib/channels";

type Row = Record<string, unknown>;
type ApiUser = { id: string; organization_id: string; email?: string; role: string };
type Bot = Row & { id: string; name: string; status: string; published_version_id?: string };
type BotVersion = Row & { id: string; bot_id: string; version_number: number; status: string; start_step_key: string };
type BotModule = Row & { id: string; module_key: string; name: string; parameters?: string; metadata?: string; sort_order?: number };
type BotVariable = Row & { id: string; name: string; type: string; scope: string };
type BotQuestion = Row & { id: string; question_key: string; text: string; type: string; response_mode: string; variable_name?: string };
type BotAction = Row & { id: string; action_key: string; action_type: string; name: string };
type BotCondition = Row & { id: string; condition_key: string; name: string };
type BotStep = Row & { id: string; step_key: string; title: string; type: string; message?: string; next_step_key?: string; sort_order?: number };
type BotConfig = {
  version: BotVersion;
  modules: BotModule[];
  variables: BotVariable[];
  questions: BotQuestion[];
  actions: BotAction[];
  conditions: BotCondition[];
  integrations: Row[];
  steps: BotStep[];
};
type ValidationResult = { valid: boolean; issues: { path: string; message: string }[] };
type BotModuleSpec = { key: string; name: string; category: string; description: string };
type ActionSpec = { key: string; name: string; description: string };
type QuestionTypeSpec = { key: string; name: string; response_modes: string[] };
type BotFAQ = Row & { id: string; question: string; answer: string; keywords?: string; status: string };
type ShareLink = { available: boolean; url?: string; encoded_text?: string; display_number?: string; message?: string; reason?: string };
type SetupStatus = { complete_count: number; total_count: number; ready: boolean; items: { key: string; label: string; complete: boolean; description: string }[] };
type Channel = Row & { id: string; provider: string; display_name: string; phone_number_id?: string; display_number?: string; status: string };
type PaymentConfiguration = Row & { id: string; provider: string; display_name: string; status: string; enabled: boolean; public_config?: string; secret_source?: string; has_secret?: boolean };
type RuntimeMessage = { from: "customer" | "bot" | "debug"; text: string };
function parseJSON<T>(value: string, fallback: T): T {
  try {
    return JSON.parse(value || "{}") as T;
  } catch {
    return fallback;
  }
}

const hiddenColumnNames = new Set([
  "id",
  "organization_id",
  "bot_id",
  "bot_version_id",
  "channel_id",
  "customer_id",
  "store_id",
  "variant_id",
  "product_id",
  "session_id",
  "published_version_id",
  "idempotency_key",
  "metadata",
  "secret_config",
  "config",
  "lock_version",
  "expected_input",
  "variables",
  "system_context",
  "external_conversation_id",
  "current_step_key",
]);

function isUUID(value: unknown) {
  return typeof value === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}

function money(minor: unknown, currency = "NGN") {
  const amount = Number(minor ?? 0) / 100;
  try {
    return new Intl.NumberFormat("en-NG", { style: "currency", currency, maximumFractionDigits: 0 }).format(amount);
  } catch {
    return `${currency} ${amount.toLocaleString()}`;
  }
}

function humanStatus(value: unknown) {
  const raw = String(value ?? "").replace(/_/g, " ");
  if (!raw) return "—";
  return raw.replace(/\b\w/g, (letter: string) => letter.toUpperCase());
}

function relativeTime(value: unknown) {
  const date = new Date(String(value ?? ""));
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString("en-NG", { hour: "2-digit", minute: "2-digit", day: "numeric", month: "short" });
}

function PaymentsScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [init, setInit] = useState({ order_id: "", email: "", provider: "test" });
  const [verify, setVerify] = useState({ reference: "" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<Row[]>("/payments");
      setRows(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function initialize(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/payments/initialize", { ...init, idempotency_key: `admin-${init.order_id}` });
    await load();
  }

  async function verifyPayment(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/payments/verify", verify);
    await load();
  }

  return (
    <section className="content">
      <div className="page-header"><div>
        <h1>Payment tools</h1>
        <p>Initialize an order payment or check a provider reference.</p></div>
      </div>
      <div className="split">
        <FormGrid onSubmit={initialize} title="Initialize payment">

          <label>Order ID<input aria-label="Order ID" placeholder="Order ID" value={init.order_id} onChange={(event) => setInit({ ...init, order_id: event.target.value })} /></label>
          <label>Email<input aria-label="Email" placeholder="Email" value={init.email} onChange={(event) => setInit({ ...init, email: event.target.value })} /></label>
          <label>Provider<input aria-label="Provider" placeholder="Provider" value={init.provider} onChange={(event) => setInit({ ...init, provider: event.target.value })} /></label>
          <div className="full"><button type="submit" className="primary">Initialize payment</button></div>
        </FormGrid>
        <FormGrid onSubmit={verifyPayment} title="Verify payment">

          <label>Reference<input aria-label="Reference" placeholder="Reference" value={verify.reference} onChange={(event) => setVerify({ reference: event.target.value })} /></label>
          <div className="full"><button type="submit" className="primary">Verify payment</button></div>
        </FormGrid>
      </div>
      <ResourceTable rows={rows} message={message} />
    </section>
  );
}

function FulfilmentScreen() {
  const [orderId, setOrderId] = useState("");
  const [row, setRow] = useState<Row | null>(null);
  const [message, setMessage] = useState("");

  async function load(event: FormEvent) {
    event.preventDefault();
    try {
      const response = await apiGet<Row>(`/fulfilment/${orderId}`);
      setRow(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  return (
    <section className="content">
      <div className="page-header"><div>
        <h1>Fulfilment</h1>
        <p>Find delivery and collection details using an order ID.</p></div>
      </div>
      <FormGrid onSubmit={load}>
        <label>Order ID<input aria-label="Order ID" placeholder="Order ID" value={orderId} onChange={(event) => setOrderId(event.target.value)} /></label>
        <button type="submit">Find fulfilment</button>
      </FormGrid>
      <ResourceTable rows={row ? [row] : []} message={message} />
    </section>
  );
}

function MerchantImportScreen() {
  const [body, setBody] = useState(JSON.stringify({ stores: [], categories: [], products: [], inventory: [], channels: [] }, null, 2));
  const [result, setResult] = useState<Row | null>(null);
  const [jobs, setJobs] = useState<Row[]>([]);
  const [message, setMessage] = useState("");

  async function loadJobs() {
    try {
      const response = await apiGet<Row[]>("/merchant-imports");
      setJobs(response.data);
    } catch {
      setJobs([]);
    }
  }

  useEffect(() => {
    void loadJobs();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setMessage("");
    setResult(null);
    try {
      const parsed = JSON.parse(body);
      const response = await apiPost<Row>("/merchant-imports/configuration", parsed);
      setResult(response.data);
      setMessage("Merchant configuration imported.");
      await loadJobs();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Import failed");
    }
  }

  return (
    <section className="content">
      <div className="page-header"><div>
        <h1>Import business data</h1>
        <p>Load stores, categories, products, images, inventory, fulfilment modes, and channels from one validated JSON document.</p></div>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <form className="import-form" onSubmit={submit}>
        <label>Configuration JSON<textarea aria-label="Configuration JSON" spellCheck={false} rows={18} value={body} onChange={(event) => setBody(event.target.value)} /></label>
        <div><button type="submit" className="primary">Import configuration</button></div>
      </form>
      {result ? <ResourceTable rows={[result]} title="Import result" /> : null}
      <ResourceTable rows={jobs} title="Recent import jobs" />
    </section>
  );
}

function AuditLogScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<Row[]>("/organizations/current/audit-logs");
      setRows(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <Page title="Audit log" description="Administrative events for organization, team, invitation, and store-access changes.">
      <ResourceTable rows={rows} message={message} />
    </Page>
  );
}

function BotBuilderScreen({ variant = "builder" }: { variant?: "simple" | "builder" }) {
  const [bots, setBots] = useState<Bot[]>([]);
  const [versions, setVersions] = useState<BotVersion[]>([]);
  const [config, setConfig] = useState<BotConfig | null>(null);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [faqs, setFaqs] = useState<BotFAQ[]>([]);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [paymentConfigs, setPaymentConfigs] = useState<PaymentConfiguration[]>([]);
  const [shareLink, setShareLink] = useState<ShareLink | null>(null);
  const [modules, setModules] = useState<BotModuleSpec[]>([]);
  const [actions, setActions] = useState<ActionSpec[]>([]);
  const [questionTypes, setQuestionTypes] = useState<QuestionTypeSpec[]>([]);
  const [activeTab, setActiveTab] = useState("overview");
  const [selectedBotID, setSelectedBotID] = useState("");
  const [selectedVersionID, setSelectedVersionID] = useState("");
  const [botForm, setBotForm] = useState({ name: "", description: "" });
  const [botTemplate, setBotTemplate] = useState("field_service");
  const [faqForm, setFaqForm] = useState({ question: "", answer: "", keywords: "" });
  const [faqQuery, setFaqQuery] = useState("");
  const [faqMatch, setFaqMatch] = useState<Row | null>(null);
  const [channelForm, setChannelForm] = useState({ provider: "whatsapp", display_name: "WhatsApp", phone_number_id: "", display_number: "", status: "active", config: "" });
  const [paymentForm, setPaymentForm] = useState({ provider: "paystack", display_name: "Paystack", enabled: true, public_key: "", secret_key: "", public_config: "{\"mode\":\"test\"}", secret_source: "environment" });
  const [testSessionID, setTestSessionID] = useState("");
  const [testInput, setTestInput] = useState("");
  const [testMessages, setTestMessages] = useState<RuntimeMessage[]>([]);
  const [moduleForm, setModuleForm] = useState({ module_key: "ORDER", require_payment: true });
  const [variableForm, setVariableForm] = useState({ name: "", type: "string", scope: "user", description: "" });
  const [questionForm, setQuestionForm] = useState({ question_key: "", text: "", type: "text", response_mode: "free_text", variable_name: "", options: "" });
  const [actionForm, setActionForm] = useState({ action_key: "", action_type: "get_store", name: "", input_name: "", input_variable: "", output_name: "", output_variable: "" });
  const [conditionForm, setConditionForm] = useState({ condition_key: "", name: "", combinator: "and", field: "", operator: "equals", value: "" });
  const [integrationForm, setIntegrationForm] = useState({ provider: "paystack", display_name: "Paystack", required: true, account_reference: "" });
  const [stepForm, setStepForm] = useState({ step_key: "", type: "message", title: "", message: "", response_mode: "free_text", next_step_key: "", fallback_step_key: "", question_id: "", module_id: "", action_id: "", condition_id: "", options: "", sort_order: "" });
  const [validation, setValidation] = useState<ValidationResult | null>(null);
  const [preview, setPreview] = useState<Row | null>(null);
  const [message, setMessage] = useState("");

  async function loadCatalogue() {
    const [moduleResponse, actionResponse, questionResponse] = await Promise.all([
      apiGet<BotModuleSpec[]>("/bot-modules"),
      apiGet<ActionSpec[]>("/bot-actions"),
      apiGet<QuestionTypeSpec[]>("/bot-question-types"),
    ]);
    setModules(moduleResponse.data);
    setActions(actionResponse.data);
    setQuestionTypes(questionResponse.data);
  }

  async function loadAuxiliary(nextBotID = selectedBotID) {
    const [setupResponse, faqResponse, channelResponse, paymentResponse] = await Promise.all([
      apiGet<SetupStatus>("/bot-setup/status"),
      apiGet<BotFAQ[]>("/bot-faqs"),
      apiGet<Channel[]>("/channels"),
      apiGet<PaymentConfiguration[]>("/payment-configurations"),
    ]);
    setSetupStatus(setupResponse.data);
    setFaqs(faqResponse.data);
    setChannels(channelResponse.data);
    setPaymentConfigs(paymentResponse.data);
    if (nextBotID) {
      const linkResponse = await apiGet<ShareLink>(`/bots/${nextBotID}/share-link`);
      setShareLink(linkResponse.data);
    } else {
      setShareLink(null);
    }
  }

  async function loadBots(nextBotID = selectedBotID) {
    const response = await apiGet<Bot[]>("/bots");
    setBots(response.data);
    const resolvedBotID = nextBotID || response.data[0]?.id || "";
    setSelectedBotID(resolvedBotID);
    if (resolvedBotID) {
      await loadVersions(resolvedBotID);
      await loadAuxiliary(resolvedBotID);
    } else {
      setVersions([]);
      setConfig(null);
      await loadAuxiliary("");
    }
  }

  async function loadVersions(botID = selectedBotID, nextVersionID = selectedVersionID) {
    if (!botID) return;
    const response = await apiGet<BotVersion[]>(`/bots/${botID}/versions`);
    setVersions(response.data);
    const resolvedVersionID = nextVersionID || response.data[0]?.id || "";
    setSelectedVersionID(resolvedVersionID);
    if (resolvedVersionID) {
      await loadConfig(resolvedVersionID);
    }
  }

  async function loadConfig(versionID = selectedVersionID) {
    if (!versionID) return;
    const response = await apiGet<BotConfig>(`/bot-versions/${versionID}/configuration`);
    setConfig(response.data);
  }

  useEffect(() => {
    void Promise.all([loadCatalogue(), loadBots()]);
  }, []);

  async function createBot(event: FormEvent) {
    event.preventDefault();
    const response = await apiPost<Bot>("/bots", { ...botForm, default_language: "en", timezone: "Africa/Lagos" });
    setBotForm({ name: "", description: "" });
    setMessage("Bot created with a draft version.");
    await loadBots(response.data.id);
  }

  async function createEmptyBot() {
    const response = await apiPost<Bot>("/bots", { ...botForm, default_language: "en", timezone: "Africa/Lagos" });
    setBotForm({ name: "", description: "" });
    setMessage("Empty bot created with a draft version.");
    await loadBots(response.data.id);
  }

  async function createSelfServiceBot(event: FormEvent) {
    event.preventDefault();
    const fieldService = botTemplate === "field_service";
    const response = await apiPost<Bot>(fieldService ? "/bots/service-booking" : "/bots/self-service", fieldService ? {
      name: botForm.name,
      welcome_message: "Welcome. I can help you book a trusted professional for your home-service request.",
    } : {
      ...botForm,
      welcome_message: "Welcome. I can help you place an order, track an order, answer questions, or contact support.",
      require_payment: true,
      allow_pickup: true,
      allow_customer_rider: true,
      allow_merchant_rider: false,
    });
    setBotForm({ name: "", description: "" });
    setMessage(fieldService ? "Field-service assistant created with booking, payment, matching, and handoff steps." : "Self-service commerce bot created with starter modules.");
    await loadBots(response.data.id);
  }

  async function createVersion() {
    const response = await apiPost<BotVersion>(`/bots/${selectedBotID}/versions`, { source_version_id: selectedVersionID || undefined });
    setMessage(`Draft v${response.data.version_number} created from the selected version.`);
    await loadVersions(selectedBotID, response.data.id);
  }

  async function addModule(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotModule>(`/bot-versions/${selectedVersionID}/modules`, {
      module_key: moduleForm.module_key,
      parameters: JSON.stringify({ require_payment: moduleForm.require_payment }),
    });
    await refreshAfterMutation("Module added.");
  }

  async function updateModule(module: BotModule, patch: Partial<BotModule>) {
    await apiPatch<BotModule>(`/bot-modules/${module.id}`, patch);
    await refreshAfterMutation("Module updated.");
  }

  async function setModuleEnabled(module: BotModule, enabled: boolean) {
    await apiPost<BotModule>(`/bot-modules/${module.id}/${enabled ? "enable" : "disable"}`, {});
    await refreshAfterMutation(enabled ? "Module enabled." : "Module disabled.");
  }

  async function moveModule(module: BotModule, direction: -1 | 1) {
    if (!config) return;
    const sorted = [...config.modules].sort((left, right) => Number(left.sort_order ?? 0) - Number(right.sort_order ?? 0));
    const index = sorted.findIndex((item) => item.id === module.id);
    const nextIndex = index + direction;
    if (index < 0 || nextIndex < 0 || nextIndex >= sorted.length) return;
    [sorted[index], sorted[nextIndex]] = [sorted[nextIndex], sorted[index]];
    await apiPost<BotModule[]>(`/bot-versions/${selectedVersionID}/modules/reorder`, {
      modules: sorted.map((item, position) => ({ module_id: item.id, sort_order: (position + 1) * 10 })),
    });
    await refreshAfterMutation("Modules reordered.");
  }

  async function addVariable(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotVariable>(`/bot-versions/${selectedVersionID}/variables`, clean(variableForm));
    setVariableForm({ name: "", type: "string", scope: "user", description: "" });
    await refreshAfterMutation("Variable added.");
  }

  async function addQuestion(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotQuestion>(`/bot-versions/${selectedVersionID}/questions`, {
      ...clean(questionForm),
      options: csvToJSON(questionForm.options),
    });
    setQuestionForm({ question_key: "", text: "", type: "text", response_mode: "free_text", variable_name: "", options: "" });
    await refreshAfterMutation("Question added.");
  }

  async function addAction(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotAction>(`/bot-versions/${selectedVersionID}/actions`, {
      action_key: actionForm.action_key,
      action_type: actionForm.action_type,
      name: actionForm.name,
      input_mappings: pairToJSON(actionForm.input_name, actionForm.input_variable),
      output_mappings: pairToJSON(actionForm.output_name, actionForm.output_variable),
    });
    setActionForm({ action_key: "", action_type: "get_store", name: "", input_name: "", input_variable: "", output_name: "", output_variable: "" });
    await refreshAfterMutation("Action added.");
  }

  async function addCondition(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotCondition>(`/bot-versions/${selectedVersionID}/conditions`, {
      condition_key: conditionForm.condition_key,
      name: conditionForm.name,
      combinator: conditionForm.combinator,
      rules: JSON.stringify([{ field: conditionForm.field, operator: conditionForm.operator, value: conditionForm.value }]),
    });
    setConditionForm({ condition_key: "", name: "", combinator: "and", field: "", operator: "equals", value: "" });
    await refreshAfterMutation("Condition added.");
  }

  async function addIntegration(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/bot-versions/${selectedVersionID}/integrations`, {
      provider: integrationForm.provider,
      display_name: integrationForm.display_name,
      required: integrationForm.required,
      config: JSON.stringify({ account_reference: integrationForm.account_reference }),
    });
    await refreshAfterMutation("Integration requirement added.");
  }

  async function addStep(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotStep>(`/bot-versions/${selectedVersionID}/steps`, {
      step_key: stepForm.step_key,
      type: stepForm.type,
      title: stepForm.title,
      message: stepForm.message,
      response_mode: stepForm.response_mode,
      next_step_key: stepForm.next_step_key,
      fallback_step_key: stepForm.fallback_step_key,
      question_id: stepForm.question_id || undefined,
      module_id: stepForm.module_id || undefined,
      action_id: stepForm.action_id || undefined,
      condition_id: stepForm.condition_id || undefined,
      options: csvToJSON(stepForm.options),
      sort_order: Number(stepForm.sort_order || 0),
    });
    setStepForm({ step_key: "", type: "message", title: "", message: "", response_mode: "free_text", next_step_key: "", fallback_step_key: "", question_id: "", module_id: "", action_id: "", condition_id: "", options: "", sort_order: "" });
    await refreshAfterMutation("Step added.");
  }

  async function validateVersion() {
    const response = await apiPost<ValidationResult>(`/bot-versions/${selectedVersionID}/validate`, {});
    setValidation(response.data);
    setMessage(response.data.valid ? "Configuration is valid." : "Validation found issues.");
    await loadConfig();
  }

  async function publishVersion() {
    const response = await apiPost<Row>(`/bot-versions/${selectedVersionID}/publish`, {});
    setMessage(`Published immutable snapshot ${String(response.data.id)}.`);
    await loadBots(selectedBotID);
  }

  async function addFAQ(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotFAQ>("/bot-faqs", { question: faqForm.question, answer: faqForm.answer, keywords: faqForm.keywords.split(",").map((item) => item.trim()).filter(Boolean), status: "active" });
    setFaqForm({ question: "", answer: "", keywords: "" });
    setMessage("FAQ added to bot knowledge.");
    await loadAuxiliary(selectedBotID);
  }

  async function runFAQMatch() {
    const response = await apiPost<Row>("/bot-faqs/match", { query: faqQuery });
    setFaqMatch(response.data);
  }

  async function saveChannel(event: FormEvent) {
    event.preventDefault();
    await apiPost<Channel>("/channels", { ...clean(channelForm), config: channelForm.config || "{}" });
    setChannelForm({ provider: "whatsapp", display_name: "WhatsApp", phone_number_id: "", display_number: "", status: "active", config: "" });
    setMessage("Channel saved.");
    await loadAuxiliary(selectedBotID);
  }

  async function testChannel(channelID: string) {
    const response = await apiPost<Row>(`/channels/${channelID}/test`, {});
    setMessage(`Channel test: ${String(response.data.status)}`);
  }

  async function disconnectChannel(channelID: string) {
    await apiPost<Channel>(`/channels/${channelID}/disconnect`, {});
    setMessage("Channel disconnected.");
    await loadAuxiliary(selectedBotID);
  }

  async function savePayment(event: FormEvent) {
    event.preventDefault();
    const publicConfig = parseJSON<Record<string, unknown>>(paymentForm.public_config, {});
    if (paymentForm.public_key.trim()) {
      publicConfig.public_key = paymentForm.public_key.trim();
    }
    const payload = {
      provider: paymentForm.provider,
      display_name: paymentForm.display_name,
      enabled: paymentForm.enabled,
      public_config: JSON.stringify(publicConfig),
      secret_source: paymentForm.secret_key.trim() ? "merchant_secret" : paymentForm.secret_source,
      secret_config: paymentForm.secret_key.trim() ? JSON.stringify({ secret_key: paymentForm.secret_key.trim() }) : "",
    };
    await apiPost<PaymentConfiguration>("/payment-configurations", payload);
    setPaymentForm({ ...paymentForm, secret_key: "", secret_source: payload.secret_source, public_config: payload.public_config });
    setMessage("Payment configuration saved.");
    await loadAuxiliary(selectedBotID);
  }

  async function testPayment(provider: string) {
    const response = await apiPost<PaymentConfiguration>(`/payment-configurations/${provider}/test`, {});
    setMessage(`Payment configuration test: ${response.data.status}`);
    await loadAuxiliary(selectedBotID);
  }

  async function startRuntimeTest() {
    const channelID = channels[0]?.id;
    if (!selectedBotID || !channelID) {
      setMessage("Select a bot and create a channel before testing.");
      return;
    }
    const response = await apiPost<Row>("/runtime/test/start", {
      bot_id: selectedBotID,
      channel_id: channelID,
      external_conversation_id: `admin-test-${Date.now()}`,
      sender: "2348000000000",
    });
    setTestSessionID(String(response.data.id));
    setTestMessages([{ from: "debug", text: `Started session ${String(response.data.id)}` }]);
  }

  async function sendRuntimeTest(event: FormEvent) {
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
    const outbound = Array.isArray(response.data.messages) ? response.data.messages as Row[] : [];
    setTestMessages((items) => [
      ...items,
      ...outbound.map((msg) => ({ from: "bot" as const, text: [String(msg.text ?? ""), Array.isArray(msg.options) ? `Options: ${(msg.options as Row[]).map((option) => option.label ?? option.id).join(", ")}` : ""].filter(Boolean).join("\n") })),
      { from: "debug", text: `Status: ${String(response.data.status ?? "unknown")}` },
    ]);
  }

  async function loadPreview() {
    const response = await apiGet<Row>(`/bot-versions/${selectedVersionID}/preview`);
    setPreview(response.data);
  }

  async function refreshAfterMutation(nextMessage: string) {
    setValidation(null);
    setPreview(null);
    setMessage(nextMessage);
    await loadConfig();
  }

  const editable = config?.version.status !== "published" && config?.version.status !== "archived";
  const selectedBot = bots.find((bot) => bot.id === selectedBotID);
  const customerMenu = config ? customerMenuModules(config.modules) : [];
  const selectedQuestionType = questionTypes.find((item) => item.key === questionForm.type);
  const tabs = variant === "simple"
    ? ["overview", "modules", "knowledge", "test", "publish"]
    : ["overview", "conversations", "modules", "knowledge", "variables", "integrations", "test", "publish", "advanced"];

  return (
    <section className="content bot-builder">
      <div className="page-header"><div>
        <h1>{variant === "simple" ? "My Bot" : "Bot Builder"}</h1>
        <p>{variant === "simple" ? "Control what customers see on WhatsApp. You do not need to know how the bot is built." : "Advanced configuration for conversation steps, actions, and published snapshots."}</p></div>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="split">
        <details className="legacy-create"><summary>Create a bot</summary><form className="resource-form" onSubmit={createSelfServiceBot}>
          <strong>Create self-service bot</strong>
          <input aria-label="Bot name" placeholder="Bot name" value={botForm.name} onChange={(event) => setBotForm({ ...botForm, name: event.target.value })} />
          <input aria-label="Description" placeholder="Description" value={botForm.description} onChange={(event) => setBotForm({ ...botForm, description: event.target.value })} />
          <label>Starter flow
            <select value={botTemplate} onChange={(event) => setBotTemplate(event.target.value)}>
              <option value="field_service">Field service / handyman</option>
              <option value="commerce">Products and orders</option>
            </select>
          </label>
          <button type="submit">Create guided bot</button>
          <button type="button" onClick={createEmptyBot}>Create empty bot</button>
        </form></details>
        <div className="table-wrap bot-toolbar">
          <label>
            Bot
            <select value={selectedBotID} onChange={(event) => { setSelectedBotID(event.target.value); void loadVersions(event.target.value, ""); void loadAuxiliary(event.target.value); }}>
              <option value="">Select bot</option>
              {bots.map((bot) => <option key={bot.id} value={bot.id}>{bot.name} · {bot.status}</option>)}
            </select>
          </label>
          <label>
            Version
            <select value={selectedVersionID} onChange={(event) => { setSelectedVersionID(event.target.value); void loadConfig(event.target.value); }}>
              <option value="">Select version</option>
              {versions.map((version) => <option key={version.id} value={version.id}>v{version.version_number} · {version.status}</option>)}
            </select>
          </label>
          <button type="button" disabled={!selectedBotID || !selectedVersionID} onClick={createVersion}>New draft from selected</button>
          <button type="button" disabled={!selectedVersionID} onClick={validateVersion}>Validate</button>
          <button type="button" disabled={!selectedVersionID} onClick={loadPreview}>Preview</button>
          <button type="button" disabled={!selectedVersionID || validation?.valid === false} onClick={publishVersion}>Publish</button>
        </div>
      </div>
      <div className="tab-bar">
        {tabs.map((tab) => (
          <button key={tab} type="button" className={activeTab === tab ? "tab active" : "tab"} onClick={() => setActiveTab(tab)}>
            {tab}
          </button>
        ))}
      </div>

      {config ? (
        <>
          {variant === "builder" ? (
          <div className="grid">
            <article className="summary-card"><span>Version</span><p>v{config.version.version_number} · {config.version.status}</p></article>
            <article className="summary-card"><span>Start step</span><p>{config.version.start_step_key}</p></article>
            <article className="summary-card"><span>Modules</span><p>{config.modules.length}</p></article>
            <article className="summary-card"><span>Steps</span><p>{config.steps.length}</p></article>
          </div>
          ) : null}
          {!editable ? <p className="muted">This version is immutable. Create a new draft from it to make changes.</p> : null}

          {activeTab === "overview" ? (
            <div className="split">
              <div className="table-wrap">
                <span className="eyebrow">Status</span>
                <h3>{whatsappConnected(channels) ? "Connected" : "Not connected"}</h3>
                <p>{selectedBot?.name || "Customer assistant"}</p>
                <p className="muted">{shareLink?.display_number || "Connect WhatsApp in Settings to go live."}</p>
                <ol className="menu-preview">
                  {customerMenu.map((module, index) => (
                    <li key={module.id}>{index + 1}. {module.label}</li>
                  ))}
                </ol>
              </div>
              <div className="table-wrap checklist-panel">
                <h3>Launch checklist</h3>
                <div className="checklist compact">
                  {(setupStatus?.items ?? []).map((item) => (
                    <button key={item.key} className={item.complete ? "step complete" : "step"} type="button">
                      <span>{item.complete ? "✓" : "○"}</span>
                      {item.label}
                    </button>
                  ))}
                </div>
                <p className="muted">{setupStatus ? `${setupStatus.complete_count} of ${setupStatus.total_count} setup checks complete.` : "Loading setup state..."}</p>
              </div>
              <div className="table-wrap share-panel">
                <h3>Customer entry link</h3>
                {shareLink?.available && shareLink.url ? (
                  <>
                    <code>{shareLink.url}</code>
                    <div className="qr-placeholder" aria-label="QR payload">{shareLink.display_number}</div>
                    <button type="button" onClick={() => navigator.clipboard.writeText(shareLink.url ?? "")}>Copy link</button>
                  </>
                ) : (
                  <p className="muted">{shareLink?.reason ?? "Publish the bot and connect WhatsApp to generate a share link."}</p>
                )}
              </div>
            </div>
          ) : null}

          {activeTab === "conversations" ? (
            <div className="split">
              <ResourceTable rows={config.steps} title="Conversation steps" />
              <ResourceTable rows={config.questions} title="Questions" />
            </div>
          ) : null}

          {activeTab === "modules" ? (
            <div className="module-manager">
              <section className="module-menu-preview">
                <div>
                  <span className="eyebrow">Customer entry menu</span>
                  <h3>Published runtime order</h3>
                  <p>
                    Enabled modules appear in this order in the customer-facing bot menu. Disabled modules are hidden from new published snapshots.
                  </p>
                  <p className="muted">
                    {editable ? "You are editing a draft. Publish this version before customers see these module changes." : "This version is published and immutable. Create a new draft for changes."}
                  </p>
                </div>
                <ol>
                  {customerMenu.length ? customerMenu.map((module) => (
                    <li key={`${module.intent}-${module.id}`}>
                      <span>{module.label}</span>
                      <code>{module.intent}</code>
                    </li>
                  )) : <li className="muted">No enabled customer menu modules.</li>}
                </ol>
              </section>
              {[...config.modules].sort((left, right) => Number(left.sort_order ?? 0) - Number(right.sort_order ?? 0)).map((module, index) => {
                const enabled = moduleIsEnabled(module);
                return (
                  <article className="module-row" key={module.id}>
                    <div>
                      <span className={enabled ? "status-pill active" : "status-pill"}>{enabled ? "Enabled" : "Disabled"}</span>
                      <h3>{module.name}</h3>
                      <p>{String(module.description ?? "Configured module")}</p>
                      <code>{module.module_key}</code>
                    </div>
                    <div className="module-controls">
                      <input disabled={!editable} value={module.name} onChange={(event) => void updateModule(module, { name: event.target.value })} />
                      <input disabled={!editable} type="number" value={Number(module.sort_order ?? 0)} onChange={(event) => void updateModule(module, { sort_order: Number(event.target.value) })} />
                      <button type="button" disabled={!editable || index === 0} onClick={() => moveModule(module, -1)}>Move up</button>
                      <button type="button" disabled={!editable || index === config.modules.length - 1} onClick={() => moveModule(module, 1)}>Move down</button>
                      <button type="button" disabled={!editable} onClick={() => setModuleEnabled(module, !enabled)}>{enabled ? "Disable" : "Enable"}</button>
                    </div>
                  </article>
                );
              })}
            </div>
          ) : null}

          {activeTab === "knowledge" ? (
            <div className="split">
              <form className="resource-form" onSubmit={addFAQ}>
                <strong>Knowledge / FAQs</strong>
                <input aria-label="Customer question" placeholder="Customer question" value={faqForm.question} onChange={(event) => setFaqForm({ ...faqForm, question: event.target.value })} />
                <textarea placeholder="Answer" value={faqForm.answer} onChange={(event) => setFaqForm({ ...faqForm, answer: event.target.value })} />
                <input aria-label="Keywords, comma separated" placeholder="Keywords, comma separated" value={faqForm.keywords} onChange={(event) => setFaqForm({ ...faqForm, keywords: event.target.value })} />
                <button type="submit">Add FAQ</button>
              </form>
              <div className="table-wrap match-panel">
                <h3>Test FAQ match</h3>
                <input aria-label="Ask a sample question" placeholder="Ask a sample question" value={faqQuery} onChange={(event) => setFaqQuery(event.target.value)} />
                <button type="button" onClick={runFAQMatch}>Match</button>
                {faqMatch ? <pre>{JSON.stringify(faqMatch, null, 2)}</pre> : null}
              </div>
              <ResourceTable rows={faqs} title="Configured FAQs" />
            </div>
          ) : null}

          {activeTab === "variables" ? (
            <div className="split">
              <ResourceTable rows={config.variables} title="Bot variables" />
              <div className="table-wrap">
                <h3>System variables</h3>
                <p className="muted">System variables are read-only runtime values, such as the WhatsApp sender phone and channel context. Merchant variables are editable in Advanced.</p>
              </div>
            </div>
          ) : null}

          {activeTab === "integrations" ? (
            <div className="split">
              <form className="resource-form" onSubmit={saveChannel}>
                <strong>WhatsApp channel</strong>
                <input aria-label="Display name" placeholder="Display name" value={channelForm.display_name} onChange={(event) => setChannelForm({ ...channelForm, display_name: event.target.value })} />
                <input aria-label="Phone number ID" placeholder="Phone number ID" value={channelForm.phone_number_id} onChange={(event) => setChannelForm({ ...channelForm, phone_number_id: event.target.value })} />
                <input aria-label="Display number" placeholder="Display number" value={channelForm.display_number} onChange={(event) => setChannelForm({ ...channelForm, display_number: event.target.value })} />
                <textarea placeholder='Public config JSON, e.g. {"webhook_path":"/runtime/webhooks/whatsapp"}' value={channelForm.config} onChange={(event) => setChannelForm({ ...channelForm, config: event.target.value })} />
                <button type="submit">Save channel</button>
              </form>
              <form className="resource-form" onSubmit={savePayment}>
                <strong>Payment provider</strong>
                <input aria-label="Provider" placeholder="Provider" value={paymentForm.provider} onChange={(event) => setPaymentForm({ ...paymentForm, provider: event.target.value })} />
                <input aria-label="Display name" placeholder="Display name" value={paymentForm.display_name} onChange={(event) => setPaymentForm({ ...paymentForm, display_name: event.target.value })} />
                <input aria-label="Paystack public key" placeholder="Paystack public key" value={paymentForm.public_key} onChange={(event) => setPaymentForm({ ...paymentForm, public_key: event.target.value })} />
                <input type="password" placeholder="Paystack secret key" value={paymentForm.secret_key} onChange={(event) => setPaymentForm({ ...paymentForm, secret_key: event.target.value })} />
                <input aria-label="Secret source" placeholder="Secret source" value={paymentForm.secret_source} onChange={(event) => setPaymentForm({ ...paymentForm, secret_source: event.target.value })} />
                <textarea value={paymentForm.public_config} onChange={(event) => setPaymentForm({ ...paymentForm, public_config: event.target.value })} />
                <label className="inline-check"><input type="checkbox" checked={paymentForm.enabled} onChange={(event) => setPaymentForm({ ...paymentForm, enabled: event.target.checked })} /> Enabled</label>
                <button type="submit">Save payment config</button>
              </form>
              <div className="table-wrap">
                <h3>Channels</h3>
                {channels.map((channel) => (
                  <div className="row-actions" key={channel.id}>
                    <span>{channel.display_name} · {channel.status}</span>
                    <button type="button" onClick={() => testChannel(channel.id)}>Test</button>
                    <button type="button" onClick={() => disconnectChannel(channel.id)}>Disconnect</button>
                  </div>
                ))}
              </div>
              <div className="table-wrap">
                <h3>Payments</h3>
                {paymentConfigs.map((payment) => (
                  <div className="row-actions" key={payment.id}>
                    <span>{payment.display_name} · {payment.status} · {payment.enabled ? "enabled" : "disabled"} · {payment.has_secret ? "secret saved" : payment.secret_source || "environment"}</span>
                    <button type="button" onClick={() => testPayment(payment.provider)}>Test</button>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {activeTab === "test" ? (
            <div className="table-wrap tester">
              <h3>Bot test</h3>
              <button type="button" onClick={startRuntimeTest}>Start test session</button>
              <div className="test-transcript">
                {testMessages.map((item, index) => <p key={`${item.from}-${index}`} className={`test-message ${item.from}`}>{item.text}</p>)}
              </div>
              <form className="test-send" onSubmit={sendRuntimeTest}>
                <input aria-label="Send a customer message" placeholder="Send a customer message" value={testInput} onChange={(event) => setTestInput(event.target.value)} />
                <button type="submit" disabled={!testSessionID}>Send</button>
              </form>
            </div>
          ) : null}

          {activeTab === "publish" ? (
            <div className="split">
              <div className="table-wrap">
                <h3>Validate and publish</h3>
                <button type="button" onClick={validateVersion}>Validate version</button>
                <button type="button" disabled={validation?.valid === false} onClick={publishVersion}>Publish immutable snapshot</button>
                {validation ? <pre>{JSON.stringify(validation, null, 2)}</pre> : <p className="muted">Run validation before publishing.</p>}
              </div>
              <div className="table-wrap">
                <h3>Preview</h3>
                <button type="button" onClick={loadPreview}>Load preview</button>
                {preview ? <pre>{JSON.stringify(preview, null, 2)}</pre> : null}
              </div>
            </div>
          ) : null}

          {activeTab === "advanced" ? (
          <>
          <div className="bot-panel">
            <form className="resource-form" onSubmit={addModule}>
              <strong>Module</strong>
              <select disabled={!editable} value={moduleForm.module_key} onChange={(event) => setModuleForm({ ...moduleForm, module_key: event.target.value })}>
                {modules.map((module) => <option key={module.key} value={module.key}>{module.name}</option>)}
              </select>
              <label className="inline-check"><input disabled={!editable} type="checkbox" checked={moduleForm.require_payment} onChange={(event) => setModuleForm({ ...moduleForm, require_payment: event.target.checked })} /> Require payment for order module</label>
              <button disabled={!editable} type="submit">Add module</button>
            </form>
            <form className="resource-form" onSubmit={addVariable}>
              <strong>Variable</strong>
              <input disabled={!editable} placeholder="customer_name" value={variableForm.name} onChange={(event) => setVariableForm({ ...variableForm, name: event.target.value })} />
              <select disabled={!editable} value={variableForm.type} onChange={(event) => setVariableForm({ ...variableForm, type: event.target.value })}>
                {["string", "number", "boolean", "date", "datetime", "location", "object", "array"].map((type) => <option key={type} value={type}>{type}</option>)}
              </select>
              <select disabled={!editable} value={variableForm.scope} onChange={(event) => setVariableForm({ ...variableForm, scope: event.target.value })}>
                {["user", "conversation", "module", "system"].map((scope) => <option key={scope} value={scope}>{scope}</option>)}
              </select>
              <input disabled={!editable} placeholder="Description" value={variableForm.description} onChange={(event) => setVariableForm({ ...variableForm, description: event.target.value })} />
              <button disabled={!editable} type="submit">Add variable</button>
            </form>
            <form className="resource-form" onSubmit={addQuestion}>
              <strong>Question</strong>
              <input disabled={!editable} placeholder="question key" value={questionForm.question_key} onChange={(event) => setQuestionForm({ ...questionForm, question_key: event.target.value })} />
              <input disabled={!editable} placeholder="Question text" value={questionForm.text} onChange={(event) => setQuestionForm({ ...questionForm, text: event.target.value })} />
              <select disabled={!editable} value={questionForm.type} onChange={(event) => setQuestionForm({ ...questionForm, type: event.target.value, response_mode: questionTypes.find((item) => item.key === event.target.value)?.response_modes[0] ?? "free_text" })}>
                {questionTypes.map((type) => <option key={type.key} value={type.key}>{type.name}</option>)}
              </select>
              <select disabled={!editable} value={questionForm.response_mode} onChange={(event) => setQuestionForm({ ...questionForm, response_mode: event.target.value })}>
                {(selectedQuestionType?.response_modes ?? ["free_text"]).map((mode) => <option key={mode} value={mode}>{mode}</option>)}
              </select>
              <select disabled={!editable} value={questionForm.variable_name} onChange={(event) => setQuestionForm({ ...questionForm, variable_name: event.target.value })}>
                <option value="">No variable</option>
                {config.variables.map((variable) => <option key={variable.id} value={variable.name}>{variable.name}</option>)}
              </select>
              <input disabled={!editable} placeholder="Options, comma separated" value={questionForm.options} onChange={(event) => setQuestionForm({ ...questionForm, options: event.target.value })} />
              <button disabled={!editable} type="submit">Add question</button>
            </form>
            <form className="resource-form" onSubmit={addAction}>
              <strong>Action</strong>
              <input disabled={!editable} placeholder="action key" value={actionForm.action_key} onChange={(event) => setActionForm({ ...actionForm, action_key: event.target.value })} />
              <select disabled={!editable} value={actionForm.action_type} onChange={(event) => setActionForm({ ...actionForm, action_type: event.target.value })}>
                {actions.map((action) => <option key={action.key} value={action.key}>{action.name}</option>)}
              </select>
              <input disabled={!editable} placeholder="Action name" value={actionForm.name} onChange={(event) => setActionForm({ ...actionForm, name: event.target.value })} />
              <input disabled={!editable} placeholder="Input field" value={actionForm.input_name} onChange={(event) => setActionForm({ ...actionForm, input_name: event.target.value })} />
              <input disabled={!editable} placeholder="Input variable" value={actionForm.input_variable} onChange={(event) => setActionForm({ ...actionForm, input_variable: event.target.value })} />
              <input disabled={!editable} placeholder="Output field" value={actionForm.output_name} onChange={(event) => setActionForm({ ...actionForm, output_name: event.target.value })} />
              <input disabled={!editable} placeholder="Output variable" value={actionForm.output_variable} onChange={(event) => setActionForm({ ...actionForm, output_variable: event.target.value })} />
              <button disabled={!editable} type="submit">Add action</button>
            </form>
            <form className="resource-form" onSubmit={addCondition}>
              <strong>Condition</strong>
              <input disabled={!editable} placeholder="condition key" value={conditionForm.condition_key} onChange={(event) => setConditionForm({ ...conditionForm, condition_key: event.target.value })} />
              <input disabled={!editable} placeholder="Condition name" value={conditionForm.name} onChange={(event) => setConditionForm({ ...conditionForm, name: event.target.value })} />
              <select disabled={!editable} value={conditionForm.combinator} onChange={(event) => setConditionForm({ ...conditionForm, combinator: event.target.value })}>
                <option value="and">and</option>
                <option value="or">or</option>
              </select>
              <input disabled={!editable} placeholder="Field or variable" value={conditionForm.field} onChange={(event) => setConditionForm({ ...conditionForm, field: event.target.value })} />
              <select disabled={!editable} value={conditionForm.operator} onChange={(event) => setConditionForm({ ...conditionForm, operator: event.target.value })}>
                {["equals", "not_equals", "contains", "greater_than", "less_than", "exists", "not_exists", "in", "not_in"].map((operator) => <option key={operator} value={operator}>{operator}</option>)}
              </select>
              <input disabled={!editable} placeholder="Value" value={conditionForm.value} onChange={(event) => setConditionForm({ ...conditionForm, value: event.target.value })} />
              <button disabled={!editable} type="submit">Add condition</button>
            </form>
            <form className="resource-form" onSubmit={addIntegration}>
              <strong>Integration requirement</strong>
              <input disabled={!editable} placeholder="Provider" value={integrationForm.provider} onChange={(event) => setIntegrationForm({ ...integrationForm, provider: event.target.value })} />
              <input disabled={!editable} placeholder="Display name" value={integrationForm.display_name} onChange={(event) => setIntegrationForm({ ...integrationForm, display_name: event.target.value })} />
              <input disabled={!editable} placeholder="Account reference, no secrets" value={integrationForm.account_reference} onChange={(event) => setIntegrationForm({ ...integrationForm, account_reference: event.target.value })} />
              <label className="inline-check"><input disabled={!editable} type="checkbox" checked={integrationForm.required} onChange={(event) => setIntegrationForm({ ...integrationForm, required: event.target.checked })} /> Required</label>
              <button disabled={!editable} type="submit">Add integration</button>
            </form>
            <form className="resource-form wide" onSubmit={addStep}>
              <strong>Step</strong>
              <input disabled={!editable} placeholder="step key" value={stepForm.step_key} onChange={(event) => setStepForm({ ...stepForm, step_key: event.target.value })} />
              <select disabled={!editable} value={stepForm.type} onChange={(event) => setStepForm({ ...stepForm, type: event.target.value })}>
                {["message", "question", "choice", "module", "condition", "action", "handoff", "end"].map((type) => <option key={type} value={type}>{type}</option>)}
              </select>
              <input disabled={!editable} placeholder="Title" value={stepForm.title} onChange={(event) => setStepForm({ ...stepForm, title: event.target.value })} />
              <input disabled={!editable} placeholder="Next step key" value={stepForm.next_step_key} onChange={(event) => setStepForm({ ...stepForm, next_step_key: event.target.value })} />
              <input disabled={!editable} placeholder="Fallback step key" value={stepForm.fallback_step_key} onChange={(event) => setStepForm({ ...stepForm, fallback_step_key: event.target.value })} />
              <select disabled={!editable} value={stepForm.question_id} onChange={(event) => setStepForm({ ...stepForm, question_id: event.target.value })}>
                <option value="">No question</option>
                {config.questions.map((question) => <option key={question.id} value={question.id}>{question.question_key}</option>)}
              </select>
              <select disabled={!editable} value={stepForm.module_id} onChange={(event) => setStepForm({ ...stepForm, module_id: event.target.value })}>
                <option value="">No module</option>
                {config.modules.map((module) => <option key={module.id} value={module.id}>{module.module_key}</option>)}
              </select>
              <select disabled={!editable} value={stepForm.action_id} onChange={(event) => setStepForm({ ...stepForm, action_id: event.target.value })}>
                <option value="">No action</option>
                {config.actions.map((action) => <option key={action.id} value={action.id}>{action.action_key}</option>)}
              </select>
              <select disabled={!editable} value={stepForm.condition_id} onChange={(event) => setStepForm({ ...stepForm, condition_id: event.target.value })}>
                <option value="">No condition</option>
                {config.conditions.map((condition) => <option key={condition.id} value={condition.id}>{condition.condition_key}</option>)}
              </select>
              <input disabled={!editable} placeholder="Options, comma separated" value={stepForm.options} onChange={(event) => setStepForm({ ...stepForm, options: event.target.value })} />
              <input disabled={!editable} placeholder="Sort order" type="number" value={stepForm.sort_order} onChange={(event) => setStepForm({ ...stepForm, sort_order: event.target.value })} />
              <textarea disabled={!editable} placeholder="Message" value={stepForm.message} onChange={(event) => setStepForm({ ...stepForm, message: event.target.value })} />
              <button disabled={!editable} type="submit">Add step</button>
            </form>
          </div>

          <div className="split">
            <ResourceTable rows={config.modules} title="Modules" />
            <ResourceTable rows={config.variables} title="Variables" />
            <ResourceTable rows={config.questions} title="Questions" />
            <ResourceTable rows={config.steps} title="Steps" />
          </div>
          <div className="split">
            <ResourceTable rows={config.actions} title="Actions" />
            <ResourceTable rows={config.conditions} title="Conditions" />
            <ResourceTable rows={config.integrations} title="Integrations" />
            <div className="table-wrap">
              <h3>Validation & preview</h3>
              {validation ? <pre>{JSON.stringify(validation, null, 2)}</pre> : <p className="muted">Run validation before publishing.</p>}
              {preview ? <pre>{JSON.stringify(preview, null, 2)}</pre> : null}
            </div>
          </div>
          </>
          ) : null}
        </>
      ) : (
        <div className="empty-state">
          <strong>No bot selected</strong>
          <span>Create or select a bot to configure a version.</span>
        </div>
      )}
    </section>
  );
}

function ResourceTable({ rows, loading, message, title }: { rows: Row[]; loading?: boolean; message?: string; title?: string }) {
  const columns = useMemo(() => Array.from(new Set(rows.flatMap((row) => Object.keys(row)))).filter((column) => !hiddenColumnNames.has(column) && !column.endsWith("_id")).slice(0, 8), [rows]);
  return (
    <div className="table-wrap">
      {title ? <h3>{title}</h3> : null}
      {message ? <p className="error-text">{message}</p> : null}
      {loading ? <p>Loading...</p> : null}
      {!loading && rows.length === 0 ? <p className="muted">No records found.</p> : null}
      {rows.length > 0 ? (
        <table>
          <thead>
            <tr>{columns.map((column) => <th key={column}>{column.replace(/_minor$/, "").replace(/_/g, " ")}</th>)}</tr>
          </thead>
          <tbody>
            {rows.map((row, index) => (
              <tr key={String(row.id ?? index)}>
                {columns.map((column) => <td key={column}>{column.endsWith("_minor") ? money(row[column], String(row.currency || "NGN")) : column.endsWith("_at") ? relativeTime(row[column]) : ["status", "action", "target_type"].includes(column) ? humanStatus(row[column]) : formatCell(row[column])}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}

function Placeholder({ title, body = "This address does not match an available workspace page." }: { title: string; body?: string }) {
  return <Page title={title} description={body}><a className="button primary" href="/">Return to overview</a></Page>;
}

function clean(form: Record<string, string>) {
  return Object.fromEntries(Object.entries(form).filter(([, value]) => value.trim() !== ""));
}

function csvToJSON(value: string) {
  return JSON.stringify(value.split(",").map((item) => item.trim()).filter(Boolean));
}

function pairToJSON(name: string, variable: string) {
  if (!name.trim() || !variable.trim()) return "{}";
  return JSON.stringify({ [name.trim()]: variable.trim() });
}

function parseObject(raw?: string) {
  if (!raw) return {} as Record<string, unknown>;
  try {
    return JSON.parse(raw) as Record<string, unknown>;
  } catch {
    return {} as Record<string, unknown>;
  }
}

function moduleIsEnabled(module: BotModule) {
  const metadata = parseObject(module.metadata);
  return metadata.enabled !== false;
}

function customerMenuModules(modules: BotModule[]) {
  const seen = new Set<string>();
  return [...modules]
    .filter(moduleIsEnabled)
    .sort((left, right) => Number(left.sort_order ?? 0) - Number(right.sort_order ?? 0))
    .map((module) => ({ id: module.id, intent: moduleMenuIntent(module), label: moduleMenuLabel(module) }))
    .filter((module) => {
      if (!module.intent || seen.has(module.intent)) return false;
      seen.add(module.intent);
      return true;
    });
}

function moduleMenuIntent(module: BotModule) {
  const parameters = parseObject(module.parameters);
  const explicit = String(parameters.menu_intent ?? "").trim();
  if (explicit) return explicit;
  switch (module.module_key) {
    case "ORDER":
      return "order";
    case "TRACK_ORDER":
      return "track_order";
    case "FAQ":
      return "faq";
    case "COMPLAINT":
      return "complaint";
    case "CONTACT_SUPPORT":
    case "HUMAN_HANDOFF":
      return "support";
    default:
      return "";
  }
}

function moduleMenuLabel(module: BotModule) {
  const parameters = parseObject(module.parameters);
  return String(parameters.menu_label ?? module.name ?? module.module_key).trim();
}

function formatCell(value: unknown) {
  if (value === null || value === undefined) return "";
  if (isUUID(value)) return "";
  if (typeof value === "object") return JSON.stringify(value).slice(0, 80);
  return String(value);
}

function whatsappConnected(channels: Channel[]) {
  return channels.some((channel) => channel.provider === "whatsapp" && isUsableChannelStatus(channel.status));
}

export default function App() {
  return (
    <Routes>
      <Route path="/forgot-password" element={<PasswordRecovery />} />
      <Route path="/reset-password" element={<PasswordRecovery key="reset" reset />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<LoginPage mode="register" />} />
      <Route path="/invitations/accept" element={<InviteAcceptPage />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route index element={<OverviewPage />} />
          <Route path="setup" element={<SetupPage />} />
          <Route path="orders" element={<OrdersPage />} />
          <Route path="catalogue" element={<CataloguePage />} />
          <Route path="inventory" element={<InventoryPage />} />
          <Route path="customers" element={<CustomersPage />} />
          <Route path="stores" element={<StoresPage />} />
          <Route path="assistant" element={<AssistantPage />} />
          <Route path="assistant/ai-chat" element={<AIChatPage />} />
          <Route path="conversations" element={<ConversationsPage />} />
          <Route path="knowledge" element={<KnowledgePage />} />
          <Route path="journeys" element={<JourneysPage />} />
          <Route path="team" element={<TeamPage />} />
          <Route path="settings/business" element={<BusinessPage />} />
          <Route path="settings/payments" element={<PaymentsPage />} />
          <Route path="settings/whatsapp" element={<ChannelsPage />} />
          <Route path="settings/whatsapp/:connectionId" element={<ChannelsPage />} />
          <Route path="advanced/bot-builder" element={<BotBuilderScreen variant="builder" />} />
          <Route path="advanced/payments" element={<PaymentsScreen />} />
          <Route path="advanced/delivery" element={<FulfilmentScreen />} />
          <Route path="advanced/import" element={<MerchantImportScreen />} />
          <Route path="advanced/audit-logs" element={<AuditLogScreen />} />
          <Route path="platform/organizations" element={<OrganizationsPage />} />
          <Route path="sell/orders" element={<OrdersPage />} />
          <Route path="sell/catalogue" element={<CataloguePage />} />
          <Route path="sell/inventory" element={<InventoryPage />} />
          <Route path="sell/customers" element={<CustomersPage />} />
          <Route path="automation/bot" element={<AssistantPage />} />
          <Route path="automation/conversations" element={<ConversationsPage />} />
          <Route path="automation/knowledge" element={<KnowledgePage />} />
          <Route path="business/stores" element={<StoresPage />} />
          <Route path="business/team" element={<TeamPage />} />
          <Route path="business/payments" element={<PaymentsPage />} />
          <Route path="business/delivery" element={<FulfilmentScreen />} />
          <Route path="settings/integrations" element={<PaymentsPage />} />
          <Route path="advanced/readiness" element={<SetupPage />} />
          <Route path="advanced/access" element={<TeamPage />} />
          <Route path="commerce/stores" element={<StoresPage />} />
          <Route path="commerce/catalogue" element={<CataloguePage />} />
          <Route path="commerce/inventory" element={<InventoryPage />} />
          <Route path="commerce/orders" element={<OrdersPage />} />
          <Route path="commerce/customers" element={<CustomersPage />} />
          <Route path="organization/business" element={<BusinessPage />} />
          <Route path="organization/team" element={<TeamPage />} />
          <Route path="organization/access" element={<TeamPage />} />
          <Route path="organization/audit-logs" element={<AuditLogScreen />} />
          <Route path="configuration/payments" element={<PaymentsScreen />} />
          <Route path="configuration/fulfilment" element={<FulfilmentScreen />} />
          <Route path="configuration/channels" element={<ChannelsPage />} />
          <Route path="automation/bots" element={<AssistantPage />} />
          <Route path="automation/versions" element={<BotBuilderScreen variant="builder" />} />
          <Route path="automation/support-handoffs" element={<ConversationsPage />} />
          <Route path="settings/readiness" element={<SetupPage />} />
          <Route path="settings/import" element={<MerchantImportScreen />} />
          <Route path="settings" element={<BusinessPage />} />
          <Route path="*" element={<Placeholder title="Page not found" />} />
        </Route>
      </Route>
    </Routes>
  );
}
