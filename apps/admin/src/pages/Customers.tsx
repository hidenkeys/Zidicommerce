import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiGet } from "../api/client";
import { Badge, Card, EmptyState, FilterBar, Flash, LoadingState, Page, SearchField, SectionHeader } from "../components/ui";
import { humanStatus, money, relativeTime, type Row } from "../lib/format";

export function CustomersPage() {
  const [params] = useSearchParams();
  const [customers, setCustomers] = useState<Row[]>([]);
  const [orders, setOrders] = useState<Row[]>([]);
  const [tickets, setTickets] = useState<Row[]>([]);
  const [conversations, setConversations] = useState<Row[]>([]);
  const [query, setQuery] = useState("");
  const [selectedID, setSelectedID] = useState(params.get("open") ?? "");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [customerResponse, orderResponse, ticketResponse, conversationResponse] = await Promise.all([
          apiGet<Row[]>("/customers"),
          apiGet<Row[]>("/orders"),
          apiGet<Row[]>("/runtime/support-tickets").catch(() => ({ data: [] as Row[] })),
          apiGet<Row[]>("/runtime/conversations").catch(() => ({ data: [] as Row[] })),
        ]);
        setCustomers(customerResponse.data);
        setOrders(orderResponse.data);
        setTickets(ticketResponse.data);
        setConversations(conversationResponse.data);
        setSelectedID((current) => current || String(customerResponse.data[0]?.id ?? ""));
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load customers");
      } finally {
        setLoading(false);
      }
    }
    void load();
  }, []);

  const filtered = useMemo(() => {
    return customers.filter((customer) => {
      const haystack = `${customer.name} ${customer.phone} ${customer.email}`.toLowerCase();
      return !query || haystack.includes(query.toLowerCase());
    });
  }, [customers, query]);

  const selected = customers.find((customer) => customer.id === selectedID);
  const customerOrders = orders.filter((order) => String(order.customer_id) === selectedID || String((order.customer as Row | undefined)?.id) === selectedID);
  const spent = customerOrders
    .filter((order) => !["cancelled", "awaiting_payment"].includes(String(order.status)))
    .reduce((sum, order) => sum + Number(order.total_minor ?? 0), 0);
  const lastOrder = customerOrders[0];
  const complaints = tickets.filter((ticket) => String(ticket.customer_id) === selectedID);
  const chats = conversations.filter((conversation) => String(conversation.customer_id) === selectedID);

  return (
    <Page title="Customers" description="People who have ordered or messaged your assistant.">
      <Flash message={message} />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search name, phone, or email" />
      </FilterBar>
      <div className="detail-layout">
        <Card>
          {loading ? <LoadingState label="Loading customers" /> : filtered.length === 0 ? (
            <EmptyState title="No customers yet" body="Customers appear here when they order or start a conversation." />
          ) : (
            <div className="table-wrap" style={{ border: 0, margin: 0 }}>
              <table>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Phone</th>
                    <th>Orders</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((customer) => {
                    const count = orders.filter((order) => String(order.customer_id) === customer.id || String((order.customer as Row | undefined)?.id) === customer.id).length;
                    return (
                      <tr key={String(customer.id)} className={selectedID === customer.id ? "selected-row click-row" : "click-row"} onClick={() => setSelectedID(String(customer.id))}>
                        <td>{String(customer.name || "Customer")}</td>
                        <td>{String(customer.phone || "—")}</td>
                        <td>{count}</td>
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
            <EmptyState title="Select a customer" body="See orders, spend, and support history." />
          ) : (
            <>
              <SectionHeader title={String(selected.name || "Customer")} description="Customer profile and recent activity" />
              <div className="customer-facts">
                <div><span>Phone</span><strong>{String(selected.phone || "Not provided")}</strong></div>
                <div><span>Email</span><strong>{String(selected.email || "Not provided")}</strong></div>
                <div><span>Saved address</span><strong>{String(selected.default_address || "Not provided")}</strong></div>
                <div><span>Total spent</span><strong>{money(spent)}</strong></div>
              </div>
              <SectionHeader title="Recent orders" description={lastOrder ? `Last order ${relativeTime(lastOrder.created_at)}` : "No order history yet"} />
              {customerOrders.length === 0 ? <p className="muted">No orders yet.</p> : customerOrders.slice(0, 8).map((order) => (
                <div className="activity-row" key={String(order.id)}>
                  <span><Link to={`/orders?open=${String(order.id)}`}>{String(order.order_number)}</Link><small>{relativeTime(order.created_at)}</small></span>
                  <span>{money(order.total_minor, String(order.currency ?? "NGN"))} <Badge tone={String(order.status) === "cancelled" ? "danger" : String(order.status) === "completed" ? "success" : "warning"}>{humanStatus(order.status)}</Badge></span>
                </div>
              ))}
              <SectionHeader title="Support history" />
              <div className="customer-support-summary">
                <span><strong>{chats.length}</strong> conversation{chats.length === 1 ? "" : "s"}</span>
                <span><strong>{complaints.length}</strong> complaint{complaints.length === 1 ? "" : "s"}</span>
              </div>
              {chats[0] ? <Link to={`/conversations?open=${String(chats[0].id)}`}>Open latest conversation</Link> : <p className="muted">No support conversations yet.</p>}
            </>
          )}
        </Card>
      </div>
    </Page>
  );
}
