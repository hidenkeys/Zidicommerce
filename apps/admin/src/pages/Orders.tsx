import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiGet, apiPost } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Button, Card, ConfirmButton, Drawer, EmptyState, FilterBar, Flash, LoadingState, Page, SearchField, Tabs } from "../components/ui";
import { fulfilmentLabel, humanStatus, money, relativeTime, type Row } from "../lib/format";
import { hasPermission } from "../lib/permissions";

type OrderAction = Row & { key: string; label: string; resource_type: string; target_status: string };
type OrderOperations = Row & {
  order: Row;
  payment?: Row;
  fulfilment?: Row;
  events: Row[];
  conversation_id?: string;
  next_actions: OrderAction[];
};

function statusTone(status: string): "success" | "warning" | "danger" | "info" | "neutral" {
  if (["paid", "completed"].includes(status)) return "success";
  if (["awaiting_payment", "processing", "ready", "out_for_delivery"].includes(status)) return "warning";
  if (["cancelled", "refunded"].includes(status)) return "danger";
  return "neutral";
}

export function OrdersPage() {
  const { user } = useAuth();
  const canManageOrders = hasPermission(user.role, "orders.manage");
  const canCancelOrders = hasPermission(user.role, "orders.cancel");
  const [params] = useSearchParams();
  const [rows, setRows] = useState<Row[]>([]);
  const [selectedID, setSelectedID] = useState(params.get("open") ?? "");
  const [detail, setDetail] = useState<OrderOperations | null>(null);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [detailRevision, setDetailRevision] = useState(0);
  const [transitioning, setTransitioning] = useState(false);

  async function load() {
    try {
      const response = await apiGet<Row[]>("/orders");
      setRows(response.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load orders");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    if (!selectedID) return;
    let current = true;
    setLoadingDetail(true);
    setDetail(null);
    apiGet<OrderOperations>(`/orders/${selectedID}/operations`).then(response => {
      if (current) setDetail(response.data);
    }).catch(error => {
      if (current) setMessage(error instanceof Error ? error.message : "Could not load order details");
    }).finally(() => { if (current) setLoadingDetail(false); });
    return () => { current = false; };
  }, [selectedID, detailRevision]);

  async function transition(nextStatus: string) {
    if (!detail || detail.order.id !== selectedID || transitioning) return;
    setTransitioning(true);
    try {
      await apiPost<Row>(`/orders/${selectedID}/transition`, { status: nextStatus, idempotency_key: `admin-${Date.now()}` });
      setFlash("Order updated.");
      await load();
      setDetailRevision(n => n+1);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "The order could not be updated");
    } finally {
      setTransitioning(false);
    }
  }

  const filtered = useMemo(() => {
    return rows.filter((order) => {
      const customer = (order.customer ?? {}) as Row;
      const haystack = `${order.order_number} ${customer.name} ${customer.phone}`.toLowerCase();
      const matchesQuery = !query || haystack.includes(query.toLowerCase());
      const orderStatus = String(order.status);
      const matchesStatus = status === "all" || (status === "attention" ? ["paid", "processing", "ready", "out_for_delivery"].includes(orderStatus) : orderStatus === status);
      return matchesQuery && matchesStatus;
    });
  }, [rows, query, status]);

  const currentDetail = detail?.order.id === selectedID ? detail : null;
  const selected = currentDetail?.order ?? rows.find((row) => row.id === selectedID) ?? null;
  const action = currentDetail?.next_actions.find((item) => Boolean(item.target_status)) ?? null;
  const customer = ((selected?.customer ?? {}) as Row);
  const items = Array.isArray(selected?.items) ? (selected.items as Row[]) : [];
  const store = ((selected?.store ?? {}) as Row);
  const events = currentDetail?.events ?? [];

  return (
    <Page title="Orders" description="Manage payment, preparation and collection in one place.">
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search order, name, or phone" />
      </FilterBar>
      <Tabs label="Order status" value={status} onChange={setStatus} items={[
        { value: "all", label: "All", count: rows.length },
        { value: "attention", label: "Needs action", count: rows.filter((order) => ["paid", "processing", "ready", "out_for_delivery"].includes(String(order.status))).length },
        { value: "awaiting_payment", label: "Awaiting payment", count: rows.filter((order) => order.status === "awaiting_payment").length },
        { value: "completed", label: "Completed" },
        { value: "cancelled", label: "Cancelled" },
      ]} />
      <div className="orders-workspace">
        <Card className="orders-register">
          {loading ? <LoadingState label="Loading orders" /> : filtered.length === 0 ? (
            <EmptyState title={rows.length ? "No matching orders" : "No orders yet"} body={rows.length ? "Try another search or status." : "Add products to your catalogue so customers can place their first order."} action={!rows.length && hasPermission(user.role, "catalogue.view") ? <Link className="button" to="/catalogue">Open catalogue</Link> : undefined} />
          ) : (
            <div className="table-wrap" style={{ border: 0, margin: 0 }}>
              <table className="order-list-table">
                <thead>
                  <tr>
                    <th>Order</th>
                    <th>Customer</th>
                    <th>Status</th>
                    <th>Total</th>
                    <th>Store</th>
                    <th>Placed</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((order) => {
                    const person = (order.customer ?? {}) as Row;
                    return (
                      <tr key={String(order.id)} className={selectedID === order.id ? "selected-row click-row" : "click-row"} onClick={() => setSelectedID(String(order.id))}>
                        <td><button type="button" className="table-record-link" aria-label={`Open order ${order.order_number}`}>{String(order.order_number)}</button></td>
                        <td>{String(person.name || person.phone || "Customer")}</td>
                        <td><Badge tone={statusTone(String(order.status))}>{humanStatus(order.status)}</Badge></td>
                        <td>{money(order.total_minor, String(order.currency ?? "NGN"))}</td>
                        <td>{String(((order.store ?? {}) as Row).name || "—")}</td>
                        <td>{relativeTime(order.created_at)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>
        <Drawer open={Boolean(selectedID)} title="Order details" onClose={() => setSelectedID("")}>
          <Flash message={message} />
          {loadingDetail ? <LoadingState label="Loading order details" /> : !selected ? (
            <EmptyState title="Select an order" body="Choose a row to see items, payment, and fulfilment." />
          ) : (
            <>
              <div className="order-detail-head"><div><span className="section-kicker">Order</span><h2>{String(selected.order_number)}</h2><p>{relativeTime(selected.created_at)}</p></div><div><Badge tone={statusTone(String(selected.status))}>{humanStatus(selected.status)}</Badge><strong>{money(selected.total_minor, String(selected.currency ?? "NGN"))}</strong></div></div>
              {selected.status === "awaiting_payment" ? <div className="order-wait-state"><Badge tone="warning">Waiting</Badge><span>Zidi will only mark this paid after provider verification.</span></div> : null}
              <div className="order-facts">
                <div><span>Customer</span><strong>{String(customer.name || "Customer")}</strong><small>{String(customer.phone || "No phone")}</small></div>
                <div><span>Store</span><strong>{String(store.name || "Store")}</strong><small>{String(store.address || "")}</small></div>
                <div><span>Payment</span><strong>{currentDetail?.payment ? humanStatus(currentDetail.payment.status) : "Not initialized"}</strong><small>{currentDetail?.payment ? String(currentDetail.payment.provider || "") : ""}</small></div>
                <div><span>Fulfilment</span><strong>{fulfilmentLabel(String(currentDetail?.fulfilment?.type ?? selected.fulfilment_type ?? ""))}</strong><small>{humanStatus(currentDetail?.fulfilment?.status ?? "pending")}</small></div>
              </div>
              <div className="panel-heading order-items-heading"><div><h3>Items</h3><p>{items.length} line item{items.length === 1 ? "" : "s"}</p></div>{currentDetail?.conversation_id ? <Link to={`/conversations?open=${currentDetail.conversation_id}`}>Open conversation</Link> : null}</div>
              <div className="order-items">
                {items.map((item, index) => <div key={String(item.id ?? index)}><span><strong>{String(item.product_name)}</strong><small>{item.variant_name && item.variant_name !== "Regular" ? String(item.variant_name) : "Regular"}</small></span><span>{Number(item.quantity)} x {money(item.unit_price_minor, String(selected.currency ?? "NGN"))}</span><strong>{money(item.total_minor, String(selected.currency ?? "NGN"))}</strong></div>)}
              </div>
              <div className="operations-timeline">
                <h3>Order activity</h3>
                {events.length === 0 ? <p className="muted">No commerce events recorded yet.</p> : events.map((event) => (
                  <div key={String(event.id)}>
                    <span className="event-marker" aria-hidden="true" />
                    <div><strong>{humanStatus(event.event_type)}</strong><small>{humanStatus(event.source)} · {relativeTime(event.created_at)}</small></div>
                  </div>
                ))}
              </div>
              <div className="page-actions">
                {action && canManageOrders ? <Button type="button" className="primary" loading={transitioning} onClick={() => void transition(action.target_status)}>{action.label}</Button> : null}
                {currentDetail && selected.status === "awaiting_payment" && canCancelOrders ? (
                  <ConfirmButton tone="danger" label="Cancel order" confirm="Cancel this unpaid order? Reserved inventory will be released." onConfirm={() => void transition("cancelled")} />
                ) : null}
              </div>
              {!canManageOrders && !["completed", "cancelled"].includes(String(selected.status)) ? <p className="read-only-note">Viewing only. Operational actions are hidden for this role.</p> : null}
            </>
          )}
        </Drawer>
      </div>
    </Page>
  );
}
