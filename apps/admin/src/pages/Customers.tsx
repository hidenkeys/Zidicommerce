import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet } from "../api/client";
import { Card, EmptyState, FilterBar, Flash, Page, SearchField } from "../components/ui";
import { humanStatus, money, relativeTime, type Row } from "../lib/format";

export function CustomersPage() {
  const [customers, setCustomers] = useState<Row[]>([]);
  const [orders, setOrders] = useState<Row[]>([]);
  const [tickets, setTickets] = useState<Row[]>([]);
  const [conversations, setConversations] = useState<Row[]>([]);
  const [query, setQuery] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [message, setMessage] = useState("");

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
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load customers");
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
          {filtered.length === 0 ? (
            <EmptyState title="No customers yet" body="Customers are created automatically when they message on WhatsApp." />
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
              <h3>{String(selected.name || "Customer")}</h3>
              <p>{String(selected.phone || "No phone")}</p>
              <p className="muted">{String(selected.email || "No email")}</p>
              <p>{String(selected.default_address || "No saved address")}</p>
              <p><strong>Total spent</strong><br />{money(spent)}</p>
              <p><strong>Last order</strong><br />{lastOrder ? `${lastOrder.order_number} · ${humanStatus(lastOrder.status)} · ${relativeTime(lastOrder.created_at)}` : "No orders yet"}</p>
              <h3>Orders</h3>
              {customerOrders.length === 0 ? <p className="muted">No orders yet.</p> : customerOrders.slice(0, 8).map((order) => (
                <p key={String(order.id)}><Link to={`/orders?open=${String(order.id)}`}>{String(order.order_number)}</Link> · {money(order.total_minor, String(order.currency ?? "NGN"))} · {humanStatus(order.status)}</p>
              ))}
              <h3>Support</h3>
              <p>{complaints.length} complaint{complaints.length === 1 ? "" : "s"} · {chats.length} conversation{chats.length === 1 ? "" : "s"}</p>
              {chats[0] ? <Link to="/conversations">Open conversations</Link> : null}
            </>
          )}
        </Card>
      </div>
    </Page>
  );
}
