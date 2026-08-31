import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Card, EmptyState, Flash, LoadingState, Metric, Page, SectionHeader, SetupItem, StatusDot } from "../components/ui";
import { humanStatus, isToday, money, relativeTime, setupHref, type Row } from "../lib/format";
import { hasPermission, isOrganizationAdmin } from "../lib/permissions";
import { merchantReadinessItems, type SetupStatus } from "../lib/readiness";
import { isUsableChannelStatus } from "../lib/channels";

type Organization = Row & { name?: string; currency?: string; metadata?: string };
type Channel = Row & { provider: string; display_name: string; display_number?: string; status: string };
type PaymentConfiguration = Row & { provider: string; display_name: string; status: string; enabled: boolean };
type Bot = Row & { name: string; status: string; published_version_id?: string };

export function OverviewPage() {
  const { user } = useAuth();
  const canSeeInventory = hasPermission(user.role, "inventory.view");
  const canSeeChannels = hasPermission(user.role, "channels.view");
  const canSeePayments = hasPermission(user.role, "payments.view");
  const canSeeBot = hasPermission(user.role, "bot.view");
  const canManageBot = hasPermission(user.role, "bot.manage");
  const canManageSetup = isOrganizationAdmin(user.role);
  const [org, setOrg] = useState<Organization | null>(null);
  const [orders, setOrders] = useState<Row[]>([]);
  const [conversations, setConversations] = useState<Row[]>([]);
  const [inventory, setInventory] = useState<Row[]>([]);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [payments, setPayments] = useState<PaymentConfiguration[]>([]);
  const [bots, setBots] = useState<Bot[]>([]);
  const [fieldOverview, setFieldOverview] = useState<Row | null>(null);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      let partialFailure = false;
      async function safeGet<T>(enabled: boolean, path: string, fallback: T): Promise<T> {
        if (!enabled) return fallback;
        try {
          return (await apiGet<T>(path)).data;
        } catch {
          partialFailure = true;
          return fallback;
        }
      }
      try {
        const orgResponse = await apiGet<Organization>("/organizations/current");
        setOrg(orgResponse.data);
        const metadata = String(orgResponse.data.metadata ?? "").toLowerCase();
        const fieldService = metadata.includes("field_service") || metadata.includes("handyman");
        if (fieldService) {
          const [fieldResponse, channelResponse, paymentResponse, botResponse] = await Promise.all([
            safeGet(true, "/field/overview", {} as Row),
            safeGet(canSeeChannels, "/channels", [] as Channel[]),
            safeGet(canSeePayments, "/payment-configurations", [] as PaymentConfiguration[]),
            safeGet(canSeeBot, "/bots", [] as Bot[]),
          ]);
          setFieldOverview(fieldResponse);
          setChannels(channelResponse);
          setPayments(paymentResponse);
          setBots(botResponse);
          if (partialFailure) setMessage("Some overview information is temporarily unavailable. The available sections are current.");
          return;
        }
        const [orderResponse, conversationResponse, inventoryResponse, setupResponse, channelResponse, paymentResponse, botResponse] = await Promise.all([
          safeGet(hasPermission(user.role, "orders.view"), "/orders", [] as Row[]),
          safeGet(hasPermission(user.role, "conversations.view"), "/runtime/conversations", [] as Row[]),
          safeGet(canSeeInventory, "/inventory", [] as Row[]),
          safeGet(canManageSetup, "/bot-setup/status", null as SetupStatus | null),
          safeGet(canSeeChannels, "/channels", [] as Channel[]),
          safeGet(canSeePayments, "/payment-configurations", [] as PaymentConfiguration[]),
          safeGet(canSeeBot, "/bots", [] as Bot[]),
        ]);
        setOrders(orderResponse);
        setConversations(conversationResponse);
        setInventory(inventoryResponse);
        setSetupStatus(setupResponse);
        setChannels(channelResponse);
        setPayments(paymentResponse);
        setBots(botResponse);
        if (partialFailure) setMessage("Some overview information is temporarily unavailable. The available sections are current.");
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load overview");
      } finally {
        setLoading(false);
      }
    }
    void load();
  }, [user.role]);

  const todayOrders = orders.filter((order) => isToday(order.created_at));
  const todayRevenue = todayOrders
    .filter((order) => !["cancelled", "awaiting_payment"].includes(String(order.status)))
    .reduce((sum, order) => sum + Number(order.total_minor ?? 0), 0);
  const awaitingPrep = orders.filter((order) => ["paid", "processing"].includes(String(order.status))).length;
  const awaitingFulfilment = orders.filter((order) => ["ready", "out_for_delivery"].includes(String(order.status))).length;
  const awaitingPayment = orders.filter((order) => order.status === "awaiting_payment").length;
  const waitingConversations = conversations.filter((conversation) => ["human_requested", "human_assigned"].includes(String(conversation.conversation_status)) || conversation.handoff_status === "open" || conversation.handoff_status === "assigned" || conversation.status === "handoff").length;
  const unreadConversations = conversations.reduce((sum, conversation) => sum + Number(conversation.unread_count ?? 0), 0);
  const lowStock = inventory.filter((row) => Number(row.on_hand ?? 0) - Number(row.reserved ?? 0) <= Number(row.reorder_threshold ?? 0));
  const customerChannel = channels.find((channel) => isUsableChannelStatus(channel.status)) ?? channels[0];
  const paymentConfiguration = payments.find((row) => row.enabled && row.status === "active") ?? payments[0];
  const bot = bots.find((row) => row.published_version_id) ?? bots[0];
  const currency = String(org?.currency ?? todayOrders[0]?.currency ?? "NGN");
  const merchantItems = merchantReadinessItems(setupStatus);
  const requiredItems = merchantItems.filter((item) => item.required);
  const setupDone = requiredItems.filter((item) => item.complete).length;
  const attentionClear = awaitingPayment === 0 && awaitingPrep === 0 && awaitingFulfilment === 0 && waitingConversations === 0 && lowStock.length === 0
    && (!canSeeChannels || isUsableChannelStatus(customerChannel?.status))
    && (!canSeePayments || Boolean(paymentConfiguration?.enabled && paymentConfiguration.status === "active"))
    && (!canManageBot || Boolean(bot?.published_version_id));
  const description = user.role === "store_staff"
    ? "Assigned-store orders, fulfilment, stock, and customer conversations."
    : user.role === "support_agent"
      ? "Customers and conversations that need support, with linked order context."
      : user.role === "viewer"
        ? "A read-only view of current business operations."
        : "What needs attention now, followed by today's business activity.";
  const fieldPortalURL = String(import.meta.env.VITE_FIELD_PORTAL_URL ?? "").replace(/\/$/, "");

  if (loading) {
    return (
      <Page title={org?.name || "Overview"}>
        <LoadingState label="Loading your day" />
      </Page>
    );
  }

  if (fieldOverview) {
    return (
      <Page title={org?.name || "Field service"} description="Requests, professionals, payments, and customer conversations at a glance.">
        <Flash message={message} />
        <div className="metrics">
          <Metric label="Requests today" value={Number(fieldOverview.todays_requests ?? 0)} />
          <Metric label="Active jobs" value={Number(fieldOverview.active_jobs ?? 0)} />
          <Metric label="Available handymen" value={Number(fieldOverview.available_providers ?? 0)} />
          <Metric label="Completed jobs" value={Number(fieldOverview.completed_jobs ?? 0)} />
          <Metric label="Booking fees" value={money(fieldOverview.booking_fees_minor, String(org?.currency ?? "NGN"))} />
          <Metric label="Total revenue" value={money(fieldOverview.revenue_minor, String(org?.currency ?? "NGN"))} />
        </div>
        <div className="split">
          <Card>
            <h3>Field operations</h3>
            <p>Manage handymen, service categories, jobs, quotes, and conversations in the operations workspace.</p>
            {fieldPortalURL ? <a className="button primary" href={`${fieldPortalURL}/owner`}>Open field operations</a> : <p className="muted">The field operations URL has not been configured.</p>}
          </Card>
          <Card>
            <h3>Customer assistant</h3>
            <p><StatusDot live={Boolean(bot?.published_version_id) && bot?.status === "active"} label={bot?.published_version_id ? "Live" : "Not published"} /></p>
            <p><strong>{bot?.name || "No assistant yet"}</strong></p>
            <p className="muted">{isUsableChannelStatus(customerChannel?.status) ? `Customer channel ${customerChannel?.display_number || "connected"}` : "Customer channel not connected"}</p>
            <p className="muted">{paymentConfiguration?.enabled ? "Payments connected" : "Using platform payment settings"}</p>
            <div className="page-actions"><Link to="/assistant">Open assistant</Link><Link to="/settings/whatsapp">Channels</Link><Link to="/settings/payments">Payments</Link></div>
          </Card>
        </div>
      </Page>
    );
  }

  return (
    <Page
      title={org?.name ? `${org.name}` : "Overview"}
      description={description}
    >
      <Flash message={message} />
      <section className="attention-queue" aria-labelledby="attention-title">
        <div>
          <span className="section-kicker">Priority</span>
          <h2 id="attention-title">Needs attention</h2>
          <p>Work that may be blocking a customer or store.</p>
        </div>
        {attentionClear ? (
          <div className="attention-clear"><StatusDot live label="All caught up" /><span>New orders and conversations will appear here.</span></div>
        ) : (
          <ul className="attention-links">
            {awaitingPrep > 0 ? <li><Badge tone="warning">Orders</Badge><Link to="/orders">{awaitingPrep} paid order{awaitingPrep === 1 ? "" : "s"} waiting to be prepared</Link></li> : null}
            {awaitingFulfilment > 0 ? <li><Badge tone="info">Fulfilment</Badge><Link to="/orders">{awaitingFulfilment} order{awaitingFulfilment === 1 ? "" : "s"} ready or in transit</Link></li> : null}
            {awaitingPayment > 0 ? <li><Badge tone="neutral">Payment</Badge><Link to="/orders">{awaitingPayment} order{awaitingPayment === 1 ? " is" : "s are"} waiting for verified payment</Link></li> : null}
            {waitingConversations > 0 ? <li><Badge tone="danger">Customers</Badge><Link to="/conversations">{waitingConversations} conversation{waitingConversations === 1 ? "" : "s"} waiting for a person</Link></li> : null}
            {canSeeInventory && lowStock.length > 0 ? <li><Badge tone="warning">Stock</Badge><Link to="/inventory">{lowStock.length} product{lowStock.length === 1 ? "" : "s"} low or out of stock</Link></li> : null}
            {canSeeChannels && !isUsableChannelStatus(customerChannel?.status) ? <li><Badge tone="neutral">Channel</Badge><Link to="/settings/whatsapp">Customer messaging is not connected</Link></li> : null}
            {canSeePayments && !(paymentConfiguration?.enabled && paymentConfiguration.status === "active") ? <li><Badge tone="neutral">Payments</Badge><Link to="/settings/payments">Payment collection is not ready</Link></li> : null}
            {canManageBot && !bot?.published_version_id ? <li><Badge tone="neutral">Assistant</Badge><Link to="/assistant">Review and publish the customer assistant</Link></li> : null}
            {canManageSetup && setupStatus && !setupStatus.ready ? <li><Badge tone="warning">Setup</Badge><Link to="/setup">Complete required business readiness steps</Link></li> : null}
          </ul>
        )}
      </section>

      <SectionHeader title="Today" description="Live totals from orders, payments, fulfilment, and conversations." />
      <div className="metrics">
        <Metric label="Orders" value={todayOrders.length} to="/orders" hint={todayOrders.length === 0 ? "No orders yet" : "Created today"} />
        <Metric label="Confirmed revenue" value={money(todayRevenue, currency)} to="/orders" hint="Excludes unpaid and cancelled" />
        <Metric label="Pending fulfilment" value={awaitingPrep + awaitingFulfilment} to="/orders" hint="Paid through in transit" />
        <Metric label="Unread messages" value={unreadConversations} to="/conversations" hint={waitingConversations ? `${waitingConversations} need a person` : "Inbox is covered"} />
      </div>

      <div className="overview-operations">
        <Card className="recent-orders-panel">
          <div className="panel-heading"><div><h3>Recent orders</h3><p>Latest activity across permitted stores.</p></div><Link to="/orders">View all orders</Link></div>
          {orders.length === 0 ? (
            <EmptyState title="No orders yet" body="Orders will appear here after customers complete checkout." action={canSeeBot ? <Link to="/assistant">Review your assistant</Link> : undefined} />
          ) : (
            <div className="table-wrap flush-table">
              <table>
                <thead><tr><th>Order</th><th>Customer</th><th>Total</th><th>Status</th><th>When</th></tr></thead>
                <tbody>
                  {orders.slice(0, 8).map((order) => {
                    const customer = (order.customer ?? {}) as Row;
                    const status = String(order.status);
                    return <tr key={String(order.id)}><td><Link to={`/orders?open=${String(order.id)}`}>{String(order.order_number ?? "Order")}</Link></td><td>{String(customer.name || customer.phone || "Customer")}</td><td>{money(order.total_minor, String(order.currency ?? currency))}</td><td><Badge tone={["paid", "completed"].includes(status) ? "success" : status === "cancelled" ? "danger" : "warning"}>{humanStatus(status)}</Badge></td><td>{relativeTime(order.created_at)}</td></tr>;
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>
        <div className="operations-stack">
          <Card>
            <div className="panel-heading"><div><h3>Customer operations</h3><p>Current service readiness.</p></div></div>
            <div className="health-list">
              <div><span>Human handoffs</span><StatusDot live={waitingConversations === 0} label={waitingConversations === 0 ? "Covered" : `${waitingConversations} need attention`} /></div>
              {canSeeBot ? <div><span>Assistant</span><StatusDot live={Boolean(bot?.published_version_id) && bot?.status === "active"} label={bot?.published_version_id ? "Live" : "Not published"} /></div> : null}
              {canSeeChannels ? <div><span>Customer channel</span><StatusDot live={isUsableChannelStatus(customerChannel?.status)} label={isUsableChannelStatus(customerChannel?.status) ? "Connected" : "Not connected"} /></div> : null}
              {canSeePayments ? <div><span>Payments</span><StatusDot live={Boolean(paymentConfiguration?.enabled)} label={paymentConfiguration?.enabled ? "Ready" : "Not ready"} /></div> : null}
            </div>
            <div className="page-actions compact-actions">{canSeeBot ? <Link className="button" to="/assistant">Assistant</Link> : null}<Link className="button" to="/conversations">Inbox</Link></div>
          </Card>
          {canManageSetup && setupStatus && !setupStatus.ready ? (
            <Card>
              <div className="panel-heading"><div><h3>Setup progress</h3><p>{setupDone} of {requiredItems.length} required steps ready.</p></div><Link to="/setup">Continue</Link></div>
              <div className="progress-bar"><span style={{ width: `${requiredItems.length ? (setupDone / requiredItems.length) * 100 : 0}%` }} /></div>
              <div className="setup-list compact-setup">
                {requiredItems.filter((item) => !item.complete).slice(0, 3).map((item) => <SetupItem key={item.key} complete={false} label={item.label} to={setupHref(item.key)} description={item.description} />)}
              </div>
            </Card>
          ) : null}
        </div>
      </div>
    </Page>
  );
}
