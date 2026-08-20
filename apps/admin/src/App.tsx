import { FormEvent, useEffect, useMemo, useState } from "react";
import { Route, Routes } from "react-router-dom";
import { API_BASE_URL, apiGet, apiPatch, apiPost, apiPut, setStoredToken } from "./api/client";
import { Shell } from "./components/Shell";

type Row = Record<string, unknown>;
type ApiUser = { id: string; organization_id: string; email?: string; role: string };
type Organization = Row & { id: string; name?: string; onboarding_state?: string };
type Member = Row & { id: string; role?: string; status?: string; user?: Row };
type Store = Row & { id: string; name?: string };
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
        <Route path="automation/bots" element={<Placeholder title="Bots" body="Automation is intentionally disabled until the organization and access model is stable." />} />
        <Route path="settings" element={<BusinessScreen />} />
        <Route path="*" element={<Placeholder title="Planned module" />} />
      </Route>
    </Routes>
  );
}
