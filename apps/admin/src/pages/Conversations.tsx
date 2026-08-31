import { FormEvent, useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Bot, CheckCircle2, Eye, EyeOff, Send, UserCheck, Users } from "lucide-react";
import { apiGet, apiPost } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Button, Card, EmptyState, FilterBar, Flash, IconButton, LoadingState, Page, SearchField } from "../components/ui";
import { humanStatus, maskPhone, relativeTime, roleLabel, type Row } from "../lib/format";
import { hasPermission } from "../lib/permissions";

type Conversation = Row & {
  id: string;
  channel_id?: string;
  external_conversation_id?: string;
  status?: string;
  conversation_status?: string;
  handoff_status?: string;
  handoff_id?: string;
  assigned_user_id?: string;
  assigned_user_name?: string;
  customer_name?: string;
  customer_phone?: string;
  customer_id?: string;
  store_name?: string;
  order_id?: string;
  order_number?: string;
  order_status?: string;
  payment_status?: string;
  fulfilment_status?: string;
  fulfilment_type?: string;
  last_message?: string;
  unread_count?: number;
  priority?: string;
  updated_at?: string;
};

type Message = Row & {
  id: string;
  direction?: string;
  body?: string;
  text?: string;
  author_type?: string;
};

type Member = Row & {
  id: string;
  user_id?: string;
  role?: string;
  user?: Row;
};

type WhatsAppPolicyContext = {
  consent_status: string;
  masked_phone?: string;
  service_window_open: boolean;
  last_inbound_at?: string;
  service_window_expires_at?: string;
  template_required: boolean;
  can_send_freeform: boolean;
  reason: string;
};

const statusFilters = [
  { value: "all", label: "All" },
  { value: "human_requested", label: "Needs owner" },
  { value: "human_assigned", label: "Assigned" },
  { value: "waiting", label: "Waiting" },
  { value: "ai_handling", label: "AI handling" },
  { value: "resolved", label: "Resolved" },
];

function inboxStatus(row: Conversation) {
  const status = String(row.conversation_status || "");
  if (status === "resolved" || row.status === "completed" || row.status === "cancelled") return "Resolved";
  if (status === "human_assigned" || row.handoff_status === "assigned") return "Assigned";
  if (status === "human_requested" || row.status === "handoff" || row.handoff_status === "open" || row.handoff_status === "reopened") return "Needs owner";
  if (status === "waiting") return "Waiting";
  if (status === "ai_handling") return "AI handling";
  return "Open";
}

function tone(status: string): "success" | "warning" | "danger" | "info" | "neutral" {
  if (status === "Resolved") return "success";
  if (status === "Needs owner") return "danger";
  if (status === "Assigned" || status === "Waiting") return "warning";
  if (status === "AI handling") return "info";
  return "neutral";
}

function priorityTone(priority: unknown): "warning" | "danger" | "neutral" {
  const value = String(priority || "normal");
  if (value === "urgent" || value === "high") return "danger";
  if (value === "low") return "neutral";
  return "warning";
}

function memberUserID(member: Member) {
  return String(member.user_id || (member.user?.id as string | undefined) || member.id || "");
}

function memberName(member: Member) {
  const user = member.user ?? {};
  return [user.first_name, user.last_name].filter(Boolean).join(" ") || String(user.email ?? "Team member");
}

function canHandleConversation(member: Member) {
  return ["merchant_admin", "store_manager", "support_agent"].includes(String(member.role));
}

function queryString(filter: string, assigned: string, unreadOnly: boolean, search: string) {
  const params = new URLSearchParams();
  if (filter !== "all") params.set("status", filter);
  if (assigned !== "all") params.set("assigned", assigned);
  if (unreadOnly) params.set("unread", "true");
  if (search.trim()) params.set("search", search.trim());
  const value = params.toString();
  return value ? `?${value}` : "";
}

export function ConversationsPage() {
  const { user } = useAuth();
  const canManageConversations = hasPermission(user.role, "conversations.manage");
  const [params] = useSearchParams();
  const [rows, setRows] = useState<Conversation[]>([]);
  const [members, setMembers] = useState<Member[]>([]);
  const [selectedID, setSelectedID] = useState(params.get("open") ?? "");
  const [messages, setMessages] = useState<Message[]>([]);
  const [reply, setReply] = useState("");
  const [note, setNote] = useState("");
  const [assigneeID, setAssigneeID] = useState("");
  const [filter, setFilter] = useState("all");
  const [assigned, setAssigned] = useState("all");
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [query, setQuery] = useState("");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [policyContext, setPolicyContext] = useState<WhatsAppPolicyContext | null>(null);
  const [loadingPolicyContext, setLoadingPolicyContext] = useState(false);

  async function load() {
    try {
      const [conversationResponse, memberResponse] = await Promise.all([
        apiGet<Conversation[]>(`/runtime/conversations${queryString(filter, assigned, unreadOnly, query)}`),
        apiGet<Member[]>("/organizations/current/members").catch(() => ({ data: [] as Member[] })),
      ]);
      setRows(conversationResponse.data);
      setSelectedID((current) => current || conversationResponse.data[0]?.id || "");
      setMembers(memberResponse.data.filter(canHandleConversation));
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load conversations");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [filter, assigned, unreadOnly]);

  async function openConversation(id: string) {
    setSelectedID(id);
    setLoadingDetail(true);
    setLoadingPolicyContext(true);
    setPolicyContext(null);
    try {
      const [detailResponse, messageResponse] = await Promise.all([
        apiGet<Conversation>(`/runtime/conversations/${id}`),
        apiGet<Message[]>(`/runtime/conversations/${id}/messages`),
      ]);
      setRows((current) => {
        const next = current.filter((row) => row.id !== id);
        return [detailResponse.data, ...next].sort((a, b) => String(b.updated_at ?? "").localeCompare(String(a.updated_at ?? "")));
      });
      setMessages(messageResponse.data);
      setAssigneeID(String(detailResponse.data.assigned_user_id ?? ""));
      if (detailResponse.data.channel_id) {
        const policyResponse = await apiGet<WhatsAppPolicyContext>(`/channel-platform/connections/${detailResponse.data.channel_id}/whatsapp/conversations/${id}/policy-context`).catch(() => null);
        setPolicyContext(policyResponse?.data ?? null);
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load the conversation");
    } finally {
      setLoadingDetail(false);
      setLoadingPolicyContext(false);
    }
  }

  useEffect(() => {
    if (selectedID) void openConversation(selectedID);
  }, [selectedID]);

  const selected = rows.find((row) => row.id === selectedID);

  async function refreshSelected() {
    await load();
    if (selectedID) {
      await openConversation(selectedID);
    }
  }

  async function sendReply(event: FormEvent) {
    event.preventDefault();
    if (!selectedID || !reply.trim()) return;
    try {
      await apiPost<Row>(`/runtime/conversations/${selectedID}/reply`, { text: reply });
      setReply("");
      setFlash("Reply sent.");
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Reply could not be sent");
    }
  }

  async function claim() {
    if (!selectedID) return;
    try {
      const handoffID = String(selected?.handoff_id || "");
      if (handoffID) {
        await apiPost<Row>(`/runtime/support-handoffs/${handoffID}/claim`, { note, priority: selected?.priority || "normal" });
      } else {
        const response = await apiPost<Row>(`/runtime/conversations/${selectedID}/handoff`, { reason: note });
        await apiPost<Row>(`/runtime/support-handoffs/${response.data.id}/claim`, { note, priority: selected?.priority || "normal" });
      }
      setNote("");
      setFlash("Assigned to you.");
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Conversation could not be claimed");
    }
  }

  async function assignToUser() {
    if (!selectedID || !assigneeID) return;
    try {
      await apiPost<Row>(`/runtime/conversations/${selectedID}/assign`, { user_id: assigneeID, reason: note });
      setNote("");
      setFlash("Conversation assigned.");
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Conversation could not be assigned");
    }
  }

  async function addNote() {
    if (!selectedID || !note.trim()) return;
    try {
      await apiPost<Row>(`/runtime/conversations/${selectedID}/notes`, { note, internal: true });
      setNote("");
      setFlash("Note saved.");
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Note could not be saved");
    }
  }

  async function markRead(unread: boolean) {
    if (!selectedID) return;
    try {
      await apiPost<Row>(`/runtime/conversations/${selectedID}/${unread ? "unread" : "read"}`, {});
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Read state could not be changed");
    }
  }

  async function releaseToAI() {
    if (!selectedID) return;
    try {
      await apiPost<Row>(`/runtime/conversations/${selectedID}/unassign`, { reason: note, resume_bot: true });
      setNote("");
      setFlash("Released to AI.");
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Conversation could not be released");
    }
  }

  async function resolve(resumeBot = false) {
    if (!selectedID) return;
    try {
      await apiPost<Row>(`/runtime/conversations/${selectedID}/resolve`, { resolution_note: note, resume_bot: resumeBot });
      setNote("");
      setFlash(resumeBot ? "Resolved and returned to AI." : "Marked resolved.");
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Conversation could not be resolved");
    }
  }

  async function reopen() {
    if (!selectedID) return;
    try {
      await apiPost<Row>(`/runtime/conversations/${selectedID}/reopen`, { reason: note });
      setNote("");
      setFlash("Reopened.");
      await refreshSelected();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Conversation could not be reopened");
    }
  }

  const selectedStatus = selected ? inboxStatus(selected) : "";
  const humanOwned = selectedStatus === "Assigned" || selectedStatus === "Needs owner";
  const freeformBlocked = loadingPolicyContext || Boolean(policyContext && !policyContext.can_send_freeform);

  return (
    <Page title="Conversations" description="Support inbox for customer messages, orders, and human handoff." help={canManageConversations ? "When a person owns a conversation, the assistant stays paused until it is explicitly released." : "Your role can review conversations, but cannot reply, change ownership, or resolve them."}>
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search customer, store, or message" />
        <select value={filter} onChange={(event) => setFilter(event.target.value)}>
          {statusFilters.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
        </select>
        <select value={assigned} onChange={(event) => setAssigned(event.target.value)}>
          <option value="all">All owners</option>
          <option value="me">Assigned to me</option>
          <option value="unassigned">Unassigned</option>
        </select>
        <label className="inline-check compact-check">
          <input type="checkbox" checked={unreadOnly} onChange={(event) => setUnreadOnly(event.target.checked)} />
          Unread
        </label>
        <button type="button" onClick={() => void load()}>Search</button>
      </FilterBar>
      <div className="inbox-workspace">
        <Card className="inbox-list">
          {loading ? <LoadingState label="Loading inbox" /> : rows.length === 0 ? (
            <EmptyState title="No conversations yet" body="Customer conversations will appear here." />
          ) : (
            <div className="conversation-list">
              {rows.map((row) => {
                const status = inboxStatus(row);
                return (
                  <button type="button" key={row.id} className={selectedID === row.id ? "conversation-item selected" : "conversation-item"} onClick={() => setSelectedID(row.id)}>
                    <span className="conversation-main">
                      <strong>{String(row.customer_name || maskPhone(row.customer_phone, "Customer"))}</strong>
                      <span>{String(row.last_message || "").slice(0, 110) || "No message yet"}</span>
                    </span>
                    <span className="conversation-meta">
                      <Badge tone={tone(status)}>{status}</Badge>
                      <Badge tone={priorityTone(row.priority)}>{humanStatus(row.priority || "normal")}</Badge>
                      {Number(row.unread_count || 0) > 0 ? <Badge tone="danger">{Number(row.unread_count)} unread</Badge> : null}
                      <small>{String(row.assigned_user_name || row.store_name || relativeTime(row.updated_at))}</small>
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </Card>
        <Card className="conversation-thread">
          {loadingDetail ? <LoadingState label="Loading conversation" /> : !selected ? (
            <EmptyState title="Select a conversation" body="Read the timeline, claim ownership, reply, or resolve it." />
          ) : (
            <>
              <div className="conversation-head">
                <div>
                  <h3>{String(selected.customer_name || maskPhone(selected.customer_phone, "Customer"))}</h3>
                  <p className="muted">{policyContext?.masked_phone || maskPhone(selected.customer_phone, "Customer conversation")} · {relativeTime(selected.updated_at)}</p>
                </div>
                {canManageConversations ? <div className="page-actions">
                  <IconButton label="Mark read" icon={Eye} onClick={() => void markRead(false)} />
                  <IconButton label="Mark unread" icon={EyeOff} onClick={() => void markRead(true)} />
                </div> : null}
              </div>
              <div className={humanOwned ? "ownership-banner human" : "ownership-banner ai"}>{humanOwned ? <Users size={18} aria-hidden="true" /> : <Bot size={18} aria-hidden="true" />}<div><strong>{humanOwned ? "Human support is active" : "Zidi assistant is active"}</strong><span>{humanOwned ? selected.assigned_user_name ? `Owned by ${selected.assigned_user_name}` : "Waiting for a team member" : "The assistant may respond automatically"}</span></div></div>
              {policyContext ? <div className={policyContext.can_send_freeform ? "whatsapp-policy-context allowed" : "whatsapp-policy-context blocked"}>
                <div className="whatsapp-policy-status">
                  <strong>WhatsApp messaging policy</strong>
                  <Badge tone={policyContext.consent_status === "opted_out" ? "danger" : "neutral"}>{humanStatus(policyContext.consent_status)}</Badge>
                  <Badge tone={policyContext.consent_status === "opted_out" ? "danger" : policyContext.service_window_open ? "success" : "warning"}>{policyContext.consent_status === "opted_out" ? "Messaging blocked" : policyContext.service_window_open ? "Service window open" : "Template required"}</Badge>
                </div>
                <span>{policyContext.reason}{policyContext.service_window_expires_at ? ` · Window ends ${relativeTime(policyContext.service_window_expires_at)}` : ""}</span>
              </div> : null}
              <div className="thread">
                {messages.map((item) => (
                  <div key={item.id} className={item.direction === "inbound" ? "bubble customer" : item.direction === "internal" ? "bubble internal" : "bubble"}>
                    <span>{item.direction === "inbound" ? "Customer" : item.direction === "internal" ? "Internal note - not visible to customer" : item.author_type === "agent" ? "Team member" : "Zidi assistant"}</span>
                    <p>{String(item.body ?? item.text ?? "")}</p>
                  </div>
                ))}
              </div>
              {canManageConversations ? <form className="conversation-composer" onSubmit={sendReply}>
                {policyContext && !policyContext.can_send_freeform ? <p className="conversation-policy-note">{policyContext.consent_status === "opted_out" ? "Replies are disabled because this contact opted out." : "The service window is closed. Send an approved template from Channels to reopen the conversation."}</p> : null}
                <label><span className="sr-only">Reply to customer</span><textarea value={reply} onChange={(event) => setReply(event.target.value)} required disabled={freeformBlocked} placeholder={loadingPolicyContext ? "Checking WhatsApp messaging policy" : "Write a customer-visible reply"} /></label>
                <Button type="submit" className="primary" icon={Send} disabled={freeformBlocked}>Send</Button>
              </form> : <p className="read-only-note">Conversation actions are hidden because this workspace is read-only for your role.</p>}
            </>
          )}
        </Card>
        <Card className="conversation-context">
          {!selected ? <EmptyState title="Customer context" body="Select a conversation to see ownership, order, payment, and fulfilment." /> : <>
            <section><span className="context-label">Customer</span><h3>{String(selected.customer_name || "Customer")}</h3><p>{policyContext?.masked_phone || maskPhone(selected.customer_phone, "No phone number")}</p>{selected.customer_id ? <Link to={`/customers?open=${selected.customer_id}`}>View customer profile</Link> : null}</section>
            <section><span className="context-label">Conversation</span><div className="context-row"><span>Ownership</span><Badge tone={tone(selectedStatus)}>{selectedStatus}</Badge></div><div className="context-row"><span>Priority</span><Badge tone={priorityTone(selected.priority)}>{humanStatus(selected.priority || "normal")}</Badge></div><div className="context-row"><span>Store</span><strong>{selected.store_name || "Not selected"}</strong></div></section>
            {selected.order_id ? <section><span className="context-label">Order</span><h3><Link to={`/orders?open=${selected.order_id}`}>{String(selected.order_number || "View order")}</Link></h3><div className="context-row"><span>Status</span><strong>{humanStatus(selected.order_status)}</strong></div><div className="context-row"><span>Payment</span><strong>{humanStatus(selected.payment_status || "not_initialized")}</strong></div><div className="context-row"><span>Fulfilment</span><strong>{humanStatus(selected.fulfilment_status || selected.fulfilment_type || "pending")}</strong></div></section> : <section><span className="context-label">Order</span><p className="muted">No order is linked to this conversation.</p></section>}
            {canManageConversations ? <>
              <section><span className="context-label">Ownership and handoff</span><label>Transfer to<select value={assigneeID} onChange={(event) => setAssigneeID(event.target.value)}><option value="">Choose teammate</option>{members.map((member) => <option key={member.id} value={memberUserID(member)}>{memberName(member)} · {roleLabel(String(member.role))}</option>)}</select></label><div className="conversation-actions"><Button type="button" icon={UserCheck} onClick={() => void claim()}>Assign to me</Button><button type="button" onClick={() => void assignToUser()} disabled={!assigneeID}>Transfer</button>{humanOwned ? <button type="button" onClick={() => void releaseToAI()}>Release to AI</button> : null}</div></section>
              <section><span className="context-label">Internal note</span><label><span className="sr-only">Internal note</span><textarea value={note} onChange={(event) => setNote(event.target.value)} placeholder="Only your team can see this" /></label><button type="button" onClick={() => void addNote()} disabled={!note.trim()}>Save internal note</button></section>
              <section><span className="context-label">Close conversation</span><div className="conversation-actions">{selectedStatus === "Resolved" ? <button type="button" onClick={() => void reopen()}>Reopen</button> : <><Button type="button" className="primary" icon={CheckCircle2} onClick={() => void resolve(false)}>Resolve</Button><button type="button" onClick={() => void resolve(true)}>Resolve and release to AI</button></>}</div></section>
            </> : null}
          </>}
        </Card>
      </div>
    </Page>
  );
}
