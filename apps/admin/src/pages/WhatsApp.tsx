import { FormEvent, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet, apiPatch, apiPost } from "../api/client";
import { Card, EmptyState, Flash, FormGrid, Page, StatusDot } from "../components/ui";
import { type Row } from "../lib/format";

type Channel = Row & { id: string; provider: string; display_name: string; phone_number_id?: string; display_number?: string; status: string };
type Bot = Row & { name: string };

export function WhatsAppPage() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [bots, setBots] = useState<Bot[]>([]);
  const [advanced, setAdvanced] = useState(false);
  const [form, setForm] = useState({ display_number: "", phone_number_id: "", verify_token: "", access_token: "", app_secret: "" });
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

  async function load() {
    try {
      const [channelResponse, botResponse] = await Promise.all([apiGet<Channel[]>("/channels"), apiGet<Bot[]>("/bots").catch(() => ({ data: [] as Bot[] }))]);
      setChannels(channelResponse.data);
      setBots(botResponse.data);
      const channel = channelResponse.data.find((item) => item.provider === "whatsapp");
      if (channel) {
        setForm((current) => ({ ...current, display_number: channel.display_number || "", phone_number_id: channel.phone_number_id || "" }));
      }
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load WhatsApp");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const channel = channels.find((item) => item.provider === "whatsapp");
  const live = channel?.status === "active";
  const bot = bots[0];

  async function connect(event: FormEvent) {
    event.preventDefault();
    const payload: Row = {
      provider: "whatsapp",
      display_name: "WhatsApp",
      display_number: form.display_number,
      phone_number_id: form.phone_number_id,
      status: "active",
    };
    if (form.verify_token || bot?.id) {
      payload.config = JSON.stringify({ verify_token: form.verify_token || undefined, bot_id: bot?.id });
    }
    if (form.access_token || form.app_secret) {
      payload.secret_config = JSON.stringify({ access_token: form.access_token || undefined, app_secret: form.app_secret || undefined });
    }
    if (channel) {
      await apiPatch<Channel>(`/channels/${channel.id}`, payload);
    } else {
      await apiPost<Channel>("/channels", payload);
    }
    setForm((current) => ({ ...current, access_token: "", app_secret: "", verify_token: "" }));
    setFlash("WhatsApp connection saved. Secrets are never shown again.");
    await load();
  }

  async function test() {
    if (!channel) return;
    const response = await apiPost<Row>(`/channels/${channel.id}/test`, {});
    setFlash(String(response.data.status) === "ok" ? "Connection looks complete." : "This number still needs attention. Check Advanced details.");
  }

  async function disconnect() {
    if (!channel) return;
    await apiPost<Channel>(`/channels/${channel.id}/disconnect`, {});
    setFlash("WhatsApp disconnected.");
    await load();
  }

  async function reconnect() {
    if (!channel) return;
    await apiPatch<Channel>(`/channels/${channel.id}`, { status: "active" });
    setFlash("WhatsApp reconnected.");
    await load();
  }

  return (
    <Page title="WhatsApp" description="The number customers message to reach your assistant." help="Access tokens and Meta IDs stay under Advanced. They are never displayed after save.">
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <Card>
        {channel ? (
          <>
            <p><StatusDot live={live} label={live ? "Connected" : "Disconnected"} /></p>
            <p><strong>{channel.display_number || "Number not set"}</strong></p>
            <p className="muted">Assistant: {bot?.name || "Create an assistant first"}</p>
            <div className="page-actions" style={{ marginTop: 16 }}>
              <button type="button" onClick={() => void test()}>Test WhatsApp</button>
              {live ? <button type="button" className="danger" onClick={() => window.confirm("Disconnect WhatsApp?") && void disconnect()}>Disconnect</button> : <button type="button" className="primary" onClick={() => void reconnect()}>Reconnect</button>}
              <Link to="/assistant">Open assistant</Link>
            </div>
          </>
        ) : (
          <EmptyState title="WhatsApp is not connected yet" body="Add the customer-facing number. A technician may need to complete Advanced details once." />
        )}
      </Card>
      <FormGrid onSubmit={connect} title={channel ? "Update connection" : "Connect WhatsApp"}>
        <label>Phone number<input value={form.display_number} onChange={(event) => setForm({ ...form, display_number: event.target.value })} placeholder="+234..." required /></label>
        <div className="full"><button type="button" className="ghost" onClick={() => setAdvanced(!advanced)}>{advanced ? "Hide advanced" : "Show advanced"}</button></div>
        {advanced ? (
          <>
            <label>Phone number ID<input value={form.phone_number_id} onChange={(event) => setForm({ ...form, phone_number_id: event.target.value })} /></label>
            <label>Verify token<input value={form.verify_token} onChange={(event) => setForm({ ...form, verify_token: event.target.value })} /></label>
            <label>Access token<input type="password" value={form.access_token} onChange={(event) => setForm({ ...form, access_token: event.target.value })} placeholder="Leave blank to keep current" /></label>
            <label>App secret<input type="password" value={form.app_secret} onChange={(event) => setForm({ ...form, app_secret: event.target.value })} placeholder="Leave blank to keep current" /></label>
          </>
        ) : null}
        <div className="full"><button type="submit">{channel ? "Save" : "Connect"}</button></div>
      </FormGrid>
    </Page>
  );
}
