import { FormEvent, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet, apiPost } from "../api/client";
import { Badge, Card, EmptyState, FilterBar, Flash, Page, SearchField } from "../components/ui";
import { relativeTime, type Row } from "../lib/format";

function inboxStatus(row: Row) {
  if (row.status === "completed" || row.status === "cancelled") return "Resolved";
  if (row.status === "handoff" || row.handoff_status === "open" || row.handoff_status === "assigned") return "Waiting";
  return "Open";
}

function tone(status: string): "success" | "warning" | "info" {
  if (status === "Resolved") return "success";
  if (status === "Waiting") return "warning";
  return "info";
}

export function ConversationsPage() {
  const [rows, setRows] = useState<Row[]>([]);
  const [handoffs, setHandoffs] = useState<Row[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [messages, setMessages] = useState<Row[]>([]);
  const [reply, setReply] = useState("");
  const [note, setNote] = useState("");
  const [filter, setFilter] = useState("all");
  const [query, setQuery] = useState("");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

  async function load() {
    try {
      const [conversationResponse, handoffResponse] = await Promise.all([
        apiGet<Row[]>("/runtime/conversations"),
        apiGet<Row[]>("/runtime/support-handoffs"),
      ]);
      setRows(conversationResponse.data);
      setHandoffs(handoffResponse.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load conversations");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function openConversation(id: string) {
    setSelectedID(id);
    const response = await apiGet<Row[]>(`/runtime/conversations/${id}/messages`);
    setMessages(response.data);
  }

  const filtered = useMemo(() => {
    return rows.filter((row) => {
      const status = inboxStatus(row);
      const matchesFilter = filter === "all" || status === filter;
      const haystack = `${row.customer_name} ${row.customer_phone} ${row.last_message}`.toLowerCase();
      return matchesFilter && (!query || haystack.includes(query.toLowerCase()));
    });
  }, [rows, filter, query]);

  const selected = rows.find((row) => row.id === selectedID);
  const handoff = handoffs.find((item) => String(item.session_id) === selectedID);

  async function sendReply(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>(`/runtime/conversations/${selectedID}/reply`, { text: reply });
    setReply("");
    setFlash("Reply sent.");
    await openConversation(selectedID);
    await load();
  }

  async function claim() {
    if (!handoff) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/claim`, { note });
    setNote("");
    setFlash("Assigned to you.");
    await load();
  }

  async function addNote() {
    if (!handoff || !note.trim()) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/notes`, { note, internal: true });
    setNote("");
    setFlash("Note saved.");
  }

  async function resolve() {
    if (!handoff) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/resolve`, { resolution_note: note });
    setNote("");
    setFlash("Marked resolved.");
    await load();
  }

  async function reopen() {
    if (!handoff) return;
    await apiPost<Row>(`/runtime/support-handoffs/${handoff.id}/reopen`, {});
    setFlash("Reopened.");
    await load();
  }

  return (
    <Page title="Conversations" description="Your support inbox for customers who need a person." help="Open is still with the assistant. Waiting needs a teammate. Resolved is finished.">
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search customer or message" />
        <select value={filter} onChange={(event) => setFilter(event.target.value)}>
          <option value="all">All</option>
          <option value="Open">Open</option>
          <option value="Waiting">Waiting</option>
          <option value="Resolved">Resolved</option>
        </select>
        <button type="button" onClick={() => void load()}>Refresh</button>
      </FilterBar>
      <div className="inbox-layout">
        <Card>
          {filtered.length === 0 ? (
            <EmptyState title="No conversations yet" body="Chats appear here when customers message on WhatsApp." />
          ) : (
            <div className="table-wrap" style={{ border: 0, margin: 0 }}>
              <table>
                <thead>
                  <tr>
                    <th>Customer</th>
                    <th>Message</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((row) => (
                    <tr key={String(row.id)} className={selectedID === row.id ? "selected-row click-row" : "click-row"} onClick={() => void openConversation(String(row.id))}>
                      <td>{String(row.customer_name || row.customer_phone || "Customer")}</td>
                      <td>{String(row.last_message || "—").slice(0, 72)}</td>
                      <td><Badge tone={tone(inboxStatus(row))}>{inboxStatus(row)}</Badge></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
        <Card>
          {!selected ? (
            <EmptyState title="Select a conversation" body="Read the chat, reply, or pass it to a teammate." />
          ) : (
            <>
              <h3>{String(selected.customer_name || selected.customer_phone || "Customer")}</h3>
              <p className="muted">{inboxStatus(selected)}{handoff?.assigned_user_id ? " · Assigned" : ""} · {relativeTime(selected.updated_at)}</p>
              {selected.customer_id ? <p><Link to="/customers">View customer</Link></p> : null}
              <div className="thread">
                {messages.map((item) => (
                  <div key={String(item.id)} className={item.direction === "inbound" ? "bubble customer" : "bubble"}>
                    <span>{item.direction === "inbound" ? "Customer" : item.direction === "internal" ? "Note" : "Assistant"}</span>
                    <p>{String(item.body ?? item.text ?? "")}</p>
                  </div>
                ))}
              </div>
              <form className="form-grid" onSubmit={sendReply}>
                <label className="full">Reply<textarea value={reply} onChange={(event) => setReply(event.target.value)} required /></label>
                <div className="full"><button type="submit">Send reply</button></div>
              </form>
              <label>Internal note<input value={note} onChange={(event) => setNote(event.target.value)} placeholder="Only your team can see this" /></label>
              <div className="page-actions" style={{ marginTop: 12 }}>
                <button type="button" onClick={() => void claim()}>Assign to me</button>
                <button type="button" onClick={() => void addNote()}>Save note</button>
                <button type="button" className="primary" onClick={() => void resolve()}>Resolve</button>
                <button type="button" onClick={() => void reopen()}>Reopen</button>
              </div>
            </>
          )}
        </Card>
      </div>
    </Page>
  );
}
