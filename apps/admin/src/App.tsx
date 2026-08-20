import { FormEvent, useEffect, useMemo, useState } from "react";
import { Route, Routes } from "react-router-dom";
import { API_BASE_URL, apiGet, apiPatch, apiPost } from "./api/client";
import { Shell } from "./components/Shell";

type Row = Record<string, unknown>;
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
  type?: "text" | "number" | "select";
  options?: string[];
};

const endpoints: Record<string, Endpoint> = {
  organizations: {
    title: "Organizations",
    description: "Create and manage tenant records. Platform admins can see all organizations.",
    listPath: "/organizations",
    createPath: "/organizations",
    fields: [
      { name: "name", label: "Name" },
      { name: "slug", label: "Slug" },
      { name: "currency", label: "Currency", placeholder: "NGN" },
      { name: "timezone", label: "Timezone", placeholder: "Africa/Lagos" },
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

const dashboardCards = [
  ["Stores", "Locations, hours and fulfilment options."],
  ["Catalogue", "Categories, products, variants, prices and images."],
  ["Inventory", "Store-level stock and low-stock visibility."],
  ["Orders", "Controlled order lifecycle and fulfilment handoff."],
  ["Payments", "Provider abstraction with idempotent initialization."],
  ["Channels", "Communication channel configuration foundation."],
];

function Dashboard() {
  return (
    <section className="content">
      <div className="section-heading">
        <h2>Commerce operations</h2>
        <p>Phase 2 turns the foundation into usable commerce primitives that a future bot runtime can orchestrate.</p>
      </div>
      <div className="grid">
        {dashboardCards.map(([title, body]) => (
          <article className="summary-card" key={title}>
            <span>{title}</span>
            <p>{body}</p>
          </article>
        ))}
      </div>
      <div className="api-note">
        <strong>API base</strong>
        <code>{API_BASE_URL}</code>
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
    const payload = Object.fromEntries(Object.entries(form).filter(([, value]) => value !== ""));
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
            <input
              value={form[field.name] ?? ""}
              placeholder={field.placeholder}
              onChange={(event) => setForm((current) => ({ ...current, [field.name]: event.target.value }))}
            />
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
      const [categoryResponse, productResponse] = await Promise.all([
        apiGet<Row[]>("/catalogue/categories"),
        apiGet<Row[]>("/catalogue/products"),
      ]);
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
      variants: [
        {
          sku: product.sku,
          name: product.variant,
          price_minor: Number(product.price_minor || 0),
          currency: "NGN",
          status: "active",
        },
      ],
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
          {["processing", "ready", "out_for_delivery", "completed", "cancelled"].map((status) => (
            <option key={status} value={status}>{status}</option>
          ))}
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
                {columns.map((column) => (
                  <td key={column}>{formatCell(row[column])}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}

function Placeholder({ title }: { title: string }) {
  return (
    <section className="content">
      <div className="section-heading">
        <h2>{title}</h2>
        <p>This navigation area is reserved. The Bot Builder and runtime remain out of scope for Phase 2.</p>
      </div>
      <div className="empty-state">
        <strong>Boundary established</strong>
        <span>No bot configuration workflow has been implemented here.</span>
      </div>
    </section>
  );
}

function withDefaultFulfilment(payload: Record<string, string>) {
  return {
    ...payload,
    fulfilment_modes: [
      { mode: "pickup", enabled: true },
      { mode: "customer_rider", enabled: true },
      { mode: "merchant_rider", enabled: true },
    ],
  };
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
        <Route path="organizations" element={<BasicResourceScreen resource="organizations" />} />
        <Route path="commerce/stores" element={<BasicResourceScreen resource="stores" />} />
        <Route path="commerce/catalogue" element={<CatalogueScreen />} />
        <Route path="commerce/inventory" element={<InventoryScreen />} />
        <Route path="commerce/orders" element={<OrdersScreen />} />
        <Route path="commerce/customers" element={<BasicResourceScreen resource="customers" />} />
        <Route path="payments" element={<PaymentsScreen />} />
        <Route path="fulfilment" element={<FulfilmentScreen />} />
        <Route path="channels" element={<BasicResourceScreen resource="channels" />} />
        <Route path="team" element={<Placeholder title="Team" />} />
        <Route path="settings" element={<Placeholder title="Settings" />} />
        <Route path="*" element={<Placeholder title="Planned module" />} />
      </Route>
    </Routes>
  );
}
