import { FormEvent, ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { Link, Navigate, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import { api, clearToken, getToken, setToken } from "./api";

type User = { id: string; role: string; email?: string; organization_id: string };

function money(minor?: unknown) {
  const value = Number(minor ?? 0) / 100;
  return `₦${Math.round(value).toLocaleString()}`;
}

function statusLabel(value?: unknown) {
  return String(value ?? "").replaceAll("_", " ");
}

function Login({ onAuthenticated }: { onAuthenticated: (user: User) => void }) {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      const result = await api.login(email, password) as { access_token?: string };
      const token = result.access_token;
      if (!token) throw new Error("No session token returned");
      setToken(token);
      const me = await api.me();
      onAuthenticated(me);
      navigate(me.role === "service_provider" ? "/provider" : "/owner");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not sign in");
    }
  }
  return (
    <div className="login">
      <form onSubmit={onSubmit}>
        <p className="eyebrow">Zidi Field Service</p>
        <h1>Welcome back</h1>
        <p>Owner dashboard or provider portal — same sign-in.</p>
        <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="Email" />
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Password" />
        {error ? <p className="error">{error}</p> : null}
        <button className="primary" type="submit">Continue</button>
      </form>
    </div>
  );
}

function useSession() {
  const [user, setUser] = useState<User | null>(null);
  const [ready, setReady] = useState(false);
  useEffect(() => {
    if (!getToken()) {
      setReady(true);
      return;
    }
    api.me().then(setUser).catch(() => clearToken()).finally(() => setReady(true));
  }, []);
  return { user, ready, setUser };
}

function Shell({ user, links, children }: { user: User; links: { to: string; label: string }[]; children: ReactNode }) {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <div className="shell">
      <aside className="nav">
        <strong>{user.role === "service_provider" ? "Provider" : "Business"}</strong>
        <span>{user.email || "Signed in"}</span>
        {links.map((link) => (
          <Link key={link.to} className={location.pathname === link.to || (link.to !== "/owner" && link.to !== "/provider" && location.pathname.startsWith(link.to)) ? "active" : ""} to={link.to}>{link.label}</Link>
        ))}
        <button className="ghost" onClick={() => { clearToken(); navigate("/login"); }}>Sign out</button>
      </aside>
      <main className="main">{children}</main>
    </div>
  );
}

function Overview() {
  const [data, setData] = useState<Record<string, number>>({});
  useEffect(() => { api.overview().then(setData).catch(() => undefined); }, []);
  const cards = [
    ["Today's requests", data.todays_requests],
    ["Active jobs", data.active_jobs],
    ["Available handymen", data.available_providers],
    ["Waiting on providers", data.pending_provider_requests],
    ["Completed", data.completed_jobs],
    ["Outstanding quotes", data.outstanding_quotes],
    ["Booking fees", money(data.booking_fees_minor)],
    ["Revenue", money(data.revenue_minor)],
  ];
  return (
    <div>
      <h1>Today</h1>
      <p>A quiet snapshot of the business — not the machinery underneath.</p>
      <div className="grid">
        {cards.map(([label, value]) => (
          <div className="card" key={String(label)}><span className="muted">{label}</span><b>{value ?? 0}</b></div>
        ))}
      </div>
    </div>
  );
}

const LAGOS_AREAS = [
  "Agege", "Ajah", "Alausa", "Anthony", "Festac", "Gbagada", "Ikeja",
  "Ikorodu", "Ikoyi", "Lagos Island", "Lagos Mainland", "Lekki", "Magodo",
  "Maryland", "Ogba", "Oshodi", "Surulere", "Victoria Island", "Yaba",
];

type ProviderForm = {
  id: string;
  name: string;
  phone: string;
  whatsapp_number: string;
  area: string;
  address: string;
  availability: string;
  status: string;
  pool_ids: string[];
  password: string;
};

const emptyProvider: ProviderForm = {
  id: "", name: "", phone: "", whatsapp_number: "", area: "Lekki", address: "",
  availability: "available", status: "active", pool_ids: [], password: "",
};

function ProviderForm({ pools, initial, onSaved, onCancel }: {
  pools: Array<Record<string, unknown>>;
  initial: ProviderForm;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<ProviderForm>(initial);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const editing = Boolean(form.id);

  function togglePool(id: string) {
    setForm((current) => ({
      ...current,
      pool_ids: current.pool_ids.includes(id)
        ? current.pool_ids.filter((value) => value !== id)
        : [...current.pool_ids, id],
    }));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    if (!form.name.trim()) return setError("A name is required.");
    if (form.pool_ids.length === 0) return setError("Choose at least one service.");
    setSaving(true);
    try {
      await api.saveProvider({
        ...(form.id ? { id: form.id } : {}),
        name: form.name.trim(),
        phone: form.phone.trim(),
        whatsapp_number: (form.whatsapp_number || form.phone).trim(),
        area: form.area,
        address: form.address.trim() || `${form.area}, Lagos`,
        availability: form.availability,
        status: form.status,
        pool_ids: form.pool_ids,
        ...(form.password ? { password: form.password } : {}),
      });
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="card stack" onSubmit={submit}>
      <h2 style={{ margin: 0 }}>{editing ? "Edit handyman" : "Add a handyman"}</h2>
      <div className="grid tight">
        <label>Name<input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></label>
        <label>Phone<input value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} placeholder="+2348..." /></label>
        <label>WhatsApp<input value={form.whatsapp_number} onChange={(e) => setForm({ ...form, whatsapp_number: e.target.value })} placeholder="Same as phone" /></label>
        <label>Area
          <select value={form.area} onChange={(e) => setForm({ ...form, area: e.target.value })}>
            {LAGOS_AREAS.map((area) => <option key={area} value={area}>{area}</option>)}
          </select>
        </label>
      </div>
      <label>Address or landmark<input value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} /></label>
      <div className="grid tight">
        <label>Availability
          <select value={form.availability} onChange={(e) => setForm({ ...form, availability: e.target.value })}>
            <option value="available">Available</option>
            <option value="busy">Busy</option>
            <option value="offline">Offline</option>
          </select>
        </label>
        <label>Status
          <select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })}>
            <option value="active">Active</option>
            <option value="inactive">Deactivated</option>
          </select>
        </label>
        {editing ? null : <label>Portal password<input value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} placeholder="Leave blank for default" /></label>}
      </div>
      <div>
        <span className="muted">Services</span>
        <div className="row wrap" style={{ marginTop: 8 }}>
          {pools.map((pool) => {
            const id = String(pool.id);
            const on = form.pool_ids.includes(id);
            return (
              <button type="button" key={id} className={on ? "primary" : ""} onClick={() => togglePool(id)}>
                {String(pool.name)}
              </button>
            );
          })}
        </div>
      </div>
      {error ? <p className="error">{error}</p> : null}
      <div className="row wrap">
        <button className="primary" disabled={saving}>{saving ? "Saving…" : editing ? "Save changes" : "Add handyman"}</button>
        <button type="button" className="ghost" onClick={onCancel}>Cancel</button>
      </div>
    </form>
  );
}

function Providers() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  const [pools, setPools] = useState<Array<Record<string, unknown>>>([]);
  const [editing, setEditing] = useState<ProviderForm | null>(null);
  const [search, setSearch] = useState("");
  const [poolFilter, setPoolFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const load = useCallback(() => {
    api.providers().then(setRows).catch(() => undefined);
    api.pools().then(setPools).catch(() => undefined);
  }, []);
  useEffect(() => { load(); }, [load]);

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    return rows.filter((row) => {
      const providerPools = Array.isArray(row.pools) ? row.pools as Array<{ id: string; name: string }> : [];
      const searchable = [row.name, row.phone, row.area, ...providerPools.map((pool) => pool.name)].join(" ").toLowerCase();
      return (!query || searchable.includes(query))
        && (!poolFilter || providerPools.some((pool) => String(pool.id) === poolFilter))
        && (!statusFilter || String(row.status) === statusFilter);
    });
  }, [poolFilter, rows, search, statusFilter]);

  function editRow(row: Record<string, unknown>) {
    setEditing({
      id: String(row.id),
      name: String(row.name ?? ""),
      phone: String(row.phone ?? ""),
      whatsapp_number: String(row.whatsapp_number ?? ""),
      area: String(row.area ?? "Lekki"),
      address: String(row.address ?? ""),
      availability: String(row.availability ?? "available"),
      status: String(row.status ?? "active"),
      pool_ids: Array.isArray(row.pools) ? (row.pools as Array<{ id: string }>).map((pool) => String(pool.id)) : [],
      password: "",
    });
  }

  return (
    <div>
      <h1>Handymen</h1>
      <p>Who is available, where they work, what they have earned.</p>
      {editing ? (
        <ProviderForm
          pools={pools}
          initial={editing}
          onCancel={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      ) : (
        <button className="primary" onClick={() => setEditing({ ...emptyProvider })}>Add a handyman</button>
      )}
      <div className="filters" aria-label="Handyman filters">
        <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search name, phone, area or service" />
        <select aria-label="Filter by service" value={poolFilter} onChange={(event) => setPoolFilter(event.target.value)}>
          <option value="">All services</option>
          {pools.map((pool) => <option key={String(pool.id)} value={String(pool.id)}>{String(pool.name)}</option>)}
        </select>
        <select aria-label="Filter by account status" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
          <option value="">All account statuses</option>
          <option value="active">Active</option>
          <option value="inactive">Deactivated</option>
        </select>
      </div>
      <div className="list" style={{ marginTop: 16 }}>
        {filteredRows.length === 0 ? <div className="card muted">No handymen match these filters.</div> : null}
        {filteredRows.map((row) => (
          <div className="card row" key={String(row.id)}>
            <div>
              <strong>{String(row.name)}{row.status === "inactive" ? " · deactivated" : ""}</strong>
              <div className="muted">{String(row.area)} · {Number(row.rating_average).toFixed(1)}★ · {String(row.jobs_completed)} jobs · {money(row.earnings_minor)} earned</div>
              <div className="muted">{Array.isArray(row.pools) ? (row.pools as Array<{ name: string }>).map((p) => p.name).join(", ") : ""}</div>
              {Number(row.active_jobs) > 0 || Number(row.open_requests) > 0 ? (
                <div className="muted">{Number(row.active_jobs)} active · {Number(row.open_requests)} awaiting reply</div>
              ) : null}
            </div>
            <div className="stack">
              <span className="pill">{statusLabel(row.availability)}</span>
              <button onClick={() => editRow(row)}>Edit</button>
              <button onClick={() => api.availability(String(row.id), row.availability === "available" ? "offline" : "available").then(load)}>
                {row.availability === "available" ? "Set offline" : "Set available"}
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function Services() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<{ id: string; name: string; slug: string; status: string; displayOrder: string } | null>(null);
  const load = useCallback(() => { api.pools().then(setRows).catch(() => undefined); }, []);
  useEffect(() => { load(); }, [load]);

  function saveEditing(event: FormEvent) {
    event.preventDefault();
    if (!editing?.name.trim()) return;
    setError("");
    api.savePool({
      id: editing.id,
      name: editing.name.trim(),
      slug: editing.slug,
      status: editing.status,
      sort_order: Math.max(0, Number(editing.displayOrder || 1) - 1),
    }).then(() => {
      setEditing(null);
      load();
    }).catch((err) => setError(err instanceof Error ? err.message : "Could not save"));
  }

  return (
    <div>
      <h1>Services</h1>
      <p>The categories customers can choose from on WhatsApp. Reorder by sort value; deactivate to hide one without losing its history.</p>
      <form className="row" onSubmit={(event) => {
        event.preventDefault();
        setError("");
        if (!name.trim()) return;
        api.savePool({ name: name.trim(), sort_order: rows.length, status: "active" })
          .then(() => { setName(""); load(); })
          .catch((err) => setError(err instanceof Error ? err.message : "Could not add"));
      }}>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Add a service, e.g. Tiler" />
        <button className="primary">Add</button>
      </form>
      {error ? <p className="error">{error}</p> : null}
      <div className="list" style={{ marginTop: 16 }}>
        {rows.map((row) => {
          const isEditing = editing?.id === String(row.id);
          return (
            <div className="card" key={String(row.id)}>
              {isEditing && editing ? (
                <form className="row wrap" onSubmit={saveEditing}>
                  <label>Service name<input value={editing.name} onChange={(event) => setEditing({ ...editing, name: event.target.value })} /></label>
                  <label>Menu position<input type="number" min="1" value={editing.displayOrder} onChange={(event) => setEditing({ ...editing, displayOrder: event.target.value })} /></label>
                  <button className="primary">Save</button>
                  <button type="button" onClick={() => setEditing(null)}>Cancel</button>
                </form>
              ) : (
                <div className="row wrap">
                  <div>
                    <strong>{String(row.name)}</strong>
                    <div className="muted">Customer option {Number(row.sort_order) + 1} · {row.status === "inactive" ? "hidden" : "active"}</div>
                  </div>
                  <button onClick={() => setEditing({
                    id: String(row.id), name: String(row.name), slug: String(row.slug),
                    status: String(row.status || "active"), displayOrder: String(Number(row.sort_order) + 1),
                  })}>Edit</button>
                  <button onClick={() => api.savePool({
                    id: row.id, name: row.name, slug: row.slug, sort_order: row.sort_order,
                    status: row.status === "inactive" ? "active" : "inactive",
                  }).then(load)}>
                    {row.status === "inactive" ? "Activate" : "Deactivate"}
                  </button>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function Requests() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  useEffect(() => { api.requests().then(setRows).catch(() => undefined); }, []);
  const statuses = useMemo(() => [...new Set(rows.map((row) => String(row.status)))].sort(), [rows]);
  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    return rows.filter((row) => {
      const pool = row.pool as { name?: string } | undefined;
      const provider = row.assigned_provider as { name?: string } | undefined;
      const searchable = [row.public_code, row.customer_name, row.customer_phone, row.area, pool?.name, provider?.name].join(" ").toLowerCase();
      return (!query || searchable.includes(query)) && (!statusFilter || row.status === statusFilter);
    });
  }, [rows, search, statusFilter]);
  return (
    <div>
      <h1>Jobs</h1>
      <div className="filters" aria-label="Job filters">
        <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search job, customer, phone, area or handyman" />
        <select aria-label="Filter by job status" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
          <option value="">All job statuses</option>
          {statuses.map((status) => <option key={status} value={status}>{statusLabel(status)}</option>)}
        </select>
      </div>
      <div className="list">
        {filteredRows.length === 0 ? <div className="card muted">No jobs match these filters.</div> : null}
        {filteredRows.map((row) => (
          <Link className="card row" key={String(row.public_code)} to={`/owner/jobs/${row.id}`}>
            <div>
              <strong>{String(row.public_code)} · {(row.pool as { name?: string } | undefined)?.name || "Service"}</strong>
              <div className="muted">{String(row.area)} · {statusLabel(row.status)}</div>
            </div>
            <span>{(row.assigned_provider as { name?: string } | undefined)?.name || "Unassigned"}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}

function JobDetail() {
  const { id = "" } = useParams();
  const [job, setJob] = useState<Record<string, unknown> | null>(null);
  const [matches, setMatches] = useState<Array<Record<string, unknown>>>([]);
  const [messages, setMessages] = useState<Array<Record<string, unknown>>>([]);
  const [draft, setDraft] = useState("");
  const [weights, setWeights] = useState<Record<string, number>>({});
  useEffect(() => {
    api.request(id).then(setJob).catch(() => undefined);
    api.matches(id).then(setMatches).catch(() => undefined);
    api.messages(id).then(setMessages).catch(() => undefined);
    api.settings().then((s) => setWeights({
      service: Number(s.weight_service ?? 0.4) * 100,
      availability: Number(s.weight_availability ?? 0.2) * 100,
      distance: Number(s.weight_distance ?? 0.2) * 100,
      rating: Number(s.weight_rating ?? 0.15) * 100,
      experience: Number(s.weight_experience ?? 0.05) * 100,
    })).catch(() => undefined);
  }, [id]);
  if (!job) return <p>Loading…</p>;
  const share = (key: string) => Math.round(weights[key] ?? 0);
  return (
    <div>
      <h1>{String(job.public_code)}</h1>
      <p>{(job.pool as { name?: string })?.name} in {String(job.area)} — {String(job.description)}</p>
      <div className="row wrap">
        <button onClick={() => api.takeOver(id)}>Take over</button>
        <button onClick={() => api.release(id)}>Release to handyman</button>
        <button onClick={() => api.close(id)}>Close</button>
      </div>
      <h2>Matching</h2>
      <div className="list">
        {matches.map((match) => {
          const breakdown = JSON.parse(String(match.breakdown || "{}"));
          return (
            <div className="card" key={String(match.rank)}>
              <div className="row"><strong>{(match.provider as { name?: string })?.name}</strong><b>{Number(match.score).toFixed(0)}%</b></div>
              <p className="muted">
                Service {Number(breakdown.service).toFixed(0)}/{share("service")} ·
                Availability {Number(breakdown.availability).toFixed(0)}/{share("availability")} ·
                Distance {Number(breakdown.distance).toFixed(0)}/{share("distance")} ·
                Rating {Number(breakdown.rating).toFixed(0)}/{share("rating")} ·
                Experience {Number(breakdown.experience).toFixed(0)}/{share("experience")}
                {breakdown.distance_km ? ` · ${Number(breakdown.distance_km).toFixed(1)} km away` : ""}
                {Number(breakdown.workload_penalty) > 0 ? ` · −${Number(breakdown.workload_penalty).toFixed(0)} for current workload` : ""}
              </p>
            </div>
          );
        })}
      </div>
      <h2>Conversation</h2>
      <div className="chat">
        {messages.map((message) => <div key={String(message.id)} className={`bubble ${message.author_type === "provider" || message.author_type === "owner" ? "me" : ""}`}><span className="muted">{String(message.author_type)}</span><div>{String(message.body)}</div></div>)}
      </div>
      <form className="row" onSubmit={(event) => { event.preventDefault(); api.sendMessage(id, draft).then(() => { setDraft(""); return api.messages(id).then(setMessages); }); }}>
        <input value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="Message the customer" />
        <button className="primary">Send</button>
      </form>
    </div>
  );
}

function SettingsPage() {
  const [form, setForm] = useState({
    booking_fee: "5000",
    require_booking_fee: true,
    booking_fee_refundable: false,
    acceptance_window_seconds: "120",
    max_distance_km: "25",
    welcome_message: "",
    company_display_name: "",
    provider_portal_base_url: "",
    weight_service: "40",
    weight_availability: "20",
    weight_distance: "20",
    weight_rating: "15",
    weight_experience: "5",
  });
  const [saved, setSaved] = useState("");
  useEffect(() => {
    api.settings().then((s) => setForm({
      booking_fee: String(Number(s.booking_fee_minor || 0) / 100),
      require_booking_fee: Boolean(s.require_booking_fee),
      booking_fee_refundable: Boolean(s.booking_fee_refundable),
      acceptance_window_seconds: String(s.acceptance_window_seconds ?? 120),
      max_distance_km: String(s.max_distance_km ?? 25),
      welcome_message: String(s.welcome_message ?? ""),
      company_display_name: String(s.company_display_name ?? ""),
      provider_portal_base_url: String(s.provider_portal_base_url ?? ""),
      weight_service: String(Math.round(Number(s.weight_service ?? 0.4) * 100)),
      weight_availability: String(Math.round(Number(s.weight_availability ?? 0.2) * 100)),
      weight_distance: String(Math.round(Number(s.weight_distance ?? 0.2) * 100)),
      weight_rating: String(Math.round(Number(s.weight_rating ?? 0.15) * 100)),
      weight_experience: String(Math.round(Number(s.weight_experience ?? 0.05) * 100)),
    })).catch(() => undefined);
  }, []);
  return (
    <div>
      <h1>Settings</h1>
      <p>Booking fee, matching weights, and how providers are contacted.</p>
      <form className="card stack" style={{ maxWidth: 520 }} onSubmit={(event) => {
        event.preventDefault();
        api.saveSettings({
          booking_fee_minor: Math.round(Number(form.booking_fee) * 100),
          require_booking_fee: form.require_booking_fee,
          booking_fee_refundable: form.booking_fee_refundable,
          currency: "NGN",
          acceptance_window_seconds: Number(form.acceptance_window_seconds),
          max_distance_km: Number(form.max_distance_km),
          welcome_message: form.welcome_message,
          company_display_name: form.company_display_name,
          provider_portal_base_url: form.provider_portal_base_url,
          weight_service: Number(form.weight_service) / 100,
          weight_availability: Number(form.weight_availability) / 100,
          weight_distance: Number(form.weight_distance) / 100,
          weight_rating: Number(form.weight_rating) / 100,
          weight_experience: Number(form.weight_experience) / 100,
        }).then(() => setSaved("Saved")).catch((err) => setSaved(err instanceof Error ? err.message : "Could not save"));
      }}>
        <label>Company name<input value={form.company_display_name} onChange={(e) => setForm({ ...form, company_display_name: e.target.value })} /></label>
        <label>Booking fee (₦)<input value={form.booking_fee} onChange={(e) => setForm({ ...form, booking_fee: e.target.value })} /></label>
        <label className="check"><input type="checkbox" checked={form.require_booking_fee} onChange={(e) => setForm({ ...form, require_booking_fee: e.target.checked })} /> Require booking fee before matching</label>
        <label className="check"><input type="checkbox" checked={form.booking_fee_refundable} onChange={(e) => setForm({ ...form, booking_fee_refundable: e.target.checked })} /> Booking fee is refundable</label>
        <label>Acceptance window (seconds)<input value={form.acceptance_window_seconds} onChange={(e) => setForm({ ...form, acceptance_window_seconds: e.target.value })} /></label>
        <label>Max distance (km)<input value={form.max_distance_km} onChange={(e) => setForm({ ...form, max_distance_km: e.target.value })} /></label>
        <label>Provider portal URL<input value={form.provider_portal_base_url} onChange={(e) => setForm({ ...form, provider_portal_base_url: e.target.value })} /></label>
        <label>Welcome message<textarea rows={4} value={form.welcome_message} onChange={(e) => setForm({ ...form, welcome_message: e.target.value })} /></label>
        <h2>Matching weights (%)</h2>
        <div className="grid tight">
          <label>Service<input value={form.weight_service} onChange={(e) => setForm({ ...form, weight_service: e.target.value })} /></label>
          <label>Availability<input value={form.weight_availability} onChange={(e) => setForm({ ...form, weight_availability: e.target.value })} /></label>
          <label>Distance<input value={form.weight_distance} onChange={(e) => setForm({ ...form, weight_distance: e.target.value })} /></label>
          <label>Rating<input value={form.weight_rating} onChange={(e) => setForm({ ...form, weight_rating: e.target.value })} /></label>
          <label>Experience<input value={form.weight_experience} onChange={(e) => setForm({ ...form, weight_experience: e.target.value })} /></label>
        </div>
        <button className="primary">Save</button>
        {saved ? <p className="muted">{saved}</p> : null}
      </form>
    </div>
  );
}

function Quotes() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  useEffect(() => { api.quotes().then(setRows).catch(() => undefined); }, []);
  return (
    <div>
      <h1>Quotes</h1>
      <div className="list">{rows.map((row) => <div className="card row" key={String(row.public_code)}><strong>{String(row.public_code)}</strong><span>{money(row.total_minor)} · {statusLabel(row.status)}</span></div>)}</div>
    </div>
  );
}

function Conversations() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  useEffect(() => { api.conversations().then(setRows).catch(() => undefined); }, []);
  return (
    <div>
      <h1>Conversations</h1>
      <p>Active customer ↔ handyman threads.</p>
      <div className="list">
        {rows.map((row) => (
          <Link className="card row" key={String(row.id)} to={`/owner/jobs/${row.id}`}>
            <div>
              <strong>{String(row.customer_name || row.customer_phone || "Customer")}</strong>
              <div className="muted">{(row.pool as { name?: string })?.name} · {statusLabel(row.status)}</div>
            </div>
            <span>{(row.assigned_provider as { name?: string })?.name || "Unassigned"}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}

function Payments() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  useEffect(() => { api.transactions().then(setRows).catch(() => undefined); }, []);
  return (
    <div>
      <h1>Payments</h1>
      <p>Booking fees and quote payments.</p>
      <div className="list">
        {rows.map((row, index) => (
          <div className="card row" key={`${row.code}-${index}`}>
            <div>
              <strong>{String(row.code)}</strong>
              <div className="muted">{statusLabel(row.kind)} · {statusLabel(row.status)}</div>
            </div>
            <span>{money(row.amount_minor)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function ProviderHome() {
  const [data, setData] = useState<Record<string, unknown> | null>(null);
  const load = useCallback(() => { api.home().then(setData).catch(() => undefined); }, []);
  useEffect(() => { load(); }, [load]);
  if (!data) return <p>Loading…</p>;
  const provider = data.provider as Record<string, unknown>;
  const current = data.current_job as Record<string, unknown> | undefined;
  const request = current?.request as Record<string, unknown> | undefined;
  const hasCurrentJob = Boolean(request?.id) && request?.id !== "00000000-0000-0000-0000-000000000000";
  return (
    <div>
      <h1>Hi, {String(provider?.name || "there")}</h1>
      <div className="grid">
        <div className="card"><span className="muted">Availability</span><b>{statusLabel(provider?.availability)}</b>
          <button style={{ marginTop: 10 }} onClick={() => api.availability(String(provider.id), provider.availability === "available" ? "offline" : "available").then(load)}>
            {provider.availability === "available" ? "Go offline" : "Go available"}
          </button>
        </div>
        <div className="card"><span className="muted">Rating</span><b>{Number(provider?.rating_average || 0).toFixed(1)}★</b></div>
        <div className="card"><span className="muted">Jobs done</span><b>{String(provider?.jobs_completed)}</b></div>
        <div className="card"><span className="muted">Completed today</span><b>{String(data.completed_today ?? 0)}</b></div>
        <div className="card"><span className="muted">Earnings</span><b>{money(data.earnings_minor)}</b></div>
      </div>
      {hasCurrentJob ? <div className="card" style={{ marginTop: 16 }}><h2>Current job</h2><p>{String(request?.description)} · {String(request?.area)}</p><Link to={`/provider/jobs/${request?.id}`}>Open job</Link></div> : null}
    </div>
  );
}

function ProviderInbox() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  const navigate = useNavigate();
  useEffect(() => { api.inbox().then(setRows).catch(() => undefined); }, []);
  return (
    <div>
      <h1>Requests</h1>
      <div className="list">
        {rows.length === 0 ? <div className="card muted">No pending requests right now.</div> : null}
        {rows.map((row) => {
          const request = row.request as Record<string, unknown> | undefined;
          const pool = request?.pool as { name?: string } | undefined;
          return (
            <div className="card" key={String(row.id)}>
              <div className="row"><strong>{pool?.name || "Service request"}</strong><span className="muted">Expires {new Date(String(row.expires_at)).toLocaleTimeString()}</span></div>
              <p className="muted">{String(request?.area || "Lagos")} · {String(request?.preferred_at || "Flexible")}</p>
              <p>{String(request?.description || "Open the portal for details.")}</p>
              <div className="row">
                <button className="primary" onClick={() => api.accept(String(row.id)).then(() => navigate("/provider/jobs"))}>Accept</button>
                <button onClick={() => api.decline(String(row.id)).then(() => api.inbox().then(setRows))}>Decline</button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function ProviderJobs() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  useEffect(() => { api.requests().then(setRows).catch(() => undefined); }, []);
  return (
    <div>
      <h1>My jobs</h1>
      <div className="list">
        {rows.map((row) => (
          <Link className="card" key={String(row.public_code)} to={`/provider/jobs/${row.id}`}>
            <strong>{(row.pool as { name?: string })?.name}</strong>
            <div className="muted">{String(row.area)} · {statusLabel(row.status)}</div>
          </Link>
        ))}
      </div>
    </div>
  );
}

// Mirrors the server-side job state machine so the portal only offers moves the
// API will actually accept.
const NEXT_STATUSES: Record<string, string[]> = {
  assigned: ["on_the_way", "arrived"],
  on_the_way: ["arrived"],
  arrived: ["in_progress"],
  in_progress: ["completed"],
  quote_sent: ["in_progress"],
  quote_approved: ["in_progress"],
  payment_confirmed: ["in_progress", "completed"],
  completed: [],
  cancelled: [],
};

function ProviderJob() {
  const { id = "" } = useParams();
  const [job, setJob] = useState<Record<string, unknown> | null>(null);
  const [messages, setMessages] = useState<Array<Record<string, unknown>>>([]);
  const [quotes, setQuotes] = useState<Array<Record<string, unknown>>>([]);
  const [draft, setDraft] = useState("");
  const [labour, setLabour] = useState("20000");
  const [materials, setMaterials] = useState("12500");
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");
  const load = useCallback(() => {
    api.request(id).then(setJob).catch(() => undefined);
    api.messages(id).then(setMessages).catch(() => undefined);
    api.quotes().then((rows) => setQuotes(rows.filter((row) => String(row.request_id) === id))).catch(() => undefined);
  }, [id]);
  useEffect(() => { load(); }, [load]);
  if (!job) return <p>Loading…</p>;

  const status = String(job.status ?? "");
  const nextSteps = NEXT_STATUSES[status] ?? [];
  const latestQuote = quotes[0];
  const canQuote = ["assigned", "on_the_way", "arrived", "in_progress", "quote_sent", "quote_approved"].includes(status);

  function act(promise: Promise<unknown>) {
    setError("");
    promise.then(load).catch((err) => setError(err instanceof Error ? err.message : "That didn't work"));
  }

  return (
    <div>
      <h1>{(job.pool as { name?: string })?.name}</h1>
      <p>{String(job.area)} · {String(job.description)}</p>
      <p className="muted">{String(job.customer_name || "Customer")} · {String(job.address)} · {String(job.preferred_at)}</p>
      <p><span className="pill">{statusLabel(status)}</span></p>
      {nextSteps.length > 0 ? (
        <div className="row wrap">
          {nextSteps.map((next) => (
            <button key={next} className="primary" onClick={() => act(api.status(id, next))}>{statusLabel(next)}</button>
          ))}
        </div>
      ) : <p className="muted">This job is {statusLabel(status)} — nothing left to update.</p>}
      {error ? <p className="error">{error}</p> : null}

      <h2>Chat</h2>
      <div className="chat">
        {messages.length === 0 ? <p className="muted">No messages yet.</p> : null}
        {messages.map((message) => (
          <div className={`bubble ${message.author_type === "provider" ? "me" : ""}`} key={String(message.id)}>
            <span className="muted">{statusLabel(message.author_type)}</span>
            <div>{String(message.body)}</div>
          </div>
        ))}
      </div>
      <form className="row" onSubmit={(event) => { event.preventDefault(); if (!draft.trim()) return; act(api.sendMessage(id, draft).then(() => setDraft(""))); }}>
        <input value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="Message customer" />
        <button className="primary">Send</button>
      </form>

      <h2>Quote</h2>
      {latestQuote ? (
        <div className="card row">
          <div>
            <strong>{String(latestQuote.public_code)}</strong>
            <div className="muted">{statusLabel(latestQuote.status)}</div>
          </div>
          <span>{money(latestQuote.total_minor)}</span>
        </div>
      ) : null}
      {canQuote ? (
        <form className="card stack" style={{ maxWidth: 420, marginTop: 12 }} onSubmit={(event) => {
          event.preventDefault();
          act(api.quote(id, {
            labour_minor: Math.round(Number(labour) * 100),
            materials_minor: Math.round(Number(materials) * 100),
            notes,
          }));
        }}>
          <label>Labour (₦)<input value={labour} onChange={(e) => setLabour(e.target.value)} /></label>
          <label>Materials (₦)<input value={materials} onChange={(e) => setMaterials(e.target.value)} /></label>
          <label>Notes<textarea rows={3} value={notes} onChange={(e) => setNotes(e.target.value)} /></label>
          <p className="muted">Total {money(Math.round((Number(labour) + Number(materials)) * 100))}</p>
          <button className="primary">{latestQuote ? "Send a revised quote" : "Send quote"}</button>
        </form>
      ) : <p className="muted">A quote can't be sent for a job that is {statusLabel(status)}.</p>}
    </div>
  );
}

function SimpleList({ title, loader, line }: { title: string; loader: () => Promise<Array<Record<string, unknown>>>; line: (row: Record<string, unknown>) => string }) {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  useEffect(() => { loader().then(setRows).catch(() => undefined); }, [loader]);
  return (
    <div>
      <h1>{title}</h1>
      <div className="list">{rows.map((row, index) => <div className="card" key={String(row.id || index)}>{line(row)}</div>)}</div>
    </div>
  );
}

function AuditLog() {
  const [rows, setRows] = useState<Array<Record<string, unknown>>>([]);
  useEffect(() => { api.audit().then(setRows).catch(() => undefined); }, []);
  return (
    <div>
      <h1>Audit log</h1>
      <p>Important business and system activity, newest first.</p>
      <div className="list">
        {rows.map((row, index) => {
          const actorID = String(row.actor_user_id || "");
          const actor = row.actor_email ? String(row.actor_email) : actorID ? `Team member · ${actorID.slice(0, 8)}` : "System";
          const target = statusLabel(row.target_type || "record");
          const created = row.created_at ? new Date(String(row.created_at)).toLocaleString() : "Time unavailable";
          return (
            <div className="card row wrap" key={String(row.id || index)}>
              <div>
                <strong>{statusLabel(row.action)}</strong>
                <div className="muted">{target} · {actor}</div>
              </div>
              <time className="muted" dateTime={String(row.created_at || "")}>{created}</time>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export default function App() {
  const { user, ready, setUser } = useSession();
  const ownerLinks = useMemo(() => [
    { to: "/owner", label: "Overview" },
    { to: "/owner/handymen", label: "Handymen" },
    { to: "/owner/services", label: "Services" },
    { to: "/owner/jobs", label: "Jobs" },
    { to: "/owner/conversations", label: "Conversations" },
    { to: "/owner/customers", label: "Customers" },
    { to: "/owner/quotes", label: "Quotes" },
    { to: "/owner/payments", label: "Payments" },
    { to: "/owner/audit", label: "Audit" },
    { to: "/owner/settings", label: "Settings" },
  ], []);
  const providerLinks = useMemo(() => [
    { to: "/provider", label: "Home" },
    { to: "/provider/inbox", label: "Requests" },
    { to: "/provider/jobs", label: "Jobs" },
  ], []);
  if (!ready) return null;
  const providerUser = user?.role === "service_provider";
  const ownerPage = (content: ReactNode) => user && !providerUser
    ? <Shell user={user} links={ownerLinks}>{content}</Shell>
    : <Navigate to={user ? "/provider" : "/login"} />;
  const providerPage = (content: ReactNode) => user && providerUser
    ? <Shell user={user} links={providerLinks}>{content}</Shell>
    : <Navigate to={user ? "/owner" : "/login"} />;
  return (
    <Routes>
      <Route path="/login" element={<Login onAuthenticated={setUser} />} />
      <Route path="/owner" element={ownerPage(<Overview />)} />
      <Route path="/owner/handymen" element={ownerPage(<Providers />)} />
      <Route path="/owner/services" element={ownerPage(<Services />)} />
      <Route path="/owner/jobs" element={ownerPage(<Requests />)} />
      <Route path="/owner/jobs/:id" element={ownerPage(<JobDetail />)} />
      <Route path="/owner/conversations" element={ownerPage(<Conversations />)} />
      <Route path="/owner/quotes" element={ownerPage(<Quotes />)} />
      <Route path="/owner/payments" element={ownerPage(<Payments />)} />
      <Route path="/owner/customers" element={ownerPage(<SimpleList title="Customers" loader={api.customers} line={(row) => `${row.name || "Customer"} · ${row.phone || ""}`} />)} />
      <Route path="/owner/audit" element={ownerPage(<AuditLog />)} />
      <Route path="/owner/settings" element={ownerPage(<SettingsPage />)} />
      <Route path="/provider" element={providerPage(<ProviderHome />)} />
      <Route path="/provider/inbox" element={providerPage(<ProviderInbox />)} />
      <Route path="/provider/jobs" element={providerPage(<ProviderJobs />)} />
      <Route path="/provider/jobs/:id" element={providerPage(<ProviderJob />)} />
      <Route path="*" element={<Navigate to={getToken() ? (user?.role === "service_provider" ? "/provider" : "/owner") : "/login"} />} />
    </Routes>
  );
}
