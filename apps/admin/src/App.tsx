import { FormEvent, useEffect, useMemo, useState } from "react";
import { Route, Routes } from "react-router-dom";
import { apiGet, apiPatch, apiPost, apiPut, setStoredToken } from "./api/client";
import { RequireAuth } from "./auth";
import { LoginPage } from "./components/LoginPage";
import { Shell } from "./components/Shell";

type Row = Record<string, unknown>;
type ApiUser = { id: string; organization_id: string; email?: string; role: string };
type Organization = Row & { id: string; name?: string; onboarding_state?: string };
type Member = Row & { id: string; role?: string; status?: string; user?: Row };
type Store = Row & { id: string; name?: string };
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
type Endpoint = {
  title: string;
  description: string;
  listPath?: string;
  createPath?: string;
  fields?: Field[];
};
type Field = {
  name: string;
  label: string;
  placeholder?: string;
};

const roles = ["merchant_admin", "store_manager", "store_staff", "support_agent", "viewer"];
const onboardingSteps = [
  ["business_profile", "Business profile"],
  ["first_store", "First store"],
  ["fulfilment", "Fulfilment"],
  ["team", "Team"],
  ["catalogue", "Catalogue"],
  ["inventory", "Inventory"],
  ["channels", "Channels"],
  ["payments", "Payments"],
];

function parseJSON<T>(value: string, fallback: T): T {
  try {
    return JSON.parse(value || "{}") as T;
  } catch {
    return fallback;
  }
}

const endpoints: Record<string, Endpoint> = {
  organizations: {
    title: "Organizations",
    description: "Platform-level tenant records. Merchant users should use Organization > Business.",
    listPath: "/organizations",
    createPath: "/organizations",
    fields: [
      { name: "name", label: "Name" },
      { name: "slug", label: "Slug" },
      { name: "currency", label: "Currency", placeholder: "USD" },
      { name: "timezone", label: "Timezone", placeholder: "UTC" },
      { name: "description", label: "Description" },
    ],
  },
  stores: {
    title: "Stores",
    description: "Manage locations, status, fulfilment modes, and store-level access boundaries.",
    listPath: "/stores",
    createPath: "/stores",
    fields: [
      { name: "name", label: "Name" },
      { name: "code", label: "Code" },
      { name: "address", label: "Address" },
      { name: "city", label: "City" },
      { name: "country", label: "Country" },
    ],
  },
  customers: {
    title: "Customers",
    description: "Organization-scoped customer records. Phone/email data stays tenant-isolated.",
    listPath: "/customers",
    createPath: "/customers",
    fields: [
      { name: "name", label: "Name" },
      { name: "phone", label: "Phone" },
      { name: "email", label: "Email" },
      { name: "default_address", label: "Default address" },
    ],
  },
  channels: {
    title: "Channels",
    description: "Channel identity records such as WhatsApp. Secrets are not exposed in API responses.",
    listPath: "/channels",
    createPath: "/channels",
    fields: [
      { name: "provider", label: "Provider", placeholder: "whatsapp" },
      { name: "display_name", label: "Display name" },
      { name: "phone_number_id", label: "Phone number ID" },
      { name: "display_number", label: "Display number" },
    ],
  },
};

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

function isToday(value: unknown) {
  const date = new Date(String(value ?? ""));
  if (Number.isNaN(date.getTime())) return false;
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
}

function nextOrderAction(status: string) {
  switch (status) {
    case "paid":
      return { status: "processing", label: "Start preparing" };
    case "processing":
      return { status: "ready", label: "Mark ready" };
    case "ready":
      return { status: "out_for_delivery", label: "Hand to rider" };
    case "out_for_delivery":
      return { status: "completed", label: "Mark delivered" };
    default:
      return null;
  }
}

function Dashboard() {
  const [org, setOrg] = useState<Organization | null>(null);
  const [orders, setOrders] = useState<Row[]>([]);
  const [conversations, setConversations] = useState<Row[]>([]);
  const [inventory, setInventory] = useState<Row[]>([]);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    setMessage("");
    try {
      const [orgResponse, orderResponse, conversationResponse, inventoryResponse, setupResponse, channelResponse] = await Promise.all([
        apiGet<Organization>("/organizations/current"),
        apiGet<Row[]>("/orders"),
        apiGet<Row[]>("/runtime/conversations"),
        apiGet<Row[]>("/inventory"),
        apiGet<SetupStatus>("/bot-setup/status"),
        apiGet<Channel[]>("/channels"),
      ]);
      setOrg(orgResponse.data);
      setOrders(orderResponse.data);
      setConversations(conversationResponse.data);
      setInventory(inventoryResponse.data);
      setSetupStatus(setupResponse.data);
      setChannels(channelResponse.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load overview");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const todayOrders = orders.filter((order) => isToday(order.created_at));
  const todayRevenue = todayOrders
    .filter((order) => !["cancelled", "awaiting_payment"].includes(String(order.status)))
    .reduce((sum, order) => sum + Number(order.total_minor ?? 0), 0);
  const awaitingPrep = orders.filter((order) => ["paid", "processing"].includes(String(order.status))).length;
  const awaitingFulfilment = orders.filter((order) => ["ready", "out_for_delivery"].includes(String(order.status))).length;
  const activeCustomers = new Set(orders.map((order) => String(order.customer_id ?? order.customer))).size;
  const openConversations = conversations.filter((conversation) => ["active", "handoff"].includes(String(conversation.status))).length;
  const waitingConversations = conversations.filter((conversation) => conversation.handoff_status === "open" || conversation.handoff_status === "assigned" || conversation.status === "handoff").length;
  const lowStock = inventory.filter((row) => Number(row.on_hand ?? 0) <= Number(row.reorder_threshold ?? 0)).length;
  const whatsapp = channels.find((channel) => channel.provider === "whatsapp");
  const currency = String(org?.currency ?? todayOrders[0]?.currency ?? "NGN");
  const attention: string[] = [];
  if (awaitingPrep > 0) attention.push(`${awaitingPrep} order${awaitingPrep === 1 ? "" : "s"} waiting to be prepared`);
  if (whatsapp && whatsapp.status !== "active") attention.push("WhatsApp connection needs attention");
  if (!whatsapp) attention.push("WhatsApp is not connected yet");
  if (waitingConversations > 0) attention.push(`${waitingConversations} customer conversation${waitingConversations === 1 ? "" : "s"} waiting for a response`);
  if (lowStock > 0) attention.push(`Inventory is low for ${lowStock} product${lowStock === 1 ? "" : "s"}`);
  if (setupStatus && !setupStatus.ready) attention.push("A few setup checks still need to be completed");

  return (
    <section className="content">
      <div className="section-heading">
        <h2>{org?.name ? `Good to see you, ${org.name}` : "Today at a glance"}</h2>
        <p>How your business is doing right now.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      {loading ? <div className="empty-state"><strong>Loading your day</strong><span>Fetching orders, conversations, and stock.</span></div> : null}
      <div className="grid overview-grid">
        <article className="summary-card"><span>Orders today</span><strong>{todayOrders.length}</strong></article>
        <article className="summary-card"><span>Revenue today</span><strong>{money(todayRevenue, currency)}</strong></article>
        <article className="summary-card"><span>Awaiting preparation</span><strong>{awaitingPrep}</strong></article>
        <article className="summary-card"><span>Awaiting fulfilment</span><strong>{awaitingFulfilment}</strong></article>
        <article className="summary-card"><span>Customers</span><strong>{activeCustomers}</strong></article>
        <article className="summary-card"><span>Bot conversations</span><strong>{openConversations}</strong></article>
      </div>
      <div className="split">
        <div className="table-wrap">
          <h3>Needs your attention</h3>
          {attention.length === 0 ? <p className="muted">You are all caught up.</p> : (
            <ul className="attention-list">
              {attention.map((item) => <li key={item}>{item}</li>)}
            </ul>
          )}
        </div>
        <div className="table-wrap">
          <h3>WhatsApp</h3>
          <p className="status-pill">{whatsapp?.status === "active" ? "Connected" : "Not connected"}</p>
          <p className="muted">{whatsapp?.display_number || "Connect a number in Settings → WhatsApp."}</p>
        </div>
      </div>
      <div className="table-wrap">
        <h3>Recent orders</h3>
        {orders.length === 0 ? <p className="muted">No orders yet. They will appear here as customers order on WhatsApp.</p> : (
          <table>
            <thead>
              <tr>
                <th>Order</th>
                <th>Customer</th>
                <th>Items</th>
                <th>Total</th>
                <th>Status</th>
                <th>Time</th>
              </tr>
            </thead>
            <tbody>
              {orders.slice(0, 8).map((order) => {
                const customer = (order.customer ?? {}) as Row;
                const items = Array.isArray(order.items) ? (order.items as Row[]) : [];
                return (
                  <tr key={String(order.id)}>
                    <td>{String(order.order_number ?? "—")}</td>
                    <td>{String(customer.name || customer.phone || "Customer")}</td>
                    <td>{items.length ? items.map((item) => `${item.product_name} ×${item.quantity}`).join(", ") : "—"}</td>
                    <td>{money(order.total_minor, String(order.currency ?? currency))}</td>
                    <td><span className={`badge status-${String(order.status)}`}>{humanStatus(order.status)}</span></td>
                    <td>{relativeTime(order.created_at)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

function BusinessScreen() {
  const [org, setOrg] = useState<Organization | null>(null);
  const [form, setForm] = useState<Record<string, string>>({});
  const [signup, setSignup] = useState({ email: "", password: "", first_name: "", last_name: "" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<Organization>("/organizations/current");
      setOrg(response.data);
      setForm(rowToForm(response.data, ["name", "slug", "description", "logo_url", "country", "currency", "timezone", "contact_name", "contact_email", "contact_phone"]));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "No organization loaded yet");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function register(event: FormEvent) {
    event.preventDefault();
    const response = await apiPost<{ access_token: string }>("/auth/register", signup);
    setStoredToken(response.data.access_token);
    setMessage("Account created. Complete the business profile to create the organization.");
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const payload = clean(form);
    if (org?.id) {
      const response = await apiPatch<Organization>(`/organizations/${org.id}`, payload);
      setOrg(response.data);
      setMessage("Business information saved.");
    } else {
      const response = await apiPost<{ organization: Organization; access_token: string }>("/onboarding/organization", payload);
      setOrg(response.data.organization);
      setStoredToken(response.data.access_token);
      setMessage("Organization created. Your new merchant admin token has been saved.");
    }
  }

  async function saveProgress(key: string) {
    const next = { ...parseState(org?.onboarding_state), [key]: true };
    const response = await apiPatch<Organization>("/onboarding/progress", { onboarding_state: JSON.stringify(next) });
    setOrg(response.data);
    setMessage("Onboarding progress saved.");
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Business</h2>
        <p>Your public name, contact details, currency, and timezone.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="split">
        <form className="resource-form" onSubmit={register}>
          <strong>Create admin account</strong>
          <input placeholder="First name" value={signup.first_name} onChange={(event) => setSignup({ ...signup, first_name: event.target.value })} />
          <input placeholder="Last name" value={signup.last_name} onChange={(event) => setSignup({ ...signup, last_name: event.target.value })} />
          <input placeholder="Email" value={signup.email} onChange={(event) => setSignup({ ...signup, email: event.target.value })} />
          <input placeholder="Password" type="password" value={signup.password} onChange={(event) => setSignup({ ...signup, password: event.target.value })} />
          <button type="submit">Register</button>
        </form>
        <div className="table-wrap checklist">
          {onboardingSteps.map(([key, label]) => (
            <button key={key} className={parseState(org?.onboarding_state)[key] ? "step complete" : "step"} type="button" onClick={() => saveProgress(key)}>
              <span>{parseState(org?.onboarding_state)[key] ? "✓" : "○"}</span>
              {label}
            </button>
          ))}
        </div>
      </div>
      <form className="resource-form" onSubmit={submit}>
        {[
          ["name", "Business name"],
          ["slug", "Slug"],
          ["description", "Description"],
          ["logo_url", "Logo URL"],
          ["country", "Country"],
          ["currency", "Currency"],
          ["timezone", "Timezone"],
          ["contact_name", "Contact name"],
          ["contact_email", "Contact email"],
          ["contact_phone", "Contact phone"],
        ].map(([name, label]) => (
          <label key={name}>
            {label}
            <input value={form[name] ?? ""} onChange={(event) => setForm((current) => ({ ...current, [name]: event.target.value }))} />
          </label>
        ))}
        <button type="submit">{org?.id ? "Save business" : "Create organization"}</button>
      </form>
    </section>
  );
}

function TeamScreen() {
  const [members, setMembers] = useState<Member[]>([]);
  const [stores, setStores] = useState<Store[]>([]);
  const [invite, setInvite] = useState({ email: "", first_name: "", last_name: "", role: "store_staff", store_ids: "" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const [memberResponse, storeResponse] = await Promise.all([apiGet<Member[]>("/organizations/current/members"), apiGet<Store[]>("/stores")]);
      setMembers(memberResponse.data);
      setStores(storeResponse.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/organizations/current/invitations", {
      email: invite.email,
      first_name: invite.first_name,
      last_name: invite.last_name,
      role: invite.role,
      store_ids: splitIDs(invite.store_ids),
    });
    setInvite({ email: "", first_name: "", last_name: "", role: "store_staff", store_ids: "" });
    setMessage("Invitation created. The token is sent through configured email.");
    await load();
  }

  async function updateMember(id: string, payload: Row) {
    await apiPatch<Row>(`/organizations/current/members/${id}`, payload);
    await load();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Team</h2>
        <p>Invite staff, assign roles, deactivate/reactivate members, and apply store-scoped access.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <form className="resource-form" onSubmit={submit}>
        <strong>Invite team member</strong>
        <input placeholder="First name" value={invite.first_name} onChange={(event) => setInvite({ ...invite, first_name: event.target.value })} />
        <input placeholder="Last name" value={invite.last_name} onChange={(event) => setInvite({ ...invite, last_name: event.target.value })} />
        <input placeholder="Email" value={invite.email} onChange={(event) => setInvite({ ...invite, email: event.target.value })} />
        <select value={invite.role} onChange={(event) => setInvite({ ...invite, role: event.target.value })}>
          {roles.map((role) => <option key={role} value={role}>{role}</option>)}
        </select>
        <input placeholder={`Store IDs, comma separated (${stores.length} stores available)`} value={invite.store_ids} onChange={(event) => setInvite({ ...invite, store_ids: event.target.value })} />
        <button type="submit">Invite</button>
      </form>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Role</th>
              <th>Status</th>
              <th>Created</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {members.map((member) => (
              <tr key={member.id}>
                <td>{memberName(member)}</td>
                <td>{String(member.user?.email ?? "")}</td>
                <td>{String(member.role ?? "")}</td>
                <td>{String(member.status ?? "")}</td>
                <td>{formatCell(member.created_at)}</td>
                <td className="actions">
                  <select defaultValue={String(member.role ?? "viewer")} onChange={(event) => updateMember(member.id, { role: event.target.value })}>
                    {roles.map((role) => <option key={role} value={role}>{role}</option>)}
                  </select>
                  {member.status === "disabled" ? (
                    <button type="button" onClick={() => updateMember(member.id, { status: "active" })}>Reactivate</button>
                  ) : (
                    <button type="button" onClick={() => window.confirm("Deactivate this member?") && updateMember(member.id, { status: "disabled" })}>Deactivate</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ResourceTable rows={stores} title="Store IDs for assignment" />
    </section>
  );
}

function StoreAccessScreen() {
  const [members, setMembers] = useState<Member[]>([]);
  const [stores, setStores] = useState<Store[]>([]);
  const [selectedMember, setSelectedMember] = useState("");
  const [storeIDs, setStoreIDs] = useState("");
  const [assignments, setAssignments] = useState<Row[]>([]);
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const [memberResponse, storeResponse] = await Promise.all([apiGet<Member[]>("/organizations/current/members"), apiGet<Store[]>("/stores")]);
      setMembers(memberResponse.data);
      setStores(storeResponse.data);
      setSelectedMember((current) => current || memberResponse.data[0]?.id || "");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function showAssignments(memberID = selectedMember) {
    if (!memberID) return;
    const response = await apiGet<Row[]>(`/organizations/current/members/${memberID}/stores`);
    setAssignments(response.data);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    await apiPut<Row[]>(`/organizations/current/members/${selectedMember}/stores`, { store_ids: splitIDs(storeIDs) });
    setMessage("Store access updated.");
    await showAssignments();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Stores & Access</h2>
        <p>Assign store managers and store staff to specific locations. Backend store scoping uses these assignments.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <form className="resource-form" onSubmit={submit}>
        <strong>Assign stores</strong>
        <select value={selectedMember} onChange={(event) => setSelectedMember(event.target.value)}>
          {members.map((member) => <option key={member.id} value={member.id}>{memberName(member)} · {String(member.role ?? "")}</option>)}
        </select>
        <input placeholder="Store IDs, comma separated" value={storeIDs} onChange={(event) => setStoreIDs(event.target.value)} />
        <button type="submit">Save access</button>
        <button type="button" onClick={() => showAssignments()}>View assignments</button>
      </form>
      <div className="split">
        <ResourceTable rows={stores} title="Stores" />
        <ResourceTable rows={assignments} title="Current assignments" />
      </div>
    </section>
  );
}

function BasicResourceScreen({ resource }: { resource: keyof typeof endpoints }) {
  const config = endpoints[resource];
  const [rows, setRows] = useState<Row[]>([]);
  const [form, setForm] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);

  async function load() {
    if (!config.listPath) return;
    setLoading(true);
    setMessage("");
    try {
      const response = await apiGet<Row[]>(config.listPath);
      setRows(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [resource]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!config.createPath) return;
    const payload = clean(form);
    try {
      await apiPost<Row>(config.createPath, resource === "stores" ? withDefaultFulfilment(payload) : payload);
      setForm({});
      await load();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>{config.title}</h2>
        <p>{config.description}</p>
      </div>
      <form className="resource-form" onSubmit={submit}>
        {config.fields?.map((field) => (
          <label key={field.name}>
            {field.label}
            <input value={form[field.name] ?? ""} placeholder={field.placeholder} onChange={(event) => setForm((current) => ({ ...current, [field.name]: event.target.value }))} />
          </label>
        ))}
        <button type="submit">Create</button>
      </form>
      <ResourceTable rows={rows} loading={loading} message={message} />
    </section>
  );
}

function CatalogueScreen() {
  const [categories, setCategories] = useState<Row[]>([]);
  const [products, setProducts] = useState<Row[]>([]);
  const [category, setCategory] = useState({ name: "", slug: "" });
  const [product, setProduct] = useState({ name: "", slug: "", sku: "", variant: "Regular", price_minor: "" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const [categoryResponse, productResponse] = await Promise.all([apiGet<Row[]>("/catalogue/categories"), apiGet<Row[]>("/catalogue/products")]);
      setCategories(categoryResponse.data);
      setProducts(productResponse.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function createCategory(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/catalogue/categories", category);
    setCategory({ name: "", slug: "" });
    await load();
  }

  async function createProduct(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/catalogue/products", {
      name: product.name,
      slug: product.slug,
      status: "active",
      variants: [{ sku: product.sku, name: product.variant, price_minor: Number(product.price_minor || 0), currency: "NGN", status: "active" }],
    });
    setProduct({ name: "", slug: "", sku: "", variant: "Regular", price_minor: "" });
    await load();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Catalogue</h2>
        <p>What customers can order. Keep names, prices, and availability easy to scan.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="split">
        <form className="resource-form" onSubmit={createCategory}>
          <strong>New category</strong>
          <input placeholder="Name" value={category.name} onChange={(event) => setCategory({ ...category, name: event.target.value })} />
          <input placeholder="Short code" value={category.slug} onChange={(event) => setCategory({ ...category, slug: event.target.value })} />
          <button type="submit">Add category</button>
        </form>
        <form className="resource-form" onSubmit={createProduct}>
          <strong>New product</strong>
          <input placeholder="Name" value={product.name} onChange={(event) => setProduct({ ...product, name: event.target.value })} />
          <input placeholder="Short code" value={product.slug} onChange={(event) => setProduct({ ...product, slug: event.target.value })} />
          <input placeholder="SKU" value={product.sku} onChange={(event) => setProduct({ ...product, sku: event.target.value })} />
          <input placeholder="Variant" value={product.variant} onChange={(event) => setProduct({ ...product, variant: event.target.value })} />
          <input placeholder="Price in kobo" value={product.price_minor} onChange={(event) => setProduct({ ...product, price_minor: event.target.value })} />
          <button type="submit">Add product</button>
        </form>
      </div>
      <div className="catalogue-grid">
        {products.map((item) => {
          const variants = Array.isArray(item.variants) ? (item.variants as Row[]) : [];
          const price = variants[0]?.price_minor;
          const categoryName = categories.find((entry) => entry.id === item.category_id)?.name;
          return (
            <article className="product-card" key={String(item.id)}>
              <div className="product-image">{item.image_url ? <img src={String(item.image_url)} alt="" /> : <span>No photo</span>}</div>
              <h3>{String(item.name)}</h3>
              <p className="muted">{String(categoryName || "Uncategorised")}</p>
              <p><strong>{price ? money(price, String(variants[0]?.currency ?? "NGN")) : "No price"}</strong></p>
              <p className="muted">{humanStatus(item.status)} · {variants.length || 1} variant{variants.length === 1 ? "" : "s"}</p>
            </article>
          );
        })}
      </div>
    </section>
  );
}

function InventoryScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [stores, setStores] = useState<Store[]>([]);
  const [products, setProducts] = useState<Row[]>([]);
  const [form, setForm] = useState({ store_id: "", variant_id: "", on_hand: "", reorder_threshold: "0" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const [inventoryResponse, storeResponse, productResponse] = await Promise.all([
        apiGet<Row[]>("/inventory"),
        apiGet<Store[]>("/stores"),
        apiGet<Row[]>("/catalogue/products"),
      ]);
      setRows(inventoryResponse.data);
      setStores(storeResponse.data);
      setProducts(productResponse.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/inventory", {
      store_id: form.store_id,
      variant_id: form.variant_id,
      on_hand: Number(form.on_hand),
      reorder_threshold: Number(form.reorder_threshold),
    });
    await load();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Inventory</h2>
        <p>See what is in stock at each store and update quantities when you restock.</p>
      </div>
      <form className="resource-form" onSubmit={submit}>
        <select value={form.store_id} onChange={(event) => setForm({ ...form, store_id: event.target.value })}>
          <option value="">Store</option>
          {stores.map((store) => <option key={store.id} value={store.id}>{store.name}</option>)}
        </select>
        <select value={form.variant_id} onChange={(event) => setForm({ ...form, variant_id: event.target.value })}>
          <option value="">Product</option>
          {products.flatMap((product) => (Array.isArray(product.variants) ? product.variants as Row[] : []).map((variant) => (
            <option key={String(variant.id)} value={String(variant.id)}>{String(product.name)} · {String(variant.name || "Regular")}</option>
          )))}
        </select>
        <input placeholder="Quantity on hand" type="number" value={form.on_hand} onChange={(event) => setForm({ ...form, on_hand: event.target.value })} />
        <input placeholder="Low-stock alert at" type="number" value={form.reorder_threshold} onChange={(event) => setForm({ ...form, reorder_threshold: event.target.value })} />
        <button type="submit">Update stock</button>
      </form>
      <ResourceTable rows={rows} message={message} />
    </section>
  );
}

function OrdersScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [detail, setDetail] = useState<Row | null>(null);
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<Row[]>("/orders");
      setRows(response.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load orders");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function openOrder(id: string) {
    setSelectedID(id);
    const response = await apiGet<Row>(`/orders/${id}`);
    setDetail(response.data);
  }

  async function transition(status: string) {
    if (!selectedID) return;
    await apiPost<Row>(`/orders/${selectedID}/transition`, { status, idempotency_key: `admin-${Date.now()}` });
    await load();
    await openOrder(selectedID);
  }

  const selected = detail ?? rows.find((row) => row.id === selectedID) ?? null;
  const action = selected ? nextOrderAction(String(selected.status)) : null;
  const customer = ((selected?.customer ?? {}) as Row);
  const items = Array.isArray(selected?.items) ? (selected.items as Row[]) : [];
  const store = ((selected?.store ?? {}) as Row);

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Orders</h2>
        <p>See what customers ordered and take the next step without leaving this page.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="split order-layout">
        <div className="table-wrap">
          {rows.length === 0 ? <p className="muted">No orders yet.</p> : (
            <table>
              <thead>
                <tr>
                  <th>Order</th>
                  <th>Customer</th>
                  <th>Total</th>
                  <th>Status</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((order) => {
                  const person = (order.customer ?? {}) as Row;
                  return (
                    <tr key={String(order.id)} className={selectedID === order.id ? "selected-row" : ""} onClick={() => void openOrder(String(order.id))}>
                      <td>{String(order.order_number)}</td>
                      <td>{String(person.name || person.phone || "Customer")}</td>
                      <td>{money(order.total_minor, String(order.currency ?? "NGN"))}</td>
                      <td><span className={`badge status-${String(order.status)}`}>{humanStatus(order.status)}</span></td>
                      <td>{relativeTime(order.created_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
        <div className="table-wrap order-detail">
          {!selected ? <div className="empty-state"><strong>Select an order</strong><span>Choose a row to see items, payment, and the next action.</span></div> : (
            <>
              <div className="section-heading">
                <h2>{String(selected.order_number)}</h2>
                <p>{String(customer.name || "Customer")} · {String(customer.phone || "")}</p>
              </div>
              <p><span className={`badge status-${String(selected.status)}`}>{humanStatus(selected.status)}</span> · {humanStatus(selected.fulfilment_type)}</p>
              <p className="muted">{String(store.name || "Store")} · {relativeTime(selected.created_at)}</p>
              <ul className="item-list">
                {items.map((item, index) => (
                  <li key={String(item.id ?? index)}>{String(item.product_name)} {item.variant_name && item.variant_name !== "Regular" ? `· ${item.variant_name}` : ""} ×{Number(item.quantity)} — {money(item.total_minor, String(selected.currency ?? "NGN"))}</li>
                ))}
              </ul>
              <p><strong>Total {money(selected.total_minor, String(selected.currency ?? "NGN"))}</strong></p>
              <div className="timeline">
                {["Order placed", "Payment received", "Preparing", "Ready", "Out for delivery", "Delivered"].map((label) => (
                  <span key={label}>{label}</span>
                ))}
              </div>
              <div className="row-actions">
                {action ? <button type="button" className="primary-action" onClick={() => void transition(action.status)}>{action.label}</button> : null}
                {selected.status !== "cancelled" && selected.status !== "completed" ? <button type="button" onClick={() => void transition("cancelled")}>Cancel order</button> : null}
              </div>
            </>
          )}
        </div>
      </div>
    </section>
  );
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
      <div className="section-heading">
        <h2>Payments</h2>
        <p>Initialize and verify provider payments. Verification is authoritative and retry-safe.</p>
      </div>
      <div className="split">
        <form className="resource-form" onSubmit={initialize}>
          <strong>Initialize payment</strong>
          <input placeholder="Order ID" value={init.order_id} onChange={(event) => setInit({ ...init, order_id: event.target.value })} />
          <input placeholder="Email" value={init.email} onChange={(event) => setInit({ ...init, email: event.target.value })} />
          <input placeholder="Provider" value={init.provider} onChange={(event) => setInit({ ...init, provider: event.target.value })} />
          <button type="submit">Initialize</button>
        </form>
        <form className="resource-form" onSubmit={verifyPayment}>
          <strong>Verify payment</strong>
          <input placeholder="Reference" value={verify.reference} onChange={(event) => setVerify({ reference: event.target.value })} />
          <button type="submit">Verify</button>
        </form>
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
      <div className="section-heading">
        <h2>Fulfilment</h2>
        <p>Lookup fulfilment records by order and update fulfilment details through the API.</p>
      </div>
      <form className="resource-form" onSubmit={load}>
        <input placeholder="Order ID" value={orderId} onChange={(event) => setOrderId(event.target.value)} />
        <button type="submit">Find fulfilment</button>
      </form>
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
      <div className="section-heading">
        <h2>Merchant Import</h2>
        <p>Load stores, categories, products, images, inventory, fulfilment modes, and channels from one validated JSON document.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <form className="resource-form wide" onSubmit={submit}>
        <textarea rows={18} value={body} onChange={(event) => setBody(event.target.value)} />
        <button type="submit">Import configuration</button>
      </form>
      {result ? <ResourceTable rows={[result]} title="Import result" /> : null}
      <ResourceTable rows={jobs} title="Recent import jobs" />
    </section>
  );
}

function SupportHandoffsScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [tickets, setTickets] = useState<Row[]>([]);
  const [status, setStatus] = useState("open");
  const [resolveID, setResolveID] = useState("");
  const [claimID, setClaimID] = useState("");
  const [claimNote, setClaimNote] = useState("");
  const [noteID, setNoteID] = useState("");
  const [note, setNote] = useState("");
  const [resolutionNote, setResolutionNote] = useState("");
  const [message, setMessage] = useState("");

  async function load(nextStatus = status) {
    try {
      const query = nextStatus ? `?status=${encodeURIComponent(nextStatus)}` : "";
      const [handoffResponse, ticketResponse] = await Promise.all([
        apiGet<Row[]>(`/runtime/support-handoffs${query}`),
        apiGet<Row[]>(`/runtime/support-tickets${query}`),
      ]);
      setRows(handoffResponse.data);
      setTickets(ticketResponse.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load(status);
  }, [status]);

  async function resolve(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/runtime/support-handoffs/${resolveID}/resolve`, { resolution_note: resolutionNote });
    setResolveID("");
    setResolutionNote("");
    await load();
  }

  async function claim(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/runtime/support-handoffs/${claimID}/claim`, { note: claimNote });
    setClaimID("");
    setClaimNote("");
    await load();
  }

  async function addNote(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/runtime/support-handoffs/${noteID}/notes`, { note, internal: true });
    setNoteID("");
    setNote("");
    await load();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Support Handoffs</h2>
        <p>Review conversations paused for human support, claim ownership, add internal notes, and resolve once handled.</p>
      </div>
      <div className="split">
        <form className="resource-form" onSubmit={(event) => { event.preventDefault(); void load(); }}>
          <strong>Filter</strong>
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="open">open</option>
            <option value="assigned">assigned</option>
            <option value="resolved">resolved</option>
            <option value="cancelled">cancelled</option>
            <option value="">all</option>
          </select>
          <button type="submit">Refresh</button>
        </form>
        <form className="resource-form" onSubmit={claim}>
          <strong>Claim handoff</strong>
          <input placeholder="Handoff ID" value={claimID} onChange={(event) => setClaimID(event.target.value)} />
          <input placeholder="Internal note" value={claimNote} onChange={(event) => setClaimNote(event.target.value)} />
          <button type="submit">Claim</button>
        </form>
      </div>
      <div className="split">
        <form className="resource-form" onSubmit={addNote}>
          <strong>Add note</strong>
          <input placeholder="Handoff ID" value={noteID} onChange={(event) => setNoteID(event.target.value)} />
          <input placeholder="Internal note" value={note} onChange={(event) => setNote(event.target.value)} />
          <button type="submit">Add note</button>
        </form>
        <form className="resource-form" onSubmit={resolve}>
          <strong>Resolve handoff</strong>
          <input placeholder="Handoff ID" value={resolveID} onChange={(event) => setResolveID(event.target.value)} />
          <input placeholder="Resolution note" value={resolutionNote} onChange={(event) => setResolutionNote(event.target.value)} />
          <button type="submit">Resolve</button>
        </form>
      </div>
      <ResourceTable rows={rows} title="Handoffs" message={message} />
      <ResourceTable rows={tickets} title="Support tickets" />
    </section>
  );
}

function ConversationsScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [messages, setMessages] = useState<Row[]>([]);
  const [handoffs, setHandoffs] = useState<Row[]>([]);
  const [reply, setReply] = useState("");
  const [note, setNote] = useState("");
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const [conversationResponse, handoffResponse] = await Promise.all([
        apiGet<Row[]>("/runtime/conversations"),
        apiGet<Row[]>("/runtime/support-handoffs"),
      ]);
      setRows(conversationResponse.data);
      setHandoffs(handoffResponse.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load conversations");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function openConversation(id: string) {
    setSelectedID(id);
    const response = await apiGet<Row[]>(`/runtime/conversations/${id}/messages`);
    setMessages(response.data);
  }

  function inboxStatus(row: Row) {
    if (row.status === "completed" || row.status === "cancelled") return "Resolved";
    if (row.status === "handoff" || row.handoff_status === "open" || row.handoff_status === "assigned") return "Waiting";
    return "Open";
  }

  function currentHandoff() {
    return handoffs.find((handoff) => String(handoff.session_id) === selectedID);
  }

  async function sendReply(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/runtime/conversations/${selectedID}/reply`, { text: reply });
    setReply("");
    await openConversation(selectedID);
    await load();
  }

  async function assignHandoff() {
    const handoff = currentHandoff();
    if (!handoff) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/claim`, { note });
    setNote("");
    await load();
  }

  async function addNote() {
    const handoff = currentHandoff();
    if (!handoff || !note.trim()) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/notes`, { note, internal: true });
    setNote("");
  }

  async function resolveHandoff() {
    const handoff = currentHandoff();
    if (!handoff) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/resolve`, { resolution_note: note });
    setNote("");
    await load();
  }

  async function reopenHandoff() {
    const handoff = currentHandoff();
    if (!handoff) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/reopen`, {});
    await load();
  }

  const selected = rows.find((row) => row.id === selectedID);

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Conversations</h2>
        <p>Your support inbox for customers who need a person.</p>
        <button type="button" onClick={() => void load()}>Refresh</button>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="split order-layout">
        <div className="table-wrap">
          {rows.length === 0 ? <p className="muted">No conversations yet.</p> : (
            <table>
              <thead>
                <tr>
                  <th>Customer</th>
                  <th>Last message</th>
                  <th>Status</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={String(row.id)} className={selectedID === row.id ? "selected-row" : ""} onClick={() => void openConversation(String(row.id))}>
                    <td>{String(row.customer_name || row.customer_phone || "Customer")}</td>
                    <td>{String(row.last_message || "—").slice(0, 80)}</td>
                    <td><span className="badge">{inboxStatus(row)}</span></td>
                    <td>{relativeTime(row.updated_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
        <div className="table-wrap conversation-thread">
          {!selected ? <div className="empty-state"><strong>Select a conversation</strong><span>Read the chat, reply, or pass it to a teammate.</span></div> : (
            <>
              <h3>{String(selected.customer_name || selected.customer_phone || "Customer")}</h3>
              <p className="muted">{inboxStatus(selected)}{currentHandoff()?.assigned_user_id ? " · Assigned" : ""}</p>
              <div className="thread">
                {messages.map((item) => (
                  <div key={String(item.id)} className={item.direction === "inbound" ? "bubble customer" : "bubble agent"}>
                    <span>{item.direction === "inbound" ? "Customer" : "You"}</span>
                    <p>{String(item.body)}</p>
                  </div>
                ))}
              </div>
              <form className="resource-form" onSubmit={sendReply}>
                <textarea placeholder="Write a reply" value={reply} onChange={(event) => setReply(event.target.value)} />
                <button type="submit">Reply</button>
              </form>
              <div className="row-actions">
                <input placeholder="Internal note" value={note} onChange={(event) => setNote(event.target.value)} />
                <button type="button" onClick={() => void assignHandoff()}>Assign to me</button>
                <button type="button" onClick={() => void addNote()}>Save note</button>
                <button type="button" onClick={() => void resolveHandoff()}>Resolve</button>
                <button type="button" onClick={() => void reopenHandoff()}>Reopen</button>
              </div>
            </>
          )}
        </div>
      </div>
    </section>
  );
}

function ReadinessScreen() {
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<SetupStatus>("/bot-setup/status");
      setSetupStatus(response.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Production Readiness</h2>
        <p>Check whether the merchant has the operational configuration required to go live.</p>
        <button type="button" onClick={load}>Refresh</button>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="grid">
        <article className={setupStatus?.ready ? "summary-card success" : "summary-card warning"}>
          <span>Status</span>
          <p>{setupStatus?.ready ? "READY" : "NOT READY"}</p>
        </article>
        <article className="summary-card">
          <span>Checks</span>
          <p>{setupStatus ? `${setupStatus.complete_count} of ${setupStatus.total_count}` : "Loading"}</p>
        </article>
      </div>
      <div className="table-wrap checklist-panel">
        <div className="checklist">
          {(setupStatus?.items ?? []).map((item) => (
            <button key={item.key} className={item.complete ? "step complete" : "step"} type="button" title={item.description}>
              <span>{item.complete ? "✓" : "○"}</span>
              {item.label}
            </button>
          ))}
        </div>
      </div>
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
    <section className="content">
      <div className="section-heading">
        <h2>Audit logs</h2>
        <p>Administrative events for organization, member, invitation and store-access changes.</p>
      </div>
      <ResourceTable rows={rows} message={message} />
    </section>
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
    const response = await apiPost<Bot>("/bots/self-service", {
      ...botForm,
      welcome_message: "Welcome. I can help you place an order, track an order, answer questions, or contact support.",
      require_payment: true,
      allow_pickup: true,
      allow_customer_rider: true,
      allow_merchant_rider: false,
    });
    setBotForm({ name: "", description: "" });
    setMessage("Self-service commerce bot created with starter modules.");
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
      <div className="section-heading">
        <h2>{variant === "simple" ? "My Bot" : "Bot Builder"}</h2>
        <p>{variant === "simple" ? "Control what customers see on WhatsApp. You do not need to know how the bot is built." : "Advanced configuration for conversation steps, actions, and published snapshots."}</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="split">
        <form className="resource-form" onSubmit={createSelfServiceBot}>
          <strong>Create self-service bot</strong>
          <input placeholder="Bot name" value={botForm.name} onChange={(event) => setBotForm({ ...botForm, name: event.target.value })} />
          <input placeholder="Description" value={botForm.description} onChange={(event) => setBotForm({ ...botForm, description: event.target.value })} />
          <button type="submit">Create guided bot</button>
          <button type="button" onClick={createEmptyBot}>Create empty bot</button>
        </form>
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
                <input placeholder="Customer question" value={faqForm.question} onChange={(event) => setFaqForm({ ...faqForm, question: event.target.value })} />
                <textarea placeholder="Answer" value={faqForm.answer} onChange={(event) => setFaqForm({ ...faqForm, answer: event.target.value })} />
                <input placeholder="Keywords, comma separated" value={faqForm.keywords} onChange={(event) => setFaqForm({ ...faqForm, keywords: event.target.value })} />
                <button type="submit">Add FAQ</button>
              </form>
              <div className="table-wrap match-panel">
                <h3>Test FAQ match</h3>
                <input placeholder="Ask a sample question" value={faqQuery} onChange={(event) => setFaqQuery(event.target.value)} />
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
                <input placeholder="Display name" value={channelForm.display_name} onChange={(event) => setChannelForm({ ...channelForm, display_name: event.target.value })} />
                <input placeholder="Phone number ID" value={channelForm.phone_number_id} onChange={(event) => setChannelForm({ ...channelForm, phone_number_id: event.target.value })} />
                <input placeholder="Display number" value={channelForm.display_number} onChange={(event) => setChannelForm({ ...channelForm, display_number: event.target.value })} />
                <textarea placeholder='Public config JSON, e.g. {"webhook_path":"/runtime/webhooks/whatsapp"}' value={channelForm.config} onChange={(event) => setChannelForm({ ...channelForm, config: event.target.value })} />
                <button type="submit">Save channel</button>
              </form>
              <form className="resource-form" onSubmit={savePayment}>
                <strong>Payment provider</strong>
                <input placeholder="Provider" value={paymentForm.provider} onChange={(event) => setPaymentForm({ ...paymentForm, provider: event.target.value })} />
                <input placeholder="Display name" value={paymentForm.display_name} onChange={(event) => setPaymentForm({ ...paymentForm, display_name: event.target.value })} />
                <input placeholder="Paystack public key" value={paymentForm.public_key} onChange={(event) => setPaymentForm({ ...paymentForm, public_key: event.target.value })} />
                <input type="password" placeholder="Paystack secret key" value={paymentForm.secret_key} onChange={(event) => setPaymentForm({ ...paymentForm, secret_key: event.target.value })} />
                <input placeholder="Secret source" value={paymentForm.secret_source} onChange={(event) => setPaymentForm({ ...paymentForm, secret_source: event.target.value })} />
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
                <input placeholder="Send a customer message" value={testInput} onChange={(event) => setTestInput(event.target.value)} />
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
            <tr>{columns.map((column) => <th key={column}>{column}</th>)}</tr>
          </thead>
          <tbody>
            {rows.map((row, index) => (
              <tr key={String(row.id ?? index)}>
                {columns.map((column) => <td key={column}>{formatCell(row[column])}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}

function Placeholder({ title, body = "This section is reserved for a later phase." }: { title: string; body?: string }) {
  return (
    <section className="content">
      <div className="section-heading">
        <h2>{title}</h2>
        <p>{body}</p>
      </div>
      <div className="empty-state">
        <strong>Coming soon</strong>
        <span>No bot builder or runtime has been implemented in Phase 3.</span>
      </div>
    </section>
  );
}

function withDefaultFulfilment(payload: Row) {
  return {
    ...payload,
    fulfilment_modes: [
      { mode: "pickup", enabled: true },
      { mode: "customer_rider", enabled: true },
      { mode: "merchant_rider", enabled: true },
    ],
  };
}

function rowToForm(row: Row, fields: string[]) {
  return Object.fromEntries(fields.map((field) => [field, String(row[field] ?? "")]));
}

function clean(form: Record<string, string>) {
  return Object.fromEntries(Object.entries(form).filter(([, value]) => value.trim() !== ""));
}

function splitIDs(value: string) {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function csvToJSON(value: string) {
  return JSON.stringify(value.split(",").map((item) => item.trim()).filter(Boolean));
}

function pairToJSON(name: string, variable: string) {
  if (!name.trim() || !variable.trim()) return "{}";
  return JSON.stringify({ [name.trim()]: variable.trim() });
}

function parseState(raw?: string) {
  if (!raw) return {} as Record<string, boolean>;
  try {
    return JSON.parse(raw) as Record<string, boolean>;
  } catch {
    return {} as Record<string, boolean>;
  }
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

function memberName(member: Member) {
  const user = member.user ?? {};
  return [user.first_name, user.last_name].filter(Boolean).join(" ") || String(user.email ?? member.id);
}

function formatCell(value: unknown) {
  if (value === null || value === undefined) return "";
  if (isUUID(value)) return "";
  if (typeof value === "object") return JSON.stringify(value).slice(0, 80);
  return String(value);
}

function whatsappConnected(channels: Channel[]) {
  return channels.some((channel) => channel.provider === "whatsapp" && channel.status === "active");
}

function WhatsAppScreen() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const [channelResponse, setupResponse] = await Promise.all([apiGet<Channel[]>("/channels"), apiGet<SetupStatus>("/bot-setup/status")]);
      setChannels(channelResponse.data);
      setSetupStatus(setupResponse.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load WhatsApp status");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const channel = channels.find((item) => item.provider === "whatsapp");
  const whatsappReady = setupStatus?.items.find((item) => item.key === "whatsapp");

  return (
    <section className="content">
      <div className="section-heading">
        <h2>WhatsApp</h2>
        <p>Your customer number and whether the bot is able to receive messages.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="grid">
        <article className="summary-card"><span>Number</span><p>{channel?.display_number || "Not connected"}</p></article>
        <article className="summary-card"><span>Connection</span><p>{channel?.status === "active" ? "Connected" : "Not connected"}</p></article>
        <article className="summary-card"><span>Bot</span><p>{whatsappReady?.complete ? "Ready" : "Needs attention"}</p></article>
      </div>
      <p className="muted">Secrets stay hidden. If something looks wrong, check Advanced → Readiness.</p>
    </section>
  );
}

function KnowledgeScreen() {
  const [faqs, setFaqs] = useState<BotFAQ[]>([]);
  const [form, setForm] = useState({ question: "", answer: "", keywords: "" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<BotFAQ[]>("/bot-faqs");
      setFaqs(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load answers");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    await apiPost<BotFAQ>("/bot-faqs", form);
    setForm({ question: "", answer: "", keywords: "" });
    await load();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Knowledge</h2>
        <p>Answers the bot can give when customers ask common questions.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <form className="resource-form wide" onSubmit={submit}>
        <input placeholder="Question" value={form.question} onChange={(event) => setForm({ ...form, question: event.target.value })} />
        <input placeholder="Answer" value={form.answer} onChange={(event) => setForm({ ...form, answer: event.target.value })} />
        <input placeholder="Keywords" value={form.keywords} onChange={(event) => setForm({ ...form, keywords: event.target.value })} />
        <button type="submit">Add answer</button>
      </form>
      <div className="table-wrap">
        {faqs.length === 0 ? <p className="muted">No answers yet.</p> : (
          <table>
            <thead><tr><th>Question</th><th>Answer</th></tr></thead>
            <tbody>
              {faqs.map((faq) => (
                <tr key={faq.id}><td>{faq.question}</td><td>{faq.answer}</td></tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

function PaymentsSettingsScreen() {
  const [rows, setRows] = useState<PaymentConfiguration[]>([]);
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<PaymentConfiguration[]>("/payment-configurations");
      setRows(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load payments");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Payments</h2>
        <p>How customers pay. Secrets are never shown here.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="grid">
        {rows.map((row) => (
          <article className="summary-card" key={row.id}>
            <span>{row.display_name || row.provider}</span>
            <p>{row.enabled ? "Connected" : "Off"} · {humanStatus(row.status)}</p>
            <p className="muted">{String(parseJSON(String(row.public_config ?? "{}"), { mode: "test" }).mode === "live" ? "Live mode" : "Test mode")}</p>
          </article>
        ))}
      </div>
    </section>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<LoginPage mode="register" />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route index element={<Dashboard />} />
          <Route path="sell/orders" element={<OrdersScreen />} />
          <Route path="sell/catalogue" element={<CatalogueScreen />} />
          <Route path="sell/inventory" element={<InventoryScreen />} />
          <Route path="sell/customers" element={<BasicResourceScreen resource="customers" />} />
          <Route path="automation/bot" element={<BotBuilderScreen variant="simple" />} />
          <Route path="automation/conversations" element={<ConversationsScreen />} />
          <Route path="automation/knowledge" element={<KnowledgeScreen />} />
          <Route path="business/stores" element={<BasicResourceScreen resource="stores" />} />
          <Route path="business/team" element={<TeamScreen />} />
          <Route path="business/payments" element={<PaymentsSettingsScreen />} />
          <Route path="business/delivery" element={<FulfilmentScreen />} />
          <Route path="settings/business" element={<BusinessScreen />} />
          <Route path="settings/whatsapp" element={<WhatsAppScreen />} />
          <Route path="settings/integrations" element={<PaymentsSettingsScreen />} />
          <Route path="advanced/bot-builder" element={<BotBuilderScreen variant="builder" />} />
          <Route path="advanced/readiness" element={<ReadinessScreen />} />
          <Route path="advanced/import" element={<MerchantImportScreen />} />
          <Route path="advanced/audit-logs" element={<AuditLogScreen />} />
          <Route path="advanced/access" element={<StoreAccessScreen />} />
          <Route path="platform/organizations" element={<BasicResourceScreen resource="organizations" />} />
          <Route path="commerce/stores" element={<BasicResourceScreen resource="stores" />} />
          <Route path="commerce/catalogue" element={<CatalogueScreen />} />
          <Route path="commerce/inventory" element={<InventoryScreen />} />
          <Route path="commerce/orders" element={<OrdersScreen />} />
          <Route path="commerce/customers" element={<BasicResourceScreen resource="customers" />} />
          <Route path="organization/business" element={<BusinessScreen />} />
          <Route path="organization/team" element={<TeamScreen />} />
          <Route path="organization/access" element={<StoreAccessScreen />} />
          <Route path="organization/audit-logs" element={<AuditLogScreen />} />
          <Route path="configuration/payments" element={<PaymentsScreen />} />
          <Route path="configuration/fulfilment" element={<FulfilmentScreen />} />
          <Route path="configuration/channels" element={<WhatsAppScreen />} />
          <Route path="automation/bots" element={<BotBuilderScreen variant="simple" />} />
          <Route path="automation/versions" element={<BotBuilderScreen variant="builder" />} />
          <Route path="automation/support-handoffs" element={<SupportHandoffsScreen />} />
          <Route path="settings/readiness" element={<ReadinessScreen />} />
          <Route path="settings/import" element={<MerchantImportScreen />} />
          <Route path="settings" element={<BusinessScreen />} />
          <Route path="*" element={<Placeholder title="This page is not available yet" />} />
        </Route>
      </Route>
    </Routes>
  );
}
