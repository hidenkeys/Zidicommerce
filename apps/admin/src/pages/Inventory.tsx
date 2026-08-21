import { FormEvent, useEffect, useMemo, useState } from "react";
import { apiGet, apiPatch, apiPost } from "../api/client";
import { Badge, Card, ConfirmButton, EmptyState, FilterBar, Flash, FormGrid, Page, SearchField } from "../components/ui";
import { type Row } from "../lib/format";

type Store = Row & { id: string; name?: string };

export function InventoryPage() {
  const [rows, setRows] = useState<Row[]>([]);
  const [stores, setStores] = useState<Store[]>([]);
  const [products, setProducts] = useState<Row[]>([]);
  const [storeFilter, setStoreFilter] = useState("all");
  const [query, setQuery] = useState("");
  const [form, setForm] = useState({ store_id: "", variant_id: "", on_hand: "", reorder_threshold: "5" });
  const [adjusting, setAdjusting] = useState<Row | null>(null);
  const [quantity, setQuantity] = useState("");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

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
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load inventory");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function createStock(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/inventory", {
      store_id: form.store_id,
      variant_id: form.variant_id,
      on_hand: Number(form.on_hand),
      reorder_threshold: Number(form.reorder_threshold),
    });
    setForm({ store_id: form.store_id, variant_id: "", on_hand: "", reorder_threshold: "5" });
    setFlash("Stock saved.");
    await load();
  }

  async function saveAdjustment() {
    if (!adjusting) return;
    await apiPatch<Row>(`/inventory/${String(adjusting.id)}`, { set_on_hand: Number(quantity) });
    setAdjusting(null);
    setQuantity("");
    setFlash("Quantity updated.");
    await load();
  }

  const filtered = useMemo(() => {
    return rows.filter((row) => {
      const productName = String((row.variant as Row | undefined)?.product ? ((row.variant as Row).product as Row).name : (row.variant as Row | undefined)?.name ?? "");
      const matchesStore = storeFilter === "all" || String(row.store_id) === storeFilter;
      const matchesQuery = !query || productName.toLowerCase().includes(query.toLowerCase());
      return matchesStore && matchesQuery;
    });
  }, [rows, storeFilter, query]);

  return (
    <Page
      title="Inventory"
      description="What you have, and what you need to restock."
      help="Available is on-hand minus reserved (items in unpaid or in-progress orders)."
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search product" />
        <select value={storeFilter} onChange={(event) => setStoreFilter(event.target.value)}>
          <option value="all">All stores</option>
          {stores.map((store) => <option key={store.id} value={store.id}>{store.name}</option>)}
        </select>
      </FilterBar>
      <FormGrid onSubmit={createStock} title="Set stock">
        <label>Store
          <select value={form.store_id} onChange={(event) => setForm({ ...form, store_id: event.target.value })} required>
            <option value="">Choose store</option>
            {stores.map((store) => <option key={store.id} value={store.id}>{store.name}</option>)}
          </select>
        </label>
        <label>Product
          <select value={form.variant_id} onChange={(event) => setForm({ ...form, variant_id: event.target.value })} required>
            <option value="">Choose product</option>
            {products.flatMap((product) => (Array.isArray(product.variants) ? product.variants as Row[] : []).map((variant) => (
              <option key={String(variant.id)} value={String(variant.id)}>{String(product.name)}{variant.name && variant.name !== "Regular" ? ` · ${variant.name}` : ""}</option>
            )))}
          </select>
        </label>
        <label>Quantity on hand<input type="number" min="0" value={form.on_hand} onChange={(event) => setForm({ ...form, on_hand: event.target.value })} required /></label>
        <label>Low-stock alert at<input type="number" min="0" value={form.reorder_threshold} onChange={(event) => setForm({ ...form, reorder_threshold: event.target.value })} /></label>
        <div className="full"><button type="submit">Save stock</button></div>
      </FormGrid>
      <Card>
        {filtered.length === 0 ? (
          <EmptyState title="No stock recorded" body="Set quantities after you add products." />
        ) : (
          <div className="table-wrap" style={{ border: 0, margin: 0 }}>
            <table>
              <thead>
                <tr>
                  <th>Product</th>
                  <th>Store</th>
                  <th>Available</th>
                  <th>Reserved</th>
                  <th>On hand</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((row) => {
                  const variant = (row.variant ?? {}) as Row;
                  const product = (variant.product ?? {}) as Row;
                  const store = stores.find((item) => item.id === row.store_id);
                  const available = Number(row.on_hand ?? 0) - Number(row.reserved ?? 0);
                  const threshold = Number(row.reorder_threshold ?? 0);
                  const out = available <= 0;
                  const low = !out && available <= threshold;
                  return (
                    <tr key={String(row.id)}>
                      <td>{String(product.name || variant.name || "Product")}{variant.name && variant.name !== "Regular" ? ` · ${variant.name}` : ""}</td>
                      <td>{store?.name || "Store"}</td>
                      <td className={out ? "stock-out" : low ? "stock-low" : undefined}>
                        {available} {out ? <Badge tone="danger">Out of stock</Badge> : low ? <Badge tone="warning">Low stock</Badge> : null}
                      </td>
                      <td>{Number(row.reserved ?? 0)}</td>
                      <td>{Number(row.on_hand ?? 0)}</td>
                      <td>
                        <button type="button" onClick={() => { setAdjusting(row); setQuantity(String(row.on_hand ?? 0)); }}>Adjust</button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      {adjusting ? (
        <Card>
          <h3>Update quantity</h3>
          <p className="muted">This replaces the on-hand quantity. Reserved items stay reserved until those orders finish.</p>
          <label>
            New on-hand quantity
            <input type="number" min="0" value={quantity} onChange={(event) => setQuantity(event.target.value)} />
          </label>
          <div className="page-actions" style={{ marginTop: 12 }}>
            <ConfirmButton label="Save quantity" confirm={`Set on-hand quantity to ${quantity}?`} onConfirm={() => void saveAdjustment()} />
            <button type="button" className="ghost" onClick={() => setAdjusting(null)}>Cancel</button>
          </div>
        </Card>
      ) : null}
    </Page>
  );
}
