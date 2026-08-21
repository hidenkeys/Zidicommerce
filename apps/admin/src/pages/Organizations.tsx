import { FormEvent, useEffect, useState } from "react";
import { apiGet, apiPost } from "../api/client";
import { Card, EmptyState, Flash, FormGrid, Page } from "../components/ui";
import { type Row } from "../lib/format";

export function OrganizationsPage() {
  const [rows, setRows] = useState<Row[]>([]);
  const [form, setForm] = useState({ name: "", slug: "", currency: "NGN", timezone: "Africa/Lagos" });
  const [message, setMessage] = useState("");

  async function load() {
    try {
      const response = await apiGet<Row[]>("/organizations");
      setRows(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load organizations");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    await apiPost<Row>("/organizations", form);
    setForm({ name: "", slug: "", currency: "NGN", timezone: "Africa/Lagos" });
    await load();
  }

  return (
    <Page title="Organizations" description="Platform tenants. Merchants should not need this screen.">
      <Flash message={message} />
      <FormGrid onSubmit={submit} title="Create organization">
        <label>Name<input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required /></label>
        <label>Short code<input value={form.slug} onChange={(event) => setForm({ ...form, slug: event.target.value })} /></label>
        <label>Currency<input value={form.currency} onChange={(event) => setForm({ ...form, currency: event.target.value })} /></label>
        <label>Timezone<input value={form.timezone} onChange={(event) => setForm({ ...form, timezone: event.target.value })} /></label>
        <div className="full"><button type="submit">Create</button></div>
      </FormGrid>
      <Card>
        {rows.length === 0 ? <EmptyState title="No organizations" /> : rows.map((row) => (
          <p key={String(row.id)}><strong>{String(row.name)}</strong> · {String(row.currency)}</p>
        ))}
      </Card>
    </Page>
  );
}
