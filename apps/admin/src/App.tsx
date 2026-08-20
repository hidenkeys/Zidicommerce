import { FormEvent, useEffect, useMemo, useState } from "react";
import { Route, Routes } from "react-router-dom";
import { API_BASE_URL, apiGet, apiPatch, apiPost, apiPut, setStoredToken } from "./api/client";
import { Shell } from "./components/Shell";

type Row = Record<string, unknown>;
type ApiUser = { id: string; organization_id: string; email?: string; role: string };
type Organization = Row & { id: string; name?: string; onboarding_state?: string };
type Member = Row & { id: string; role?: string; status?: string; user?: Row };
type Store = Row & { id: string; name?: string };
type Bot = Row & { id: string; name: string; status: string; published_version_id?: string };
type BotVersion = Row & { id: string; bot_id: string; version_number: number; status: string; start_step_key: string };
type BotModule = Row & { id: string; module_key: string; name: string; parameters?: string };
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

function Dashboard() {
  const [me, setMe] = useState<ApiUser | null>(null);
  const [org, setOrg] = useState<Organization | null>(null);
  const [message, setMessage] = useState("");

  async function load() {
    setMessage("");
    try {
      const meResponse = await apiGet<ApiUser>("/auth/me");
      setMe(meResponse.data);
      if (meResponse.data.organization_id) {
        const orgResponse = await apiGet<Organization>("/organizations/current");
        setOrg(orgResponse.data);
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const state = parseState(org?.onboarding_state);
  const done = onboardingSteps.filter(([key]) => state[key]).length;

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Overview</h2>
        <p>Onboard a merchant, configure commerce operations, and keep organization access controlled from one admin surface.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="grid">
        <article className="summary-card">
          <span>API base</span>
          <p>{API_BASE_URL}</p>
        </article>
        <article className="summary-card">
          <span>Current role</span>
          <p>{me?.role ?? "Connect an API token"}</p>
        </article>
        <article className="summary-card">
          <span>Organization</span>
          <p>{org?.name ?? "No organization loaded"}</p>
        </article>
        <article className="summary-card">
          <span>Onboarding</span>
          <p>{done} of {onboardingSteps.length} steps marked complete.</p>
        </article>
      </div>
      <div className="table-wrap checklist">
        {onboardingSteps.map(([key, label]) => (
          <button key={key} className={state[key] ? "step complete" : "step"} type="button">
            <span>{state[key] ? "✓" : "○"}</span>
            {label}
          </button>
        ))}
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
        <p>Create the organization for a new merchant, then update business identity, contact, currency and timezone.</p>
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
      variants: [{ sku: product.sku, name: product.variant, price_minor: Number(product.price_minor || 0), currency: "USD", status: "active" }],
    });
    setProduct({ name: "", slug: "", sku: "", variant: "Regular", price_minor: "" });
    await load();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Catalogue</h2>
        <p>Create categories, products, variants and authoritative database prices.</p>
      </div>
      <div className="split">
        <form className="resource-form" onSubmit={createCategory}>
          <strong>Create category</strong>
          <input placeholder="Name" value={category.name} onChange={(event) => setCategory({ ...category, name: event.target.value })} />
          <input placeholder="Slug" value={category.slug} onChange={(event) => setCategory({ ...category, slug: event.target.value })} />
          <button type="submit">Create category</button>
        </form>
        <form className="resource-form" onSubmit={createProduct}>
          <strong>Create product</strong>
          <input placeholder="Product name" value={product.name} onChange={(event) => setProduct({ ...product, name: event.target.value })} />
          <input placeholder="Slug" value={product.slug} onChange={(event) => setProduct({ ...product, slug: event.target.value })} />
          <input placeholder="SKU" value={product.sku} onChange={(event) => setProduct({ ...product, sku: event.target.value })} />
          <input placeholder="Variant" value={product.variant} onChange={(event) => setProduct({ ...product, variant: event.target.value })} />
          <input placeholder="Price minor" type="number" value={product.price_minor} onChange={(event) => setProduct({ ...product, price_minor: event.target.value })} />
          <button type="submit">Create product</button>
        </form>
      </div>
      <ResourceTable rows={categories} title="Categories" message={message} />
      <ResourceTable rows={products} title="Products" />
    </section>
  );
}

function InventoryScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [form, setForm] = useState({ store_id: "", variant_id: "", on_hand: "", reorder_threshold: "0" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<Row[]>("/inventory");
      setRows(response.data);
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
        <p>Set store-level stock. Order creation locks and decrements inventory server-side.</p>
      </div>
      <form className="resource-form" onSubmit={submit}>
        <input placeholder="Store ID" value={form.store_id} onChange={(event) => setForm({ ...form, store_id: event.target.value })} />
        <input placeholder="Variant ID" value={form.variant_id} onChange={(event) => setForm({ ...form, variant_id: event.target.value })} />
        <input placeholder="On hand" type="number" value={form.on_hand} onChange={(event) => setForm({ ...form, on_hand: event.target.value })} />
        <input placeholder="Reorder at" type="number" value={form.reorder_threshold} onChange={(event) => setForm({ ...form, reorder_threshold: event.target.value })} />
        <button type="submit">Save inventory</button>
      </form>
      <ResourceTable rows={rows} message={message} />
    </section>
  );
}

function OrdersScreen() {
  const [rows, setRows] = useState<Row[]>([]);
  const [transition, setTransition] = useState({ order_id: "", status: "processing" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<Row[]>("/orders");
      setRows(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Request failed");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/orders/${transition.order_id}/transition`, { status: transition.status, idempotency_key: `admin-${Date.now()}` });
    await load();
  }

  return (
    <section className="content">
      <div className="section-heading">
        <h2>Orders</h2>
        <p>Review orders and perform controlled lifecycle transitions through backend APIs.</p>
      </div>
      <form className="resource-form" onSubmit={submit}>
        <input placeholder="Order ID" value={transition.order_id} onChange={(event) => setTransition({ ...transition, order_id: event.target.value })} />
        <select value={transition.status} onChange={(event) => setTransition({ ...transition, status: event.target.value })}>
          {["processing", "ready", "out_for_delivery", "completed", "cancelled"].map((status) => <option key={status} value={status}>{status}</option>)}
        </select>
        <button type="submit">Transition order</button>
      </form>
      <ResourceTable rows={rows} message={message} />
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
  const [message, setMessage] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setMessage("");
    setResult(null);
    try {
      const parsed = JSON.parse(body);
      const response = await apiPost<Row>("/merchant-imports/configuration", parsed);
      setResult(response.data);
      setMessage("Merchant configuration imported.");
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

function BotBuilderScreen() {
  const [bots, setBots] = useState<Bot[]>([]);
  const [versions, setVersions] = useState<BotVersion[]>([]);
  const [config, setConfig] = useState<BotConfig | null>(null);
  const [modules, setModules] = useState<BotModuleSpec[]>([]);
  const [actions, setActions] = useState<ActionSpec[]>([]);
  const [questionTypes, setQuestionTypes] = useState<QuestionTypeSpec[]>([]);
  const [selectedBotID, setSelectedBotID] = useState("");
  const [selectedVersionID, setSelectedVersionID] = useState("");
  const [botForm, setBotForm] = useState({ name: "", description: "" });
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

  async function loadBots(nextBotID = selectedBotID) {
    const response = await apiGet<Bot[]>("/bots");
    setBots(response.data);
    const resolvedBotID = nextBotID || response.data[0]?.id || "";
    setSelectedBotID(resolvedBotID);
    if (resolvedBotID) {
      await loadVersions(resolvedBotID);
    } else {
      setVersions([]);
      setConfig(null);
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
  const selectedQuestionType = questionTypes.find((item) => item.key === questionForm.type);

  return (
    <section className="content bot-builder">
      <div className="section-heading">
        <h2>Bots</h2>
        <p>Configure reusable merchant bot flows, validate them, and publish immutable snapshots for later runtime execution.</p>
      </div>
      {message ? <p className="error-text">{message}</p> : null}
      <div className="split">
        <form className="resource-form" onSubmit={createBot}>
          <strong>Create bot</strong>
          <input placeholder="Bot name" value={botForm.name} onChange={(event) => setBotForm({ ...botForm, name: event.target.value })} />
          <input placeholder="Description" value={botForm.description} onChange={(event) => setBotForm({ ...botForm, description: event.target.value })} />
          <button type="submit">Create bot</button>
        </form>
        <div className="table-wrap bot-toolbar">
          <label>
            Bot
            <select value={selectedBotID} onChange={(event) => { setSelectedBotID(event.target.value); void loadVersions(event.target.value, ""); }}>
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

      {config ? (
        <>
          <div className="grid">
            <article className="summary-card"><span>Version</span><p>v{config.version.version_number} · {config.version.status}</p></article>
            <article className="summary-card"><span>Start step</span><p>{config.version.start_step_key}</p></article>
            <article className="summary-card"><span>Modules</span><p>{config.modules.length}</p></article>
            <article className="summary-card"><span>Steps</span><p>{config.steps.length}</p></article>
          </div>
          {!editable ? <p className="muted">This version is immutable. Create a new draft from it to make changes.</p> : null}

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
  const columns = useMemo(() => Array.from(new Set(rows.flatMap((row) => Object.keys(row)))).slice(0, 8), [rows]);
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

function memberName(member: Member) {
  const user = member.user ?? {};
  return [user.first_name, user.last_name].filter(Boolean).join(" ") || String(user.email ?? member.id);
}

function formatCell(value: unknown) {
  if (value === null || value === undefined) return "";
  if (typeof value === "object") return JSON.stringify(value).slice(0, 120);
  return String(value);
}

export default function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<Dashboard />} />
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
        <Route path="configuration/channels" element={<BasicResourceScreen resource="channels" />} />
        <Route path="automation/bots" element={<BotBuilderScreen />} />
        <Route path="automation/versions" element={<BotBuilderScreen />} />
        <Route path="settings/import" element={<MerchantImportScreen />} />
        <Route path="settings" element={<BusinessScreen />} />
        <Route path="*" element={<Placeholder title="Planned module" />} />
      </Route>
    </Routes>
  );
}
