import { useState } from "react";
import { apiGet, apiPost } from "../../api/client";
import { Button, Flash } from "../ui";
import { WhatsAppPhone } from "../whatsapp/WhatsAppPhone";
import { WhatsAppThread } from "../whatsapp/WhatsAppThread";
import "./flow-studio.css";

type ChatMessage = { id: string; role: "user" | "assistant"; body: string; created_at: string };
/** Keep the test session when the responsive preview moves into or out of its sheet. */
export function useAIConversationPreview(organizationID: string) {
  const [session, setSession] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [answer, setAnswer] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function run(work: () => Promise<void>) {
    setBusy(true); setError("");
    try { await work(); } catch (reason) { setError(reason instanceof Error ? reason.message : "AI preview could not be completed."); }
    finally { setBusy(false); }
  }
  async function start() {
    await run(async () => {
      const response = await apiPost<{ session_id: string }>("/ai/test-chat/start", { organization_id: organizationID });
      const history = (await apiGet<ChatMessage[]>(`/ai/test-chat/sessions/${response.data.session_id}/messages`)).data;
      setSession(response.data.session_id); setMessages(history); setAnswer("");
    });
  }
  async function send() {
    const text = answer.trim(); if (!session || !text || busy) return;
    await run(async () => {
      const response = await apiPost<{ message: ChatMessage }>("/ai/test-chat/message", { session_id: session, text });
      setMessages(current => [...current, { id: crypto.randomUUID(), role: "user", body: text, created_at: new Date().toISOString() }, response.data.message]); setAnswer("");
    });
  }
  function reset() { setSession(""); setMessages([]); setAnswer(""); setError(""); }
  return { session, messages, answer, setAnswer, busy, error, start, send, reset };
}
/** This endpoint tests the grounded assistant and does not accept journey instructions. */
export function AIConversationPreview({ name, chat }: { name: string; chat: ReturnType<typeof useAIConversationPreview> }) {
  return <div className="flow-ai-preview"><Flash message={chat.error}/><p className="muted">Live AI test · uses your grounded assistant and Business knowledge.</p><WhatsAppPhone businessName={name} label="AI preview only"><WhatsAppThread busy={chat.busy} lines={chat.messages.map(message => ({ from: message.role === "user" ? "customer" as const : "bot" as const, message: { type: "text", text: message.body }, timestamp: message.created_at }))}/>{chat.session ? <form className="flow-ai-answer" onSubmit={event => { event.preventDefault(); void chat.send(); }}><input aria-label="AI test question" value={chat.answer} onChange={event => chat.setAnswer(event.target.value)} disabled={chat.busy}/><Button type="submit" disabled={chat.busy || !chat.answer.trim()}>Send</Button></form> : <Button disabled={chat.busy} onClick={() => void chat.start()}>Start AI preview</Button>}</WhatsAppPhone>{chat.session && <Button className="secondary" disabled={chat.busy} onClick={() => void chat.start()}>Restart AI preview</Button>}</div>;
}
