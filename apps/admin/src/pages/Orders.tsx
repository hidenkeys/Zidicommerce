import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { apiGet, apiPost } from "../api/client";
import { Badge, Card, EmptyState, FilterBar, Flash, Page, SearchField } from "../components/ui";
import { fulfilmentLabel, humanStatus, money, nextOrderAction, orderTimeline, relativeTime, type Row } from "../lib/format";

function statusTone(status: string): "success" | "warning" | "danger" | "info" | "neutral" {
  if (["paid", "completed"].includes(status)) return "success";
  if (["awaiting_payment", "processing", "ready", "out_for_delivery"].includes(status)) return "warning";
  if (["cancelled", "refunded"].includes(status)) return "danger";
  return "neutral";
}

export function OrdersPage() {
  const [params] = useSearchParams();
  const [rows, setRows] = useState<Row[]>([]);
  const [selectedID, setSelectedID] = useState(params.get("open") ?? "");
  const [detail, setDetail] = useState<Row | null>(null);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

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

  useEffect(() => {
    if (selectedID) void openOrder(selectedID);
  }, [selectedID]);

  async function transition(nextStatus: string) {
    if (!selectedID) return;
    await apiPost<Row>(`/orders/${selectedID}/transition`, { status: nextStatus, idempotency_key: `admin-${Date.now()}` });
    setFlash("Order updated.");
    await load();
    await openOrder(selectedID);
  }

  const filtered = useMemo(() => {
    return rows.filter((order) => {
      const customer = (order.customer ?? {}) as Row;
      const haystack = `${order.order_number} ${customer.name} ${customer.phone}`.toLowerCase();
      const matchesQuery = !query || haystack.includes(query.toLowerCase());
      const matchesStatus = status === "all" || String(order.status) === status;
      return matchesQuery && matchesStatus;
    });
  }, [rows, query, status]);

  const selected = detail ?? rows.find((row) => row.id === selectedID) ?? null;
  const action = selected ? nextOrderAction(String(selected.status)) : null;
  const customer = ((selected?.customer ?? {}) as Row);
  const items = Array.isArray(selected?.items) ? (selected.items as Row[]) : [];
  const store = ((selected?.store ?? {}) as Row);
  const timeline = selected ? orderTimeline(String(selected.status)) : [];

  return (
    <Page title="Orders" description="See what customers ordered and take the next step.">
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search order, name, or phone" />
        <select value={status} onChange={(event) => setStatus(event.target.value)}>
          <option value="all">All statuses</option>
          {["awaiting_payment", "paid", "processing", "ready", "out_for_delivery", "completed", "cancelled"].map((value) => (
            <option key={value} value={value}>{humanStatus(value)}</option>
          ))}
        </select>
      </FilterBar>
      <div className="detail-layout">
        <Card>
          {filtered.length === 0 ? (
            <EmptyState title="No orders yet" body="Orders from WhatsApp will show up here." />
          ) : (
            <div className="table-wrap" style={{ border: 0, margin: 0 }}>
              <table>
                <thead>
                  <tr>
                    <th>Order</th>
                    <th>Customer</th>
                    <th>Total</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((order) => {
                    const person = (order.customer ?? {}) as Row;
                    return (
                      <tr key={String(order.id)} className={selectedID === order.id ? "selected-row click-row" : "click-row"} onClick={() => void openOrder(String(order.id))}>
                        <td>{String(order.order_number)}</td>
                        <td>{String(person.name || person.phone || "Customer")}</td>
                        <td>{money(order.total_minor, String(order.currency ?? "NGN"))}</td>
                        <td><Badge tone={statusTone(String(order.status))}>{humanStatus(order.status)}</Badge></td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>
        <Card>
          {!selected ? (
            <EmptyState title="Select an order" body="Choose a row to see items, payment, and fulfilment." />
          ) : (
            <>
              <h3>{String(selected.order_number)}</h3>
              <p className="muted">{relativeTime(selected.created_at)}</p>
              <p><Badge tone={statusTone(String(selected.status))}>{humanStatus(selected.status)}</Badge></p>
              <p><strong>Customer</strong><br />{String(customer.name || "Customer")}<br />{String(customer.phone || "")}</p>
              <p><strong>Store</strong><br />{String(store.name || "—")}</p>
              <p><strong>Fulfilment</strong><br />{fulfilmentLabel(String(selected.fulfilment_type ?? ""))}</p>
              <p><strong>Payment</strong><br />{["paid", "processing", "ready", "out_for_delivery", "completed"].includes(String(selected.status)) ? "Paid" : humanStatus(selected.status)}</p>
              <ul className="item-list">
                {items.map((item, index) => (
                  <li key={String(item.id ?? index)}>
                    {String(item.product_name)} {item.variant_name && item.variant_name !== "Regular" ? `· ${item.variant_name}` : ""} ×{Number(item.quantity)} — {money(item.total_minor, String(selected.currency ?? "NGN"))}
                  </li>
                ))}
              </ul>
              <p><strong>Total {money(selected.total_minor, String(selected.currency ?? "NGN"))}</strong></p>
              <div className="timeline">
                {timeline.map((step) => (
                  <span key={step.key} className={step.current ? "current" : step.done ? "done" : ""}>
                    {step.done ? "✓" : "○"} {step.label}
                  </span>
                ))}
              </div>
              <div className="page-actions">
                {action ? <button type="button" className="primary" onClick={() => void transition(action.status)}>{action.label}</button> : null}
                {selected.status !== "cancelled" && selected.status !== "completed" ? (
                  <button type="button" className="danger" onClick={() => window.confirm("Cancel this order?") && void transition("cancelled")}>Cancel order</button>
                ) : null}
              </div>
            </>
          )}
        </Card>
      </div>
    </Page>
  );
}
