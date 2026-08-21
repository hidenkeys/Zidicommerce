import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet } from "../api/client";
import { Card, EmptyState, Flash, LoadingState, Metric, Page, SetupItem, StatusDot } from "../components/ui";
import { humanStatus, isToday, money, relativeTime, setupHref, type Row } from "../lib/format";

type Organization = Row & { name?: string; currency?: string };
type Channel = Row & { provider: string; display_name: string; display_number?: string; status: string };
type SetupStatus = { complete_count: number; total_count: number; ready: boolean; items: { key: string; label: string; complete: boolean; description: string }[] };
type PaymentConfiguration = Row & { provider: string; display_name: string; status: string; enabled: boolean };
type Bot = Row & { name: string; status: string; published_version_id?: string };

const merchantSetupKeys = ["stores", "catalogue", "inventory", "whatsapp", "payments", "bot", "faqs"];

export function OverviewPage() {
  const [org, setOrg] = useState<Organization | null>(null);
  const [orders, setOrders] = useState<Row[]>([]);
  const [conversations, setConversations] = useState<Row[]>([]);
  const [inventory, setInventory] = useState<Row[]>([]);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [payments, setPayments] = useState<PaymentConfiguration[]>([]);
  const [bots, setBots] = useState<Bot[]>([]);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [orgResponse, orderResponse, conversationResponse, inventoryResponse, setupResponse, channelResponse, paymentResponse, botResponse] = await Promise.all([
          apiGet<Organization>("/organizations/current"),
          apiGet<Row[]>("/orders"),
          apiGet<Row[]>("/runtime/conversations"),
          apiGet<Row[]>("/inventory"),
          apiGet<SetupStatus>("/bot-setup/status"),
          apiGet<Channel[]>("/channels"),
          apiGet<PaymentConfiguration[]>("/payment-configurations").catch(() => ({ data: [] as PaymentConfiguration[] })),
          apiGet<Bot[]>("/bots").catch(() => ({ data: [] as Bot[] })),
        ]);
        setOrg(orgResponse.data);
        setOrders(orderResponse.data);
        setConversations(conversationResponse.data);
        setInventory(inventoryResponse.data);
        setSetupStatus(setupResponse.data);
        setChannels(channelResponse.data);
        setPayments(paymentResponse.data);
        setBots(botResponse.data);
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load overview");
      } finally {
        setLoading(false);
      }
    }
    void load();
  }, []);

  const todayOrders = orders.filter((order) => isToday(order.created_at));
  const todayRevenue = todayOrders
    .filter((order) => !["cancelled", "awaiting_payment"].includes(String(order.status)))
    .reduce((sum, order) => sum + Number(order.total_minor ?? 0), 0);
  const awaitingPrep = orders.filter((order) => ["paid", "processing"].includes(String(order.status))).length;
  const awaitingFulfilment = orders.filter((order) => ["ready", "out_for_delivery"].includes(String(order.status))).length;
  const waitingConversations = conversations.filter((conversation) => conversation.handoff_status === "open" || conversation.handoff_status === "assigned" || conversation.status === "handoff").length;
  const lowStock = inventory.filter((row) => Number(row.on_hand ?? 0) - Number(row.reserved ?? 0) <= Number(row.reorder_threshold ?? 0));
  const whatsapp = channels.find((channel) => channel.provider === "whatsapp");
  const paystack = payments.find((row) => row.provider === "paystack");
  const bot = bots.find((row) => row.published_version_id) ?? bots[0];
  const currency = String(org?.currency ?? todayOrders[0]?.currency ?? "NGN");
  const merchantItems = (setupStatus?.items ?? []).filter((item) => merchantSetupKeys.includes(item.key));
  const setupDone = merchantItems.filter((item) => item.complete).length;

  if (loading) {
    return (
      <Page title={org?.name || "Overview"}>
        <LoadingState label="Loading your day" />
      </Page>
    );
  }

  return (
    <Page
      title={org?.name ? `${org.name}` : "Overview"}
      description="What needs attention, and how the business is doing today."
    >
      <Flash message={message} />
      <div className="metrics">
        <Metric label="Orders today" value={todayOrders.length} to="/orders" hint={todayOrders.length === 0 ? "No orders yet" : undefined} />
        <Metric label="Revenue today" value={money(todayRevenue, currency)} to="/orders" />
        <Metric label="Need preparing" value={awaitingPrep} to="/orders" hint={awaitingPrep ? "Paid orders waiting" : "None waiting"} />
        <Metric label="Out for fulfilment" value={awaitingFulfilment} to="/orders" />
      </div>

      <div className="split">
        <Card>
          <h3>Needs attention</h3>
          {awaitingPrep === 0 && waitingConversations === 0 && lowStock.length === 0 && whatsapp?.status === "active" && paystack?.enabled ? (
            <p className="muted">You are caught up. New orders and chats will appear here.</p>
          ) : (
            <ul className="attention-links">
              {awaitingPrep > 0 ? <li><Link to="/orders">{awaitingPrep} order{awaitingPrep === 1 ? "" : "s"} waiting to be prepared</Link></li> : null}
              {awaitingFulfilment > 0 ? <li><Link to="/orders">{awaitingFulfilment} order{awaitingFulfilment === 1 ? "" : "s"} being fulfilled</Link></li> : null}
              {waitingConversations > 0 ? <li><Link to="/conversations">{waitingConversations} conversation{waitingConversations === 1 ? "" : "s"} waiting for a person</Link></li> : null}
              {lowStock.length > 0 ? <li><Link to="/inventory">{lowStock.length} product{lowStock.length === 1 ? "" : "s"} low or out of stock</Link></li> : null}
              {whatsapp?.status !== "active" ? <li><Link to="/settings/whatsapp">Connect WhatsApp so customers can message you</Link></li> : null}
              {!paystack?.enabled ? <li><Link to="/settings/payments">Connect payments so customers can pay</Link></li> : null}
            </ul>
          )}
        </Card>
        <Card>
          <h3>Assistant</h3>
          <p><StatusDot live={Boolean(bot?.published_version_id) && bot?.status === "active"} label={bot?.published_version_id ? "Live" : "Not published"} /></p>
          <p><strong>{bot?.name || "No assistant yet"}</strong></p>
          <p className="muted">{whatsapp?.status === "active" ? `WhatsApp ${whatsapp.display_number || "connected"}` : "WhatsApp not connected"}</p>
          <p className="muted">{paystack?.enabled ? "Payments connected" : "Payments not connected"}</p>
          <div className="page-actions" style={{ marginTop: 16 }}>
            <Link to="/assistant"><button type="button" className="primary">Open assistant</button></Link>
            <Link to="/conversations"><button type="button">Conversations</button></Link>
          </div>
        </Card>
      </div>

      {setupStatus && !setupStatus.ready ? (
        <Card>
          <h3>Finish setting up your store</h3>
          <p className="muted">{setupDone} of {merchantItems.length} merchant steps complete</p>
          <div className="progress-bar"><span style={{ width: `${merchantItems.length ? (setupDone / merchantItems.length) * 100 : 0}%` }} /></div>
          <div className="setup-list">
            {merchantItems.map((item) => (
              <SetupItem key={item.key} complete={item.complete} label={item.label} to={setupHref(item.key)} description={item.complete ? undefined : item.description} />
            ))}
          </div>
        </Card>
      ) : null}

      <Card>
        <h3>Recent orders</h3>
        {orders.length === 0 ? (
          <EmptyState title="No orders yet" body="They will appear here as customers order on WhatsApp." action={<Link to="/assistant">Set up your assistant</Link>} />
        ) : (
          <div className="table-wrap" style={{ border: 0, margin: 0 }}>
            <table>
              <thead>
                <tr>
                  <th>Order</th>
                  <th>Customer</th>
                  <th>Total</th>
                  <th>Status</th>
                  <th>When</th>
                </tr>
              </thead>
              <tbody>
                {orders.slice(0, 8).map((order) => {
                  const customer = (order.customer ?? {}) as Row;
                  return (
                    <tr key={String(order.id)}>
                      <td><Link to={`/orders?open=${String(order.id)}`}>{String(order.order_number ?? "—")}</Link></td>
                      <td>{String(customer.name || customer.phone || "Customer")}</td>
                      <td>{money(order.total_minor, String(order.currency ?? currency))}</td>
                      <td>{humanStatus(order.status)}</td>
                      <td>{relativeTime(order.created_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </Page>
  );
}
