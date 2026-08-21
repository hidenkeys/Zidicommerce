import { FormEvent, useEffect, useState } from "react";
import { apiGet, apiPatch } from "../api/client";
import { Card, Flash, FormGrid, Page } from "../components/ui";
import { type Row } from "../lib/format";

type Organization = Row & { id: string; name?: string };

export function BusinessPage() {
  const [org, setOrg] = useState<Organization | null>(null);
  const [form, setForm] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

  useEffect(() => {
    async function load() {
      try {
        const response = await apiGet<Organization>("/organizations/current");
        setOrg(response.data);
        const next: Record<string, string> = {};
        for (const key of ["name", "description", "logo_url", "country", "currency", "timezone", "contact_name", "contact_email", "contact_phone"]) {
          next[key] = String(response.data[key] ?? "");
        }
        setForm(next);
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load business profile");
      }
    }
    void load();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!org?.id) {
      setMessage("No business is linked to this account yet.");
      return;
    }
    const payload = Object.fromEntries(Object.entries(form).filter(([, value]) => value.trim() !== ""));
    const response = await apiPatch<Organization>(`/organizations/${org.id}`, payload);
    setOrg(response.data);
    setFlash("Business information saved.");
  }

  return (
    <Page title="Business" description="Your public name, contact details, currency, and timezone." help="This is what the assistant and receipts use for your business.">
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <Card>
        <p><strong>{org?.name || "Your business"}</strong></p>
        <p className="muted">{form.currency || "NGN"} · {form.timezone || "Timezone not set"}</p>
      </Card>
      <FormGrid onSubmit={submit}>
        <label>Business name<input value={form.name ?? ""} onChange={(event) => setForm({ ...form, name: event.target.value })} /></label>
        <label>Contact name<input value={form.contact_name ?? ""} onChange={(event) => setForm({ ...form, contact_name: event.target.value })} /></label>
        <label>Contact email<input value={form.contact_email ?? ""} onChange={(event) => setForm({ ...form, contact_email: event.target.value })} /></label>
        <label>Contact phone<input value={form.contact_phone ?? ""} onChange={(event) => setForm({ ...form, contact_phone: event.target.value })} /></label>
        <label>Country<input value={form.country ?? ""} onChange={(event) => setForm({ ...form, country: event.target.value })} /></label>
        <label>Currency<input value={form.currency ?? ""} onChange={(event) => setForm({ ...form, currency: event.target.value })} /></label>
        <label>Timezone<input value={form.timezone ?? ""} onChange={(event) => setForm({ ...form, timezone: event.target.value })} placeholder="Africa/Lagos" /></label>
        <label>Logo URL<input value={form.logo_url ?? ""} onChange={(event) => setForm({ ...form, logo_url: event.target.value })} /></label>
        <label className="full">About<textarea value={form.description ?? ""} onChange={(event) => setForm({ ...form, description: event.target.value })} /></label>
        <div className="full"><button type="submit">Save</button></div>
      </FormGrid>
    </Page>
  );
}
