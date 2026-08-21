import { FormEvent, useEffect, useState } from "react";
import { apiGet, apiPatch, apiPost } from "../api/client";
import { Badge, Card, Drawer, EmptyState, Flash, FormGrid, Page, StatusDot } from "../components/ui";
import { fulfilmentLabel, humanStatus, money, slugify, type Row } from "../lib/format";

const days = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

type Store = Row & { id: string; name?: string; status?: string; hours?: Row[]; fulfilment_modes?: Row[] };
type Member = Row & { id: string; role?: string; user?: Row };

function defaultHours() {
  return days.map((_, index) => ({ day_of_week: index, opens_at: "09:00", closes_at: "21:00", is_closed: index === 0 }));
}

function defaultFulfilment() {
  return [
    { mode: "pickup", enabled: true, delivery_fee_minor: 0 },
    { mode: "customer_rider", enabled: true, delivery_fee_minor: 0 },
    { mode: "merchant_rider", enabled: false, delivery_fee_minor: 0 },
  ];
}

export function StoresPage() {
  const [stores, setStores] = useState<Store[]>([]);
  const [members, setMembers] = useState<Member[]>([]);
  const [inventory, setInventory] = useState<Row[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");
  const [form, setForm] = useState({ name: "", address: "", city: "", country: "NG", code: "" });
  const [hours, setHours] = useState(defaultHours());
  const [fulfilment, setFulfilment] = useState(defaultFulfilment());

  async function load() {
    try {
      const [storeResponse, memberResponse, inventoryResponse] = await Promise.all([
        apiGet<Store[]>("/stores"),
        apiGet<Member[]>("/organizations/current/members").catch(() => ({ data: [] as Member[] })),
        apiGet<Row[]>("/inventory").catch(() => ({ data: [] as Row[] })),
      ]);
      setStores(storeResponse.data);
      setMembers(memberResponse.data);
      setInventory(inventoryResponse.data);
      setSelectedID((current) => current || storeResponse.data[0]?.id || "");
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load stores");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function startCreate() {
    setForm({ name: "", address: "", city: "", country: "NG", code: "" });
    setHours(defaultHours());
    setFulfilment(defaultFulfilment());
    setOpen(true);
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    await apiPost<Store>("/stores", {
      name: form.name,
      code: form.code || slugify(form.name).toUpperCase(),
      address: form.address,
      city: form.city,
      country: form.country,
      status: "active",
      hours,
      fulfilment_modes: fulfilment,
    });
    setOpen(false);
    setFlash("Store created.");
    await load();
  }

  async function saveSelected(event: FormEvent) {
    event.preventDefault();
    if (!selected) return;
    await apiPatch<Store>(`/stores/${selected.id}`, {
      name: form.name || selected.name,
      address: form.address || selected.address,
      city: form.city || selected.city,
      country: form.country || selected.country,
      hours: (selected.hours ?? []).length ? selected.hours : hours,
      fulfilment_modes: fulfilmentFrom(selected),
    });
    setFlash("Store saved.");
    await load();
  }

  async function toggleStatus(store: Store) {
    await apiPost<Store>(`/stores/${store.id}/${store.status === "active" ? "deactivate" : "activate"}`, {});
    await load();
  }

  const selected = stores.find((store) => store.id === selectedID);
  const stockCount = inventory.filter((row) => String(row.store_id) === selectedID).length;

  function fulfilmentFrom(store: Store) {
    const modes = store.fulfilment_modes ?? [];
    if (modes.length) {
      return ["pickup", "customer_rider", "merchant_rider"].map((mode) => {
        const existing = modes.find((item) => item.mode === mode);
        return { mode, enabled: Boolean(existing?.enabled), delivery_fee_minor: Number(existing?.delivery_fee_minor ?? 0) };
      });
    }
    return defaultFulfilment();
  }

  return (
    <Page
      title="Stores"
      description="Locations, hours, and how customers receive their orders."
      actions={<button type="button" className="primary" onClick={startCreate}>Add store</button>}
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <div className="detail-layout">
        <Card>
          {stores.length === 0 ? (
            <EmptyState title="No stores yet" body="Add your first location so customers can order." action={<button type="button" className="primary" onClick={startCreate}>Add store</button>} />
          ) : stores.map((store) => (
            <button key={store.id} type="button" className={selectedID === store.id ? "setup-item complete" : "setup-item"} onClick={() => setSelectedID(store.id)} style={{ width: "100%", marginBottom: 8 }}>
              <span>
                <strong>{store.name}</strong>
                <em>{String(store.city || store.address || "No address")}</em>
              </span>
              <Badge tone={store.status === "active" ? "success" : "warning"}>{humanStatus(store.status)}</Badge>
            </button>
          ))}
        </Card>
        <Card>
          {!selected ? (
            <EmptyState title="Select a store" body="See hours, fulfilment, and stock." />
          ) : (
            <>
              <h3>{selected.name}</h3>
              <p><StatusDot live={selected.status === "active"} label={selected.status === "active" ? "Open for orders" : "Inactive"} /></p>
              <p>{String(selected.address || "No address")}</p>
              <p className="muted">{[selected.city, selected.country].filter(Boolean).join(", ")}</p>
              <h3>Opening hours</h3>
              {(selected.hours ?? []).length === 0 ? <p className="muted">No hours set yet.</p> : (selected.hours ?? []).map((hour) => (
                <p key={String(hour.day_of_week)}>{days[Number(hour.day_of_week)]}: {hour.is_closed ? "Closed" : `${hour.opens_at} – ${hour.closes_at}`}</p>
              ))}
              <h3>Fulfilment</h3>
              {(selected.fulfilment_modes ?? []).filter((mode) => mode.enabled).map((mode) => (
                <p key={String(mode.mode)}>{fulfilmentLabel(String(mode.mode))}{Number(mode.delivery_fee_minor) ? ` · ${money(mode.delivery_fee_minor)}` : ""}</p>
              ))}
              <p className="muted">{stockCount} inventory record{stockCount === 1 ? "" : "s"}</p>
              <p className="muted">{members.filter((member) => ["store_manager", "store_staff"].includes(String(member.role))).length} store-scoped staff (assign access in Team).</p>
              <div className="page-actions">
                <button type="button" onClick={() => void toggleStatus(selected)}>{selected.status === "active" ? "Deactivate" : "Activate"}</button>
              </div>
              <form className="form-grid" onSubmit={saveSelected}>
                <strong>Update details</strong>
                <label>Name<input value={form.name} placeholder={String(selected.name ?? "")} onChange={(event) => setForm({ ...form, name: event.target.value })} /></label>
                <label>Address<input value={form.address} placeholder={String(selected.address ?? "")} onChange={(event) => setForm({ ...form, address: event.target.value })} /></label>
                <label>City<input value={form.city} placeholder={String(selected.city ?? "")} onChange={(event) => setForm({ ...form, city: event.target.value })} /></label>
                <label>Country<input value={form.country} onChange={(event) => setForm({ ...form, country: event.target.value })} /></label>
                <div className="full"><button type="submit">Save store</button></div>
              </form>
            </>
          )}
        </Card>
      </div>
      <Drawer open={open} title="Add store" onClose={() => setOpen(false)}>
        <FormGrid onSubmit={save}>
          <label className="full">Store name<input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value, code: slugify(event.target.value).toUpperCase() })} required /></label>
          <label className="full">Address<input value={form.address} onChange={(event) => setForm({ ...form, address: event.target.value })} /></label>
          <label>City<input value={form.city} onChange={(event) => setForm({ ...form, city: event.target.value })} /></label>
          <label>Country<input value={form.country} onChange={(event) => setForm({ ...form, country: event.target.value })} /></label>
          <div className="full">
            <strong>Hours</strong>
            <div className="hours-grid">
              {hours.map((hour, index) => (
                <div className="hours-row" key={hour.day_of_week}>
                  <span>{days[index]}</span>
                  <input type="time" value={hour.opens_at} disabled={hour.is_closed} onChange={(event) => setHours(hours.map((item, i) => i === index ? { ...item, opens_at: event.target.value } : item))} />
                  <input type="time" value={hour.closes_at} disabled={hour.is_closed} onChange={(event) => setHours(hours.map((item, i) => i === index ? { ...item, closes_at: event.target.value } : item))} />
                  <label><input type="checkbox" checked={hour.is_closed} onChange={(event) => setHours(hours.map((item, i) => i === index ? { ...item, is_closed: event.target.checked } : item))} /> Closed</label>
                </div>
              ))}
            </div>
          </div>
          <div className="full">
            <strong>How customers receive orders</strong>
            {fulfilment.map((mode, index) => (
              <label key={mode.mode}>
                <input type="checkbox" checked={mode.enabled} onChange={(event) => setFulfilment(fulfilment.map((item, i) => i === index ? { ...item, enabled: event.target.checked } : item))} />
                {fulfilmentLabel(mode.mode)}
              </label>
            ))}
          </div>
          <div className="full"><button type="submit">Create store</button></div>
        </FormGrid>
      </Drawer>
    </Page>
  );
}
