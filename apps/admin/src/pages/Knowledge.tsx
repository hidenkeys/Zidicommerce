import { FormEvent, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet, apiPatch, apiPost } from "../api/client";
import { Card, EmptyState, Flash, FormGrid, Page } from "../components/ui";
import { type Row } from "../lib/format";

type FAQ = Row & { id: string; question: string; answer: string; keywords?: string; status: string };

export function KnowledgePage() {
  const [faqs, setFaqs] = useState<FAQ[]>([]);
  const [form, setForm] = useState({ question: "", answer: "" });
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState("");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

  async function load() {
    try {
      const response = await apiGet<FAQ[]>("/bot-faqs");
      setFaqs(response.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load answers");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    await apiPost<FAQ>("/bot-faqs", { question: form.question, answer: form.answer, status: "active" });
    setForm({ question: "", answer: "" });
    setFlash("Answer added. Test it below, then try the assistant.");
    await load();
  }

  async function archive(faq: FAQ) {
    await apiPatch<FAQ>(`/bot-faqs/${faq.id}`, { status: "archived" });
    await load();
  }

  async function testMatch() {
    try {
      const response = await apiPost<{ faq?: { answer?: string }; answer?: string }>("/bot-faqs/match", { query });
      setMatch(String(response.data.faq?.answer ?? response.data.answer ?? "Matched, but no answer text was returned."));
    } catch (error) {
      setMatch(error instanceof Error ? error.message : "No matching answer");
    }
  }

  return (
    <Page
      title="Knowledge"
      description="What information should your assistant know?"
      help="Write the question in the customer’s words, then the answer you want sent back."
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FormGrid onSubmit={submit} title="Add FAQ">
        <label className="full">Question<input value={form.question} onChange={(event) => setForm({ ...form, question: event.target.value })} placeholder="What time do you close?" required /></label>
        <label className="full">Answer<textarea value={form.answer} onChange={(event) => setForm({ ...form, answer: event.target.value })} placeholder="We are open from 9am to 9pm Monday to Saturday." required /></label>
        <div className="full"><button type="submit">Add FAQ</button></div>
      </FormGrid>
      <Card>
        <h3>Test your assistant</h3>
        <p className="muted">This only checks whether an FAQ would match. It does not send a WhatsApp message.</p>
        <div className="filter-bar">
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Ask a sample question" />
          <button type="button" onClick={() => void testMatch()}>Test</button>
          <Link to="/assistant">Open assistant test</Link>
        </div>
        {match ? <p>{match}</p> : null}
      </Card>
      <Card>
        {faqs.length === 0 ? (
          <EmptyState title="No answers yet" body="Add the questions customers ask most often." />
        ) : faqs.filter((faq) => faq.status !== "archived").map((faq) => (
          <div key={faq.id} className="setup-item" style={{ marginBottom: 8 }}>
            <span>
              <strong>{faq.question}</strong>
              <em>{faq.answer}</em>
            </span>
            <button type="button" className="ghost" onClick={() => void archive(faq)}>Hide</button>
          </div>
        ))}
      </Card>
    </Page>
  );
}
