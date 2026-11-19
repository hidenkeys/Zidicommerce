import { FormEvent, useEffect, useState } from "react";
import { apiGet, apiPost } from "../api/client";
import { Button, Drawer, EmptyState, FilterBar, Flash, FormGrid, LoadingState, Page, SearchField } from "../components/ui";
import { type Row } from "../lib/format";

const emptyForm = { name: "", slug: "", currency: "NGN", timezone: "Africa/Lagos" };

export function OrganizationsPage() {
  const [rows, setRows] = useState<Row[]>([]);
  const [form, setForm] = useState(emptyForm);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [formError, setFormError] = useState("");
  const [success, setSuccess] = useState("");

  async function load() {
    setLoading(true);
    setMessage("");
    try {
      const response = await apiGet<Row[]>("/organizations");
      setRows(response.data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load organizations");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, []);

  function startCreate() {
    setForm(emptyForm);
    setFormError("");
    setOpen(true);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setFormError("");
    setSuccess("");
    try {
      await apiPost<Row>("/organizations", form);
      setOpen(false);
      setSuccess(`${form.name} created.`);
      setForm(emptyForm);
      await load();
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Could not create organization");
    } finally {
      setSaving(false);
    }
  }

  const filtered = rows.filter((row) =>
    [row.name, row.slug, row.currency, row.timezone].some((value) => String(value ?? "").toLowerCase().includes(query.trim().toLowerCase())),
  );

  return (
    <Page title="Organizations" description="Business workspaces across the platform." actions={<Button className="primary" onClick={startCreate}>Create organization</Button>}>
      <Flash message={message} />
      <Flash message={success} tone="success" />
      <FilterBar>
        <SearchField value={query} onChange={setQuery} placeholder="Search organizations" />
        <span className="muted">{filtered.length} {filtered.length === 1 ? "organization" : "organizations"}</span>
      </FilterBar>
      {loading ? <LoadingState label="Loading organizations" /> : message && rows.length === 0 ? (
        <EmptyState title="Organizations unavailable" body="Try loading the directory again." action={<Button onClick={() => void load()}>Try again</Button>} />
      ) : filtered.length === 0 ? (
        <EmptyState title={query ? "No matching organizations" : "No organizations yet"} body={query ? "Try another name, short code or currency." : "Create the first business workspace to get started."} action={query ? <Button onClick={() => setQuery("")}>Clear search</Button> : <Button onClick={startCreate}>Create organization</Button>} />
      ) : (
        <div className="table-wrap">
          <table>
            <thead><tr><th scope="col">Organization</th><th scope="col">Short code</th><th scope="col">Currency</th><th scope="col">Timezone</th></tr></thead>
            <tbody>{filtered.map((row) => (
              <tr key={String(row.id)}><td><strong>{String(row.name ?? "Unnamed organization")}</strong></td><td>{String(row.slug || "—")}</td><td>{String(row.currency || "—")}</td><td>{String(row.timezone || "—")}</td></tr>
            ))}</tbody>
          </table>
        </div>
      )}
      <Drawer open={open} title="Create organization" onClose={() => { if (!saving) setOpen(false); }}>
        <Flash message={formError} />
        <FormGrid onSubmit={submit}>
          <label className="full">Business name<input autoComplete="organization" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required /></label>
          <label className="full">Short code<input value={form.slug} onChange={(event) => setForm({ ...form, slug: event.target.value })} /></label>
          <label>Currency<input value={form.currency} onChange={(event) => setForm({ ...form, currency: event.target.value })} /></label>
          <label>Timezone<input value={form.timezone} onChange={(event) => setForm({ ...form, timezone: event.target.value })} /></label>
          <div className="full page-actions"><Button type="button" disabled={saving} onClick={() => setOpen(false)}>Cancel</Button><Button type="submit" className="primary" loading={saving}>{saving ? "Creating…" : "Create organization"}</Button></div>
        </FormGrid>
      </Drawer>
    </Page>
  );
}
