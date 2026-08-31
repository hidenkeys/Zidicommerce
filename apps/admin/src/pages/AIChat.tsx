import { FormEvent, useEffect, useMemo, useState } from "react";
import { apiGet, apiPost } from "../api/client";
import { useAuth } from "../auth";
import { Card, EmptyState, Flash, Page } from "../components/ui";

type Row = Record<string, unknown>;
type Organization = Row & { id: string; name?: string };
type Customer = Row & { id: string; name?: string; phone?: string; email?: string };
type ChatMessage = { id: string; role: "user" | "assistant"; body: string; metadata?: string; created_at: string };
type ToolDebug = { name: string; inputs?: Row; result?: Row; status: string; error?: string; latency_ms?: number };
type ChatDebug = { provider: string; model: string; latency_ms: number; tools: ToolDebug[]; error?: string };
type StartResponse = { session_id: string; organization_id: string; customer_id?: string };
type MessageResponse = { session_id: string; message: ChatMessage; debug: ChatDebug };

function pretty(value: unknown) {
  return JSON.stringify(value ?? {}, null, 2);
}

export function AIChatPage() {
  const { user } = useAuth();
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [organizationID, setOrganizationID] = useState("");
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [customerID, setCustomerID] = useState("");
  const [sessionID, setSessionID] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [debug, setDebug] = useState<ChatDebug | null>(null);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [flash, setFlash] = useState("");

  const selectedCustomer = useMemo(() => customers.find((customer) => customer.id === customerID), [customers, customerID]);

  async function load() {
    setLoading(true);
    setFlash("");
    try {
      if (user.role === "platform_admin") {
        const orgResponse = await apiGet<Organization[]>("/organizations");
        setOrganizations(orgResponse.data);
        setOrganizationID((current) => current || orgResponse.data[0]?.id || "");
      } else {
        const current = await apiGet<Organization>("/organizations/current");
        setOrganizations([current.data]);
        setOrganizationID(current.data.id);
      }
    } catch (error) {
      setFlash(error instanceof Error ? error.message : "Could not load organizations");
    } finally {
      setLoading(false);
    }
  }

  async function loadCustomers() {
    setFlash("");
    try {
      const response = await apiGet<Customer[]>("/customers");
      setCustomers(response.data);
      setCustomerID((current) => current || response.data[0]?.id || "");
    } catch (error) {
      setFlash(error instanceof Error ? error.message : "Could not load customers");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    if (organizationID && user.role !== "platform_admin") {
      void loadCustomers();
    } else if (organizationID && user.role === "platform_admin") {
      setCustomers([]);
      setCustomerID("");
    }
  }, [organizationID, user.role]);

  async function startChat() {
    setFlash("");
    setDebug(null);
    setMessages([]);
    try {
      const body: Row = {};
      if (organizationID) body.organization_id = organizationID;
      if (customerID) body.customer_id = customerID;
      const response = await apiPost<StartResponse>("/ai/test-chat/start", body);
      setSessionID(response.data.session_id);
      setFlash("AI test chat started.");
      const existing = await apiGet<ChatMessage[]>(`/ai/test-chat/sessions/${response.data.session_id}/messages`);
      setMessages(existing.data);
    } catch (error) {
      setFlash(error instanceof Error ? error.message : "Could not start AI chat");
    }
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    if (!sessionID || !input.trim()) return;
    const text = input.trim();
    setInput("");
    setSending(true);
    setFlash("");
    setMessages((current) => [...current, { id: `local-${Date.now()}`, role: "user", body: text, created_at: new Date().toISOString() }]);
    try {
      const response = await apiPost<MessageResponse>("/ai/test-chat/message", { session_id: sessionID, text });
      setMessages((current) => [...current.filter((message) => !message.id.startsWith("local-")), { id: `sent-${Date.now()}`, role: "user", body: text, created_at: new Date().toISOString() }, response.data.message]);
      setDebug(response.data.debug);
    } catch (error) {
      setFlash(error instanceof Error ? error.message : "Could not send message");
    } finally {
      setSending(false);
    }
  }

  return (
    <Page title="AI Chat" description="Local test console for Ollama plus read-only Zidi tools.">
      <Flash message={flash} tone={flash.includes("started") ? "success" : "error"} />
      <div className="two-column">
        <Card>
          <h3>Session</h3>
          {loading ? <p className="muted">Loading...</p> : null}
          <div className="form-grid">
            <label className="full">Organization
              <select value={organizationID} onChange={(event) => setOrganizationID(event.target.value)} disabled={user.role !== "platform_admin"}>
                {organizations.map((organization) => <option key={organization.id} value={organization.id}>{organization.name || organization.id}</option>)}
              </select>
            </label>
            <label className="full">Customer
              <select value={customerID} onChange={(event) => setCustomerID(event.target.value)}>
                <option value="">No selected customer</option>
                {customers.map((customer) => <option key={customer.id} value={customer.id}>{customer.name || customer.phone || customer.email || customer.id}</option>)}
              </select>
            </label>
            <div className="full">
              <button type="button" onClick={() => void startChat()} disabled={!organizationID}>Start conversation</button>
            </div>
          </div>
          {sessionID ? <p className="help-text">Session: {sessionID}</p> : null}
          {selectedCustomer ? <p className="help-text">Customer context: {selectedCustomer.name || selectedCustomer.phone || selectedCustomer.email}</p> : null}
        </Card>

        <Card>
          <h3>Debug</h3>
          {!debug ? <EmptyState title="No interaction yet" body="Send a message to inspect provider, model, latency, and tool calls." /> : (
            <div className="stack">
              <p><strong>{debug.provider}</strong> / {debug.model} · {debug.latency_ms}ms</p>
              {debug.tools.length === 0 ? <p className="muted">No tools called.</p> : debug.tools.map((tool, index) => (
                <details key={`${tool.name}-${index}`} open>
                  <summary>{tool.name} · {tool.status} · {tool.latency_ms ?? 0}ms</summary>
                  <pre>{pretty({ inputs: tool.inputs, result: tool.result, error: tool.error })}</pre>
                </details>
              ))}
            </div>
          )}
        </Card>
      </div>

      <Card>
        <h3>Conversation</h3>
        {!sessionID ? <EmptyState title="Start a conversation" body="Choose a merchant/customer context, then start the local AI chat." /> : null}
        <div className="chat-window">
          {messages.map((message) => (
            <div key={message.id} className={`chat-bubble ${message.role === "user" ? "from-user" : "from-assistant"}`}>
              <span>{message.role === "user" ? "Customer" : "Zidi"}</span>
              <p>{message.body}</p>
            </div>
          ))}
        </div>
        <form className="chat-input" onSubmit={send}>
          <input value={input} onChange={(event) => setInput(event.target.value)} placeholder="Ask about stores, products, stock, or order status..." disabled={!sessionID || sending} />
          <button type="submit" disabled={!sessionID || sending || !input.trim()}>{sending ? "Sending..." : "Send"}</button>
        </form>
      </Card>
    </Page>
  );
}
