import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, ArrowRight, Bot as BotIcon, Boxes, Check, ClipboardCheck, CreditCard, MessageSquareText, PackageCheck, ShoppingBag, Truck, Users, Wallet, type LucideIcon } from "lucide-react";
import { apiGet } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Card, EmptyState, Flash, LoadingState, Metric, MetricStrip, Page, StatusDot } from "../components/ui";
import { humanStatus, isToday, money, relativeTime, setupHref, type Row } from "../lib/format";
import { hasPermission, isOrganizationAdmin } from "../lib/permissions";
import { checklistProgress, merchantReadinessItems, type SetupStatus } from "../lib/readiness";
import { isUsableChannelStatus } from "../lib/channels";

type Organization = Row & { name?: string; currency?: string; metadata?: string };
type Channel = Row & { provider: string; display_name: string; display_number?: string; status: string };
type PaymentConfiguration = Row & { provider: string; display_name: string; status: string; enabled: boolean };
type Bot = Row & { name: string; status: string; published_version_id?: string };

type AttentionItem = { key: string; icon: LucideIcon; tag: string; tone: "warning" | "info" | "danger" | "neutral"; title: string; to: string; action: string; required?: boolean };

/** Splits a formatted amount so the currency symbol can sit small beside the figure. */
function moneyParts(minor: unknown, currency: string) {
  const formatted = money(minor, currency);
  const match = formatted.match(/^([^\d\s-]+)\s?(.*)$/);
  return match ? { symbol: match[1], amount: match[2] } : { symbol: "", amount: formatted };
}

function todayLabel() {
  try { return new Intl.DateTimeFormat("en-NG", { weekday: "long", day: "numeric", month: "long" }).format(new Date()); } catch { return "Today"; }
}

export function OverviewPage() {
  const { user } = useAuth();
  const canSeeInventory = hasPermission(user.role, "inventory.view");
  const canSeeChannels = hasPermission(user.role, "channels.view");
  const canSeePayments = hasPermission(user.role, "payments.view");
  const canSeeBot = hasPermission(user.role, "bot.view");
  const canManageBot = hasPermission(user.role, "bot.manage");
  const canManageSetup = isOrganizationAdmin(user.role);
  const [readinessCollapsed, setReadinessCollapsed] = useState(false);
  const [org, setOrg] = useState<Organization | null>(null);
  const [orders, setOrders] = useState<Row[]>([]);
  const [conversations, setConversations] = useState<Row[]>([]);
  const [customerCount, setCustomerCount] = useState<number>();
  const [storeCount, setStoreCount] = useState<number>();
  const [inventory, setInventory] = useState<Row[]>([]);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [payments, setPayments] = useState<PaymentConfiguration[]>([]);
  const [bots, setBots] = useState<Bot[]>([]);
  const [fieldOverview, setFieldOverview] = useState<Row | null>(null);
  const [message, setMessage] = useState("");
  const [reload,setReload] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      setMessage(""); setLoading(true);
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
          if (partialFailure) setMessage("Some overview data could not be loaded. Refresh to see current totals.");
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
        const [customerRows, storeRows] = await Promise.all([
          safeGet<Row[] | null>(hasPermission(user.role, "customers.view"), "/customers", null),
          safeGet<Row[] | null>(hasPermission(user.role, "stores.view"), "/stores", null),
        ]);
        setCustomerCount(customerRows?.length);
        setStoreCount(storeRows?.filter(store => store.status === "active").length);
        setOrders(orderResponse);
        setConversations(conversationResponse);
        setInventory(inventoryResponse);
        setSetupStatus(setupResponse);
        setChannels(channelResponse);
        setPayments(paymentResponse);
        setBots(botResponse);
        if (partialFailure) setMessage("Some overview data could not be loaded. Refresh to see current totals.");
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load overview");
      } finally {
        setLoading(false);
      }
    }
    void load();
  }, [user.role, user.organization_id, reload]);

  const todayOrders = orders.filter((order) => isToday(order.created_at));
  const todayRevenue = todayOrders
    .filter((order) => ["paid", "processing", "ready", "out_for_delivery", "completed"].includes(String(order.status)) && order.currency === (org?.currency || "NGN"))
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
  const readiness = checklistProgress(setupStatus);
  const attentionClear = !message && awaitingPayment === 0 && awaitingPrep === 0 && awaitingFulfilment === 0 && waitingConversations === 0 && lowStock.length === 0
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
      <Page title={org?.name || "Overview"} eyebrow={todayLabel()}>
        <LoadingState label="Loading your day" />
      </Page>
    );
  }

  if (message) return <Page title={org?.name || "Overview"} eyebrow={todayLabel()}><Flash message={message}/><button onClick={()=>setReload(n=>n+1)}>Refresh overview</button></Page>;

  const assistantLive = Boolean(bot?.published_version_id) && bot?.status === "active";
  const channelLive = isUsableChannelStatus(customerChannel?.status);
  const paymentsLive = Boolean(paymentConfiguration?.enabled && paymentConfiguration.status === "active");

  if (fieldOverview) {
    const fieldCurrency = String(org?.currency ?? "NGN");
    const fees = moneyParts(fieldOverview.booking_fees_minor, fieldCurrency);
    const revenue = moneyParts(fieldOverview.revenue_minor, fieldCurrency);
    return (
      <Page title={org?.name || "Field service"} eyebrow={todayLabel()} description="Requests, professionals, payments, and customer conversations at a glance.">
        <Flash message={message} />
        <MetricStrip label="Field service today" className="metric-strip--six">
          <Metric icon={ClipboardCheck} tone="brand" label="Requests today" value={Number(fieldOverview.todays_requests ?? 0)} />
          <Metric icon={Truck} tone="info" label="Active jobs" value={Number(fieldOverview.active_jobs ?? 0)} />
          <Metric icon={Users} tone="success" label="Available handymen" value={Number(fieldOverview.available_providers ?? 0)} />
          <Metric icon={Check} tone="neutral" label="Completed jobs" value={Number(fieldOverview.completed_jobs ?? 0)} />
          <Metric icon={CreditCard} tone="neutral" label="Booking fees" prefix={fees.symbol} value={fees.amount} />
          <Metric icon={Wallet} tone="success" label="Total revenue" prefix={revenue.symbol} value={revenue.amount} />
        </MetricStrip>
        <div className="overview-operations">
          <Card className="overview-card">
            <div className="panel-heading"><div><h3>Field operations</h3><p>Manage handymen, service categories, jobs, quotes, and conversations in the operations workspace.</p></div></div>
            {fieldPortalURL ? <a className="button primary" href={`${fieldPortalURL}/owner`}>Open field operations</a> : <p className="muted">The field operations URL has not been configured.</p>}
          </Card>
          <AssistantCard name={bot?.name} live={assistantLive} published={Boolean(bot?.published_version_id)} channel={channelLive ? `Customer channel ${customerChannel?.display_number || "connected"}` : "Customer channel not connected"} payments={paymentConfiguration?.enabled ? "Payments connected" : "Using platform payment settings"} />
        </div>
      </Page>
    );
  }

  const revenue = moneyParts(todayRevenue, currency);
  const steps = merchantReadinessItems(setupStatus);
  const attention: AttentionItem[] = [
    awaitingPrep > 0 ? { key: "prep", icon: PackageCheck, tag: "Orders", tone: "warning", title: `${awaitingPrep} paid order${awaitingPrep === 1 ? "" : "s"} waiting to be prepared`, to: "/orders", action: "Prepare orders" } : null,
    awaitingFulfilment > 0 ? { key: "fulfil", icon: Truck, tag: "Fulfilment", tone: "info", title: `${awaitingFulfilment} order${awaitingFulfilment === 1 ? "" : "s"} ready or in transit`, to: "/orders", action: "Track orders" } : null,
    awaitingPayment > 0 ? { key: "payment", icon: Wallet, tag: "Payment", tone: "neutral", title: `${awaitingPayment} order${awaitingPayment === 1 ? " is" : "s are"} waiting for verified payment`, to: "/orders", action: "Review payments" } : null,
    waitingConversations > 0 ? { key: "handoff", icon: MessageSquareText, tag: "Customers", tone: "danger", title: `${waitingConversations} conversation${waitingConversations === 1 ? "" : "s"} waiting for a person`, to: "/conversations", action: "Open inbox" } : null,
    canSeeInventory && lowStock.length > 0 ? { key: "stock", icon: Boxes, tag: "Stock", tone: "warning", title: `${lowStock.length} product${lowStock.length === 1 ? "" : "s"} low or out of stock`, to: "/inventory", action: "Restock" } : null,
    canSeeChannels && !channelLive ? { key: "channel", icon: MessageSquareText, tag: "Channel", tone: "neutral", title: "Customer messaging is not connected", to: "/settings/whatsapp", action: "Connect channel", required: true } : null,
    canSeePayments && !paymentsLive ? { key: "payments", icon: CreditCard, tag: "Payments", tone: "neutral", title: "Payment collection is not ready", to: "/settings/payments", action: "Set up payments", required: true } : null,
    canManageBot && !bot?.published_version_id ? { key: "assistant", icon: BotIcon, tag: "Assistant", tone: "neutral", title: "Review and publish the customer assistant", to: "/assistant", action: "Review assistant", required: true } : null,
    canManageSetup && setupStatus && !setupStatus.ready ? { key: "setup", icon: ClipboardCheck, tag: "Setup", tone: "warning", title: "Complete required business readiness steps", to: "/setup", action: "Open checklist", required: true } : null,
  ].filter((item): item is AttentionItem => Boolean(item));

  return (
    <Page
      title={org?.name ? `${org.name}` : "Overview"}
      eyebrow={todayLabel()}
      description={description}
    >
      <Flash message={message} />
      {canManageSetup && readiness.next && <section className={readinessCollapsed ? "onboarding-readiness setup-hero is-collapsed" : "onboarding-readiness setup-hero"} data-tour-region="readiness" aria-label="Store readiness">
        {readinessCollapsed ? <button type="button" className="setup-hero__expand" onClick={() => setReadinessCollapsed(false)}>Setup: {readiness.done} of {readiness.total} complete</button> : <>
          <div className="setup-hero__head">
            <div>
              <span className="setup-hero__eyebrow">Store setup · {readiness.done} of {readiness.total} complete</span>
              <h2>Get your store ready</h2>
              <p>Next: {readiness.next.label}. {readiness.next.description}</p>
            </div>
            <div className="setup-hero__actions">
              <Link className="button primary" to={setupHref(readiness.next.key)}>{readiness.next.label}<ArrowRight size={16} aria-hidden="true" /></Link>
              <Link className="button setup-hero__ghost" to="/setup">View setup checklist</Link>
              <button type="button" className="setup-hero__link" onClick={() => setReadinessCollapsed(true)}>Minimise setup</button>
            </div>
          </div>
          <progress className="sr-only" aria-label="Store readiness" value={readiness.done} max={readiness.total} />
          <ol className="setup-hero__steps">
            {steps.map((step, index) => (
              <li key={step.key} className={step.complete ? "done" : step.key === readiness.next?.key ? "current" : undefined}>
                <i aria-hidden="true" />
                <Link to={setupHref(step.key)}><span aria-hidden="true">{step.complete ? <Check size={12} /> : index + 1}</span>{step.label}<em className="sr-only">{step.complete ? " (complete)" : " (to do)"}</em></Link>
              </li>
            ))}
          </ol>
        </>}
      </section>}

      <div className="overview-section-head"><h2>Today</h2></div>
      <MetricStrip label="Today" className="overview-summary">
        <Metric icon={ShoppingBag} tone="brand" label="Orders" value={todayOrders.length} to="/orders" dot={todayOrders.length ? "success" : "neutral"} hint={todayOrders.length === 0 ? "No orders yet" : "Created today"} />
        <Metric icon={Wallet} tone="success" label="Confirmed revenue" prefix={revenue.symbol} value={revenue.amount} to="/orders" hint="Excludes unpaid and cancelled" />
        <Metric icon={Truck} tone="info" label="Pending fulfilment" value={awaitingPrep + awaitingFulfilment} to="/orders" dot={awaitingPrep + awaitingFulfilment ? "warning" : "neutral"} hint="Paid through in transit" />
        <Metric icon={MessageSquareText} tone="neutral" label="Unread messages" value={unreadConversations} to="/conversations" dot={waitingConversations ? "danger" : "success"} hint={waitingConversations ? `${waitingConversations} need a person` : "Inbox is covered"} />
      </MetricStrip>

      <section data-tour-region="attention" className="attention-queue" aria-labelledby="attention-title">
        <div className="attention-queue__head">
          <div>
            <h2 id="attention-title">Needs attention{attention.length ? <span className="count-pill">{attention.length}</span> : null}</h2>
            <p>Keep orders moving and customers informed.</p>
          </div>
        </div>
        {attentionClear || attention.length === 0 ? (
          <div className="attention-clear"><StatusDot live label="All caught up" /><span>New orders and conversations will appear here.</span></div>
        ) : (
          <ol className="attention-links">
            {attention.map((item, index) => (
              <li key={item.key}>
                <span className={`attention-tile tone-${item.tone}`} aria-hidden="true"><item.icon size={18} /><b>{index + 1}</b></span>
                <span className="attention-text">
                  <span className="attention-tags"><Badge tone={item.tone}>{item.tag}</Badge>{item.required ? <span className="required-tag"><AlertTriangle size={12} aria-hidden="true" />Required</span> : null}</span>
                  <Link to={item.to}>{item.title}</Link>
                </span>
                <Link className={index === 0 ? "button primary attention-action" : "button attention-action"} to={item.to} tabIndex={-1} aria-hidden="true">{item.action}</Link>
              </li>
            ))}
          </ol>
        )}
      </section>

      <div className="overview-operations">
        <Card className="recent-orders-panel">
          <div className="panel-heading"><div><h3>Recent orders</h3><p>Latest activity across permitted stores.</p></div><Link to="/orders">View all orders</Link></div>
          {orders.length === 0 ? (
            <EmptyState icon={ShoppingBag} title="No orders yet" body="Orders will appear here after customers complete checkout." action={canSeeBot ? <Link className="button" to="/assistant">Review your assistant</Link> : undefined} />
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
          <Card className="overview-card">
            <div className="panel-heading"><div><h3>Customer operations</h3><p>Current service readiness.</p></div></div>
            <div className="ops-tiles">
              {customerCount !== undefined && <Link to="/customers"><span>Customers</span><strong>{customerCount}</strong></Link>}
              {storeCount !== undefined && <Link to="/stores"><span>Active stores</span><strong>{storeCount}</strong></Link>}
              {hasPermission(user.role,"conversations.view") && <Link to="/conversations"><span>AI handling</span><strong>{conversations.filter(c => c.conversation_status === "ai_handling").length}</strong></Link>}
            </div>
            <div className="health-list">
              <div><span>Human handoffs</span><StatusDot live={waitingConversations === 0} label={waitingConversations === 0 ? "Covered" : `${waitingConversations} need attention`} /></div>
              {canSeeBot ? <div><span>Assistant</span><StatusDot live={assistantLive} label={bot?.published_version_id ? "Live" : "Not published"} /></div> : null}
              {canSeeChannels ? <div><span>Customer channel</span><StatusDot live={channelLive} label={channelLive ? "Connected" : "Not connected"} /></div> : null}
              {canSeePayments ? <div><span>Payments</span><StatusDot live={Boolean(paymentConfiguration?.enabled)} label={paymentConfiguration?.enabled ? "Ready" : "Not ready"} /></div> : null}
            </div>
            <div className="page-actions compact-actions">{canSeeBot ? <Link className="button" to="/journeys">Flow Studio</Link> : null}<Link className="button" to="/conversations">Inbox</Link></div>
          </Card>
          {canSeeBot ? <AssistantCard name={bot?.name} live={assistantLive} published={Boolean(bot?.published_version_id)} channel={canSeeChannels ? (channelLive ? `Customer channel ${customerChannel?.display_number || "connected"}` : "Customer channel not connected") : undefined} /> : null}
        </div>
      </div>
    </Page>
  );
}

function AssistantCard({ name, live, published, channel, payments }: { name?: string; live: boolean; published: boolean; channel?: string; payments?: string }) {
  return (
    <article className="assistant-card">
      <div className="assistant-card__head">
        <span className="assistant-card__icon" aria-hidden="true"><BotIcon size={18} /></span>
        <div><span>Your assistant</span><strong>{name || "No assistant yet"}</strong></div>
      </div>
      <StatusDot live={live} label={published ? (live ? "Live" : "Published, paused") : "Not published"} />
      {channel ? <p>{channel}</p> : null}
      {payments ? <p>{payments}</p> : null}
      <div className="assistant-card__actions">
        <Link className="button primary" to="/assistant">Open assistant</Link>
        <Link to="/settings/whatsapp">Channels</Link>
      </div>
    </article>
  );
}
